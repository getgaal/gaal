package skill

import (
	"os"
	"path/filepath"
)

// IsAgentInstalled reports whether the directory that would own the agent's
// skills on this machine already exists. This checks the parent of the
// agent's skill dir — the same heuristic used by sync to avoid creating
// agent-owned directories as a side effect.
//
// Special case: claude-desktop has supports_generic_global=true, which
// means its SkillDir resolves to ~/.agents/skills — a generic directory
// shared with several other agents. The default check would then mark
// claude-desktop as installed any time *any* generic-supporting agent is
// installed. Detect via its app-specific config path instead (#128).
func IsAgentInstalled(name string, global bool, home, workDir string) bool {
	if name == "claude-desktop" {
		return isClaudeDesktopInstalled(home)
	}
	dir, ok := SkillDir(name, global, home)
	if !ok {
		return false
	}
	var checkDir string
	if !global && !filepath.IsAbs(dir) {
		checkDir = filepath.Join(workDir, filepath.Dir(dir))
	} else {
		checkDir = filepath.Dir(expandHome(dir, home))
	}
	_, err := os.Stat(checkDir)
	return err == nil
}

// isClaudeDesktopInstalled checks the canonical macOS / Windows install
// locations of the Claude Desktop GUI app. (Linux is not officially
// supported; the warning in mcp.warnClaudeDesktopOnLinux already covers
// the no-op case there.)
func isClaudeDesktopInstalled(home string) bool {
	candidates := []string{
		// macOS canonical
		filepath.Join(home, "Library", "Application Support", "Claude"),
		// Windows: %APPDATA%\Claude — APPDATA's value lives outside `home`,
		// but for users who set HOME on Windows or whose APPDATA points
		// inside home (sandbox e2e), this catches it. Real Windows
		// detection should consult APPDATA directly; that's a runtime
		// concern outside this helper.
		filepath.Join(home, "AppData", "Roaming", "Claude"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}
