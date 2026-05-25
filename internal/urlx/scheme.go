package urlx

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ValidateRemoteFetchURL rejects any URL that gaal should not follow over
// HTTP — used by the MCP and archive fetchers. Allowed schemes are https://
// for any host, and http:// only when the host is a loopback address (so
// fixture servers in CI / e2e still work). Plain paths (no scheme) are
// rejected; this function is for *remote* fetches only.
//
// Defends against:
//   - file:// — exfil / arbitrary local read
//   - gopher://, dict://, ftp://, etc. — unintended protocols routed through
//     a permissive transport
//   - http:// to internal hosts (RFC 1918 / link-local) — SSRF; e.g. AWS IMDS
//     at 169.254.169.254
func ValidateRemoteFetchURL(rawurl string) error {
	if rawurl == "" {
		return fmt.Errorf("empty url")
	}
	u, err := url.Parse(rawurl)
	if err != nil {
		return fmt.Errorf("parsing url: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return nil
	case "http":
		if isLoopbackHost(u.Host) {
			return nil
		}
		return fmt.Errorf("scheme http:// is only allowed for loopback hosts (got host %q)", u.Host)
	case "":
		return fmt.Errorf("url %q is missing a scheme", rawurl)
	default:
		return fmt.Errorf("scheme %q is not allowed for remote fetches (allowed: https, http+loopback)", u.Scheme)
	}
}

// ValidateRepoURL rejects URLs that VCS clone backends should not follow.
// It is laxer than ValidateRemoteFetchURL because cloning legitimately uses
// ssh:// and git:// transports.
//
// Allowed:
//   - https://... (any host)
//   - ssh://... (any host)
//   - git://... (any host; deprecated transport but still in use)
//   - http://, svn://, bzr:// only for loopback hosts (CI fixtures)
//   - SCP-style git URLs (user@host:path) — common for git remotes
//   - empty scheme — treated as a local filesystem path (containment is the
//     responsibility of the caller; see issue #118)
//
// Rejected: file://, gopher://, ftp://, dict://, anything else.
//
// svn:// and bzr:// (the daemon protocols of those backends) are gated to
// loopback only — same threat model as http://. Public-network svn / bzr
// access has not been requested in any real config; if it ever is, the
// gate can be widened with explicit security review.
func ValidateRepoURL(rawurl string) error {
	if rawurl == "" {
		return fmt.Errorf("empty url")
	}
	if isSCPStyleGitURL(rawurl) {
		return nil
	}
	// "C:\path" and "C:/path" are local Windows paths, not URLs — but net/url
	// parses them with scheme="c" and would fall through to the rejection
	// branch below. Treat them like any other local path (empty-scheme case).
	if isWindowsDrivePath(rawurl) {
		return nil
	}
	u, err := url.Parse(rawurl)
	if err != nil {
		return fmt.Errorf("parsing url: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "https", "ssh", "git":
		return nil
	case "http", "svn", "bzr":
		if isLoopbackHost(u.Host) {
			return nil
		}
		return fmt.Errorf("scheme %s:// is only allowed for loopback hosts (got host %q)", u.Scheme, u.Host)
	case "":
		// Treated as a local path; #118 enforces workspace containment.
		return nil
	default:
		return fmt.Errorf("scheme %q is not allowed (allowed: https, ssh, git, http+loopback, svn+loopback, bzr+loopback)", u.Scheme)
	}
}

// isLoopbackHost reports whether host (without trailing :port) is a loopback
// address. Accepts "localhost", "127.0.0.0/8", "::1", and bracketed IPv6.
func isLoopbackHost(hostport string) bool {
	host := hostport
	// Strip port if present.
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

// isWindowsDrivePath reports whether s starts with a Windows drive-letter
// prefix (e.g. "C:\", "d:/", or bare "Z:"). Such strings are local
// filesystem paths, but net/url parses the leading letter as a URL scheme
// and the result drops into ValidateRepoURL's default-reject branch with a
// misleading "scheme c is not allowed" error.
//
// Detection is host-OS-agnostic on purpose: a Windows path string is the
// same shape whether the running process is on Windows or not, and gaal
// configs are sometimes authored on one OS and consumed on another.
func isWindowsDrivePath(s string) bool {
	if len(s) < 2 {
		return false
	}
	c := s[0]
	if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
		return false
	}
	if s[1] != ':' {
		return false
	}
	if len(s) == 2 {
		return true
	}
	return s[2] == '\\' || s[2] == '/'
}

// isSCPStyleGitURL detects the user@host:path syntax that git accepts in
// addition to URL forms. It is *not* a URL — net/url misparses it — so we
// match it heuristically: a single ":" with no "/" before it, and the part
// before "@" is non-empty.
func isSCPStyleGitURL(s string) bool {
	at := strings.Index(s, "@")
	if at <= 0 {
		return false
	}
	rest := s[at+1:]
	colon := strings.Index(rest, ":")
	if colon <= 0 {
		return false
	}
	// Reject if the colon comes after a slash (that's a URL like
	// "user@example.com/path:other", not SCP form).
	if strings.Contains(rest[:colon], "/") {
		return false
	}
	return true
}
