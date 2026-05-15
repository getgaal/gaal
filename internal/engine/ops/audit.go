package ops

import (
	"context"
	"log/slog"
	"os"

	"gaal/internal/discover"
	"gaal/internal/engine/render"
	"gaal/internal/mcp"
)

// Audit discovers all skills and MCP servers installed on the machine and
// renders the result to stdout using the requested format.
func Audit(ctx context.Context, home, workDir string, format render.OutputFormat) error {
	slog.DebugContext(ctx, "starting audit", "home", home, "workDir", workDir)

	resources, err := discover.Scan(ctx, home, workDir, discover.ScanOptions{
		Mode:             discover.ScanModeFull,
		IncludeWorkspace: true,
	})
	if err != nil {
		slog.DebugContext(ctx, "scan error during audit", "err", err)
	}

	skills := resourcesToAuditSkills(resources)
	mcps, err := resourcesToAuditMCPs(ctx, resources)
	if err != nil {
		return err
	}

	report := &render.AuditReport{Home: home, Skills: skills, MCPs: mcps}
	return render.NewAuditRenderer(format).Render(os.Stdout, report)
}

// resourcesToAuditSkills converts discover.Resource skill entries to render entries.
func resourcesToAuditSkills(resources []discover.Resource) []render.AuditSkillEntry {
	var entries []render.AuditSkillEntry
	for _, r := range resources {
		if r.Type != discover.ResourceSkill {
			continue
		}
		entries = append(entries, render.AuditSkillEntry{
			Name:   r.Name,
			Desc:   r.Meta["desc"],
			Agent:  r.Meta["agent"],
			Source: r.Meta["source"],
			Path:   r.Path,
		})
	}
	return entries
}

// resourcesToAuditMCPs converts discover.Resource MCP entries to render entries,
// loading the server list from each config file.
func resourcesToAuditMCPs(ctx context.Context, resources []discover.Resource) ([]render.AuditMCPEntry, error) {
	var entries []render.AuditMCPEntry
	for _, r := range resources {
		if r.Type != discover.ResourceMCP {
			continue
		}
		servers, err := mcp.ListServers(r.Path)
		if err != nil {
			slog.DebugContext(ctx, "mcp list error", "agent", r.Name, "file", r.Path, "err", err)
			continue
		}
		if len(servers) == 0 {
			continue
		}
		entries = append(entries, render.AuditMCPEntry{
			Agent:      r.Name,
			ConfigFile: r.Path,
			Scope:      r.Meta["scope"],
			Servers:    servers,
		})
	}
	return entries, nil
}
