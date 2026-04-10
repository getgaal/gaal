package vcs

import (
	"os"
	"path/filepath"
	"strings"
)

// DetectType infers the VCS type for a source URL or local filesystem path.
//
// For local paths it inspects the directory for well-known VCS markers
// (.git, .hg, .svn, .bzr) and defaults to "git" when none is found.
// For remote URLs it detects archive types by extension (.tar.gz / .tgz → "tar",
// .zip → "zip") and falls back to "git" for everything else.
func DetectType(source string) string {
	if isLocalFS(source) {
		return detectLocal(source)
	}
	return detectRemote(source)
}

// isLocalFS reports whether source refers to a local filesystem path rather
// than a remote URL. It covers POSIX and Windows absolute and relative paths,
// as well as the ~ home-dir shorthand.
func isLocalFS(source string) bool {
	if filepath.IsAbs(source) {
		return true
	}
	// Windows drive-letter absolute (e.g. C:\Users\foo or C:/Users/foo).
	if len(source) >= 3 && source[1] == ':' && (source[2] == '\\' || source[2] == '/') {
		return true
	}
	return strings.HasPrefix(source, "/") ||
		strings.HasPrefix(source, "./") ||
		strings.HasPrefix(source, `.\`) ||
		strings.HasPrefix(source, "../") ||
		strings.HasPrefix(source, `..\`) ||
		strings.HasPrefix(source, "~/") ||
		strings.HasPrefix(source, `~\`)
}

// detectLocal inspects dir for VCS markers and returns the matching type.
func detectLocal(dir string) string {
	// Expand ~ so os.Stat works correctly.
	if strings.HasPrefix(dir, "~/") || strings.HasPrefix(dir, `~\`) {
		// Best-effort: if home cannot be found, leave as-is; stat will fail
		// and we fall through to the git default.
		if home, err := os.UserHomeDir(); err == nil {
			dir = filepath.Join(home, dir[2:])
		}
	}
	for _, m := range []struct{ marker, vcsType string }{
		{".git", "git"},
		{".hg", "hg"},
		{".svn", "svn"},
		{".bzr", "bzr"},
	} {
		if _, err := os.Stat(filepath.Join(dir, m.marker)); err == nil {
			return m.vcsType
		}
	}
	return "git" // default: assume git for new directories
}

// detectRemote uses the URL suffix to distinguish archive from git sources.
func detectRemote(url string) string {
	lower := strings.ToLower(url)
	switch {
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return "tar"
	case strings.HasSuffix(lower, ".zip"):
		return "zip"
	default:
		return "git"
	}
}
