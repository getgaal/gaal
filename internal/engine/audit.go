package engine

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"

	"gaal/internal/core/agent"
	"gaal/internal/mcp"
	"gaal/internal/skill"
)

// AuditSkillEntry holds the metadata of a single skill discovered during audit.
type AuditSkillEntry struct {
	Name  string `json:"name"`
	Desc  string `json:"desc,omitempty"`
	Agent string `json:"agent"`
	// Source is one of "project", "global", or "package-manager".
	Source string `json:"source"`
	Path   string `json:"path"`
}

// AuditMCPEntry holds the MCP servers found for a single agent.
type AuditMCPEntry struct {
	Agent      string   `json:"agent"`
	ConfigFile string   `json:"config_file"`
	Servers    []string `json:"servers"`
}

// AuditReport aggregates all skills and MCP servers discovered on the machine.
type AuditReport struct {
	Skills []AuditSkillEntry `json:"skills"`
	MCPs   []AuditMCPEntry   `json:"mcps"`
}

// Audit discovers all skills and MCP servers installed on the machine and
// renders the result to stdout using the requested format.
func (e *Engine) Audit(ctx context.Context, format OutputFormat) error {
	slog.DebugContext(ctx, "starting audit", "home", e.home, "workDir", e.workDir)

	skills, err := e.collectAuditSkills(ctx)
	if err != nil {
		return err
	}
	mcps, err := e.collectAuditMCPs(ctx)
	if err != nil {
		return err
	}

	report := &AuditReport{Skills: skills, MCPs: mcps}
	return NewAuditRenderer(format).Render(os.Stdout, report)
}

// collectAuditSkills iterates every registered agent and scans its project,
// global, and package-manager skill directories.
//
// Attribution uses a two-pass strategy to ensure that shared directories (e.g.
// ~/.copilot/skills) are attributed to the agent that canonically owns them
// rather than the first agent alphabetically:
//
//   - Pass 1 scans each agent's canonical dirs only (ProjectSkillsDir /
//     GlobalSkillsDir).  This builds the "seen" set with correct ownership.
//   - Pass 2 scans the full search lists of each agent.  Directories already
//     claimed in pass 1 are skipped, so only unclaimed shared dirs get a new
//     attribution at this stage.
func (e *Engine) collectAuditSkills(ctx context.Context) ([]AuditSkillEntry, error) {
	slog.DebugContext(ctx, "collecting audit skills")

	var entries []AuditSkillEntry
	seenDirs := map[string]struct{}{}

	agents := agent.List()

	// ── Pass 1: canonical dirs ───────────────────────────────────────────────
	for _, a := range agents {
		name := a.Name

		if a.Info.ProjectSkillsDir != "" {
			absDir := filepath.Join(e.workDir, a.Info.ProjectSkillsDir)
			metas, err := scanDeduped(absDir, seenDirs)
			if err != nil {
				slog.DebugContext(ctx, "canonical project scan error", "agent", name, "dir", absDir, "err", err)
			} else {
				for _, m := range metas {
					entries = append(entries, AuditSkillEntry{
						Name:   m.Name,
						Desc:   m.Desc,
						Agent:  name,
						Source: "project",
						Path:   m.Path,
					})
				}
			}
		}

		if a.Info.GlobalSkillsDir != "" {
			absDir := agent.ExpandHome(a.Info.GlobalSkillsDir, e.home)
			metas, err := scanDeduped(absDir, seenDirs)
			if err != nil {
				slog.DebugContext(ctx, "canonical global scan error", "agent", name, "dir", absDir, "err", err)
			} else {
				for _, m := range metas {
					entries = append(entries, AuditSkillEntry{
						Name:   m.Name,
						Desc:   m.Desc,
						Agent:  name,
						Source: "global",
						Path:   m.Path,
					})
				}
			}
		}
	}

	// ── Pass 2: full search lists (extended / shared dirs) ───────────────────
	for _, a := range agents {
		name := a.Name

		// ── Project skills (1 level) ────────────────────────────────────────
		for _, relDir := range agent.ExpandedProjectSkillsSearch(name) {
			absDir := filepath.Join(e.workDir, relDir)
			metas, err := scanDeduped(absDir, seenDirs)
			if err != nil {
				slog.DebugContext(ctx, "project scan error", "agent", name, "dir", absDir, "err", err)
				continue
			}
			for _, m := range metas {
				entries = append(entries, AuditSkillEntry{
					Name:   m.Name,
					Desc:   m.Desc,
					Agent:  name,
					Source: "project",
					Path:   m.Path,
				})
			}
		}

		// ── Global skills (1 level) ─────────────────────────────────────────
		for _, absDir := range agent.ExpandedGlobalSkillsSearch(name, e.home) {
			metas, err := scanDeduped(absDir, seenDirs)
			if err != nil {
				slog.DebugContext(ctx, "global scan error", "agent", name, "dir", absDir, "err", err)
				continue
			}
			for _, m := range metas {
				entries = append(entries, AuditSkillEntry{
					Name:   m.Name,
					Desc:   m.Desc,
					Agent:  name,
					Source: "global",
					Path:   m.Path,
				})
			}
		}

		// ── Package-manager skills (recursive) ──────────────────────────────
		for _, pmRoot := range agent.ExpandedPmSkillsSearch(name, e.home) {
			skillsDirs, err := skill.WalkForSkillDirs(pmRoot)
			if err != nil {
				slog.DebugContext(ctx, "pm walk error", "agent", name, "root", pmRoot, "err", err)
				continue
			}
			for _, sd := range skillsDirs {
				metas, err := scanDeduped(sd, seenDirs)
				if err != nil {
					slog.DebugContext(ctx, "pm scan error", "agent", name, "dir", sd, "err", err)
					continue
				}
				for _, m := range metas {
					entries = append(entries, AuditSkillEntry{
						Name:   m.Name,
						Desc:   m.Desc,
						Agent:  name,
						Source: "package-manager",
						Path:   m.Path,
					})
				}
			}
		}
	}

	return entries, nil
}

// collectAuditMCPs reads the project_mcp_config_file of every registered agent
// and returns agents that actually have servers configured.
func (e *Engine) collectAuditMCPs(ctx context.Context) ([]AuditMCPEntry, error) {
	slog.DebugContext(ctx, "collecting audit mcps")

	var entries []AuditMCPEntry

	for _, a := range agent.List() {
		cfgFile, ok := agent.ProjectMCPConfigPath(a.Name, e.home)
		if !ok {
			continue
		}

		servers, err := mcp.ListServers(cfgFile)
		if err != nil {
			slog.DebugContext(ctx, "mcp list error", "agent", a.Name, "file", cfgFile, "err", err)
			continue
		}
		if len(servers) == 0 {
			continue
		}

		entries = append(entries, AuditMCPEntry{
			Agent:      a.Name,
			ConfigFile: cfgFile,
			Servers:    servers,
		})
	}

	return entries, nil
}

// scanDeduped calls skill.ScanDir and skips any skill directory already seen.
func scanDeduped(dir string, seen map[string]struct{}) ([]skill.Meta, error) {
	metas, err := skill.ScanDir(dir)
	if err != nil {
		return nil, err
	}
	var out []skill.Meta
	for _, m := range metas {
		if _, ok := seen[m.Path]; ok {
			continue
		}
		seen[m.Path] = struct{}{}
		out = append(out, m)
	}
	return out, nil
}
