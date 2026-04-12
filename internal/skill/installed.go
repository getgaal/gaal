package skill

import (
	"os"
	"path/filepath"
)

// IsAgentInstalled reports whether the directory that would own the agent's
// skills on this machine already exists. This checks the parent of the
// agent's skill dir — the same heuristic used by sync to avoid creating
// agent-owned directories as a side effect.
func IsAgentInstalled(name string, global bool, home, workDir string) bool {
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
