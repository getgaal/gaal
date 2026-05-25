package vcs

import (
	"fmt"
	"net/url"
	"strings"
)

// RemoteURLMismatchError is returned by VcsGit.Update when the working copy's
// origin URL does not match the URL declared in gaal.yaml. gaal honors the
// existing remote (it never rewrites origin during sync); this error tells the
// user up-front that the two disagree, instead of leaking the low-level fetch
// failure that the protocol mismatch usually triggers (e.g. SSH agent missing
// when gaal.yaml is HTTPS but origin is SSH).
type RemoteURLMismatchError struct {
	Path          string // working copy path
	ConfiguredURL string // url: from gaal.yaml
	RemoteURL     string // origin URL inside the working copy
}

func (e *RemoteURLMismatchError) Error() string {
	return fmt.Sprintf(
		"gaal.yaml URL is %s but the remote at %s is %s; "+
			"either update the remote (`git remote set-url origin %s`) "+
			"or change `url:` in gaal.yaml to match (current remote: %s)",
		urlProtocolLabel(e.ConfiguredURL),
		e.Path,
		urlProtocolLabel(e.RemoteURL),
		e.ConfiguredURL,
		e.RemoteURL,
	)
}

// urlProtocolLabel returns a human-readable protocol label for the URL forms
// accepted under repositories.url. Used in the mismatch error message.
func urlProtocolLabel(s string) string {
	if s == "" {
		return "unknown"
	}
	if isSCPStyleGitURL(s) {
		return "SSH"
	}
	u, err := url.Parse(s)
	if err != nil || u.Scheme == "" {
		return "local path"
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return "HTTPS"
	case "http":
		return "HTTP"
	case "ssh":
		return "SSH"
	case "git":
		return "git://"
	case "file":
		return "file://"
	default:
		return u.Scheme + "://"
	}
}

// isSCPStyleGitURL detects the user@host:path form that git accepts in
// addition to URL forms. urlx has the same heuristic but keeping a local copy
// avoids widening urlx's API surface for this single internal call site.
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
	if strings.Contains(rest[:colon], "/") {
		return false
	}
	return true
}

// normalizeGitURL strips protocol, credentials, trailing ".git", and trailing
// slashes so two URLs pointing at the same logical repo compare equal.
//
// Host is lowercased (DNS is case-insensitive); path case is preserved
// (some forges treat owner/repo segments case-sensitively).
func normalizeGitURL(s string) string {
	if s == "" {
		return ""
	}
	if isSCPStyleGitURL(s) {
		at := strings.Index(s, "@")
		rest := s[at+1:]
		colon := strings.Index(rest, ":")
		host := strings.ToLower(rest[:colon])
		path := strings.TrimSuffix(strings.TrimSuffix(rest[colon+1:], "/"), ".git")
		return host + "/" + path
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return strings.TrimSuffix(strings.TrimSuffix(s, "/"), ".git")
	}
	path := strings.TrimPrefix(u.Path, "/")
	path = strings.TrimSuffix(strings.TrimSuffix(path, "/"), ".git")
	return strings.ToLower(u.Host) + "/" + path
}
