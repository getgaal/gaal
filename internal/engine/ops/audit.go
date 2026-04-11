package ops

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"

	"gaal/internal/core/agent"
	"gaal/internal/engine/render"
	"gaal/internal/mcp"
	"gaal/internal/skill"
)

// Audit discovers all skills and MCP servers installed on the machine and
// renders the result to stdout using the requested format.
func Audit(ctx context.Context, home, workDir string, format render.OutputFormat) error {
	slog.DebugContext(ctx, "starting audit", "home", home, "workDir", workDir)

	skills, err := collectAuditSkills(ctx, home, workDir)
	if err != nil {
		return err
	}
	mcps, err := collectAuditMCPs(ctx, home)
	if err != nil {
		return err
	}

	report := &render.AuditReport{Skills: skills, MCPs: mcps}
	return render.NewAuditRenderer(format).Render(os.Stdout, report)
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
func collectAuditSkills(ctx context.Context, home, workDir string) ([]render.AuditSkillEntry, error) {
	slog.DebugContext(ctx, "collecting audit skills")

	var entries []render.AuditSkillEntry
	seenDirs := map[string]struct{}{}

	agents := agent.List()

	// ── Pass 1: canonical dirs ───────────────────────────────────────────────
	for _, a := range agents {
		name := a.Name

		if a.Info.ProjectSkillsDir != "" {
			absDir := filepath.Join(workDir, a.Info.ProjectSkillsDir)
			metas, err := scanDeduped(absDir, seenDirs)
			if err != nil {
				slog.DebugContext(ctx, "canonical project scan error", "agent", name, "dir", absDir, "err", err)
			} else {
				for _, m := range metas {
					entries = append(entries, render.AuditSkillEntry{
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
			absDir := agent.ExpandHome(a.Info.GlobalSkillsDir, home)
			metas, err := scanDeduped(absDir, seenDirs)
			if err != nil {
				slog.DebugContext(ctx, "canonical global scan error", "agent", name, "dir", absDir, "err", err)
			} else {
				for _, m := range metas {
					entries = append(entries, render.AuditSkillEntry{
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
			absDir := filepath.Join(workDir, relDir)
			metas, err := scanDeduped(absDir, seenDirs)
			if err != nil {
				slog.DebugContext(ctx, "project scan error", "agent", name, "dir", absDir, "err", err)
				continue
			}
			for _, m := range metas {
				entries = append(entries, render.AuditSkillEntry{
					Name:   m.Name,
					Desc:   m.Desc,
					Agent:  name,
					Source: "project",
					Path:   m.Path,
				})
			}
		}

		// ── Global skills (1 level) ─────────────────────────────────────────
		for _, absDir := range agent.ExpandedGlobalSkillsSearch(name, home) {
			metas, err := scanDeduped(absDir, seenDirs)
			if err != nil {
				slog.DebugContext(ctx, "global scan error", "agent", name, "dir", absDir, "err", err)
				continue
			}
			for _, m := range metas {
				entries = append(entries, render.AuditSkillEntry{
					Name:   m.Name,
					Desc:   m.Desc,
					Agent:  name,
					Source: "global",
					Path:   m.Path,
				})
			}
		}

		// ── Package-manager skills (recursive) ──────────────────────────────
		for _, pmRoot := range agent.ExpandedPmSkillsSearch(name, home) {
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
					entries = append(entries, render.AuditSkillEntry{
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
func collectAuditMCPs(ctx context.Context, home string) ([]render.AuditMCPEntry, error) {
	slog.DebugContext(ctx, "collecting audit mcps")

	var entries []render.AuditMCPEntry

	for _, a := range agent.List() {
		cfgFile, ok := agent.ProjectMCPConfigPath(a.Name, home)
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

		entries = append(entries, render.AuditMCPEntry{
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
