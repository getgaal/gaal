// Package urlx provides URL utilities, including credential redaction for
// safe logging and error messages.
package urlx

import (
	"log/slog"
	"net/url"
)

// Redact returns rawurl with any embedded userinfo (user[:password]) stripped.
// If rawurl is empty or not a parseable URL, it is returned unchanged.
//
// Use this whenever a URL flows into a log line, error message, telemetry
// payload, or any other persisted/transmitted surface — per the AGENTS.md
// rule "do not log secrets or credentials."
func Redact(rawurl string) string {
	if rawurl == "" {
		return rawurl
	}
	u, err := url.Parse(rawurl)
	if err != nil || u.User == nil {
		return rawurl
	}
	u.User = nil
	return u.String()
}

// SlogURL returns a slog.Attr keyed "url" with the redacted form of rawurl.
// Convenience wrapper for the common slog.DebugContext(ctx, "...", "url", url) pattern.
func SlogURL(rawurl string) slog.Attr {
	return slog.String("url", Redact(rawurl))
}
