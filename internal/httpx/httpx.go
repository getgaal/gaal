// Package httpx provides a single hardened HTTP client for outbound
// fetches (MCP remote configs, archive downloads). Centralising the
// configuration here makes the security posture auditable and prevents
// stray http.DefaultClient calls from regressing the defaults.
//
// Defaults applied:
//   - Hard timeout per request (configurable via context, plus a generous
//     fallback Client.Timeout so a hung connection cannot pin a sync
//     forever).
//   - TLS minimum version 1.2.
//   - Redirect cap of 10 hops; cross-scheme downgrades (https→http) and
//     redirects to loopback / RFC1918 hosts are rejected to defend
//     against SSRF via redirect.
//   - Stable User-Agent ("gaal/<version>") so well-behaved servers can
//     identify and rate-limit us.
//
// Body-size enforcement is the caller's responsibility — wrap the
// returned body in io.LimitReader(MaxBodyBytes) before consuming it.
// We don't hide that inside the client because tar/zip extraction
// needs to stream large payloads with its own per-entry caps.
package httpx

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// MaxBodyBytes is the recommended cap for ad-hoc JSON/text fetches
// (e.g. remote MCP server entries). Archive downloads use their own,
// larger cap defined in core/vcs.
const MaxBodyBytes int64 = 16 << 20 // 16 MiB

// DefaultClientTimeout is the per-request hard cap. Generous so that
// large archive downloads complete on slow links, but finite so a
// stalled handshake can't hang sync indefinitely.
const DefaultClientTimeout = 5 * time.Minute

// MaxRedirects caps how many 3xx hops the client will follow before
// giving up. Matches the historical http.DefaultClient cap so existing
// well-behaved redirects keep working.
const MaxRedirects = 10

var userAgent atomic.Value // string

func init() {
	userAgent.Store("gaal")
}

// SetUserAgent updates the User-Agent header used by Client(). Safe to
// call from cmd init once the version string is known.
func SetUserAgent(ua string) {
	if ua == "" {
		return
	}
	userAgent.Store(ua)
}

// UserAgent returns the current User-Agent.
func UserAgent() string {
	if v, ok := userAgent.Load().(string); ok {
		return v
	}
	return "gaal"
}

var sharedClient = newClient()

// Client returns the shared hardened http.Client. The same instance is
// returned on every call so connection pooling works across fetches.
func Client() *http.Client {
	return sharedClient
}

func newClient() *http.Client {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}

	return &http.Client{
		Transport:     transport,
		Timeout:       DefaultClientTimeout,
		CheckRedirect: checkRedirect,
	}
}

// ErrCrossSchemeRedirect is returned when an https:// response 3xx-points
// to an http:// target (other than loopback). Returning an error from
// CheckRedirect causes Client.Do to surface it to the caller without
// following the redirect.
var ErrCrossSchemeRedirect = errors.New("redirect downgrades scheme from https to http")

// ErrTooManyRedirects is returned when the redirect chain exceeds MaxRedirects.
var ErrTooManyRedirects = errors.New("too many redirects")

// ErrInternalRedirect is returned when an external host redirects to a
// loopback or RFC1918 / link-local address (SSRF defence).
var ErrInternalRedirect = errors.New("redirect to internal/loopback host blocked")

func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= MaxRedirects {
		return fmt.Errorf("%w (>%d hops)", ErrTooManyRedirects, MaxRedirects)
	}
	if len(via) == 0 {
		return nil
	}
	prev := via[len(via)-1].URL
	next := req.URL

	// Block https → http downgrades unless the target is loopback (CI
	// fixtures legitimately bounce through 127.0.0.1).
	if strings.EqualFold(prev.Scheme, "https") && strings.EqualFold(next.Scheme, "http") {
		if !isLoopback(next.Host) {
			return fmt.Errorf("%w: %s → %s", ErrCrossSchemeRedirect, prev.Redacted(), next.Redacted())
		}
	}

	// Block external → internal redirects. If the original request went to
	// a public host but a redirect points us at an internal/loopback
	// address, that's the textbook SSRF redirect attack.
	origin := via[0].URL
	if !isInternal(origin.Host) && isInternal(next.Host) {
		return fmt.Errorf("%w: %s → %s", ErrInternalRedirect, origin.Redacted(), next.Redacted())
	}

	// Carry the User-Agent across redirects (net/http drops some headers
	// across hops; UA is fine to propagate).
	req.Header.Set("User-Agent", UserAgent())
	return nil
}

// isLoopback reports whether host:port is a literal loopback address.
// We deliberately do not DNS-resolve; that would race against the
// actual connection.
func isLoopback(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// isInternal returns true for loopback, link-local, and RFC1918 / RFC4193
// private address space. Used by the redirect SSRF guard.
func isInternal(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsPrivate() {
		return true
	}
	return false
}

// NewRequest is a thin wrapper around http.NewRequestWithContext that
// applies the default User-Agent header. Use it instead of building
// requests by hand.
func NewRequest(ctx context.Context, method, rawURL string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent())
	return req, nil
}
