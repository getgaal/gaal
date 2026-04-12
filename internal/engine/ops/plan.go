package ops

import (
	"context"
	"log/slog"
	"os"

	"gaal/internal/engine/render"
	"gaal/internal/mcp"
	"gaal/internal/repo"
	"gaal/internal/skill"
)

// SyncPlan computes what a sync would do without performing any writes.
// The returned PlanReport describes every action that RunOnce would take.
// This is the shared planner used by both `sync --dry-run` and the future
// `diff` command.
func SyncPlan(ctx context.Context, repos *repo.Manager, skills *skill.Manager, mcps *mcp.Manager) (*render.PlanReport, error) {
	slog.DebugContext(ctx, "computing sync plan")

	report, err := Collect(ctx, repos, skills, mcps)
	if err != nil {
		return nil, err
	}

	plan := &render.PlanReport{}

	plan.Repositories = planRepos(report.Repositories)
	plan.Skills = planSkills(report.Skills)
	plan.MCPs = planMCPs(report.MCPs)

	for _, r := range plan.Repositories {
		if r.Action == render.PlanError {
			plan.HasErrors = true
		}
		if r.Action != render.PlanNoOp && r.Action != render.PlanError {
			plan.HasChanges = true
		}
	}
	for _, s := range plan.Skills {
		if s.Action == render.PlanError {
			plan.HasErrors = true
		}
		if s.Action != render.PlanNoOp && s.Action != render.PlanError {
			plan.HasChanges = true
		}
	}
	for _, m := range plan.MCPs {
		if m.Action == render.PlanError {
			plan.HasErrors = true
		}
		if m.Action != render.PlanNoOp && m.Action != render.PlanError {
			plan.HasChanges = true
		}
	}

	return plan, nil
}

// RenderPlan computes and renders the sync plan to stdout.
func RenderPlan(ctx context.Context, repos *repo.Manager, skills *skill.Manager, mcps *mcp.Manager, format render.OutputFormat) (*render.PlanReport, error) {
	plan, err := SyncPlan(ctx, repos, skills, mcps)
	if err != nil {
		return nil, err
	}

	renderer, err := render.NewPlanRenderer(format)
	if err != nil {
		return nil, err
	}

	if err := renderer.Render(os.Stdout, plan); err != nil {
		return nil, err
	}

	return plan, nil
}

func planRepos(entries []render.RepoEntry) []render.PlanRepoEntry {
	out := make([]render.PlanRepoEntry, 0, len(entries))
	for _, e := range entries {
		p := render.PlanRepoEntry{
			Path:    e.Path,
			Type:    e.Type,
			URL:     e.URL,
			Current: e.Current,
			Want:    e.Want,
		}
		switch e.Status {
		case render.StatusError:
			p.Action = render.PlanError
			p.Error = e.Error
		case render.StatusNotCloned:
			p.Action = render.PlanClone
		case render.StatusDirty:
			p.Action = render.PlanUpdate
		default:
			p.Action = render.PlanNoOp
		}
		out = append(out, p)
	}
	return out
}

func planSkills(entries []render.SkillEntry) []render.PlanSkillEntry {
	out := make([]render.PlanSkillEntry, 0, len(entries))
	for _, e := range entries {
		p := render.PlanSkillEntry{
			Source: e.Source,
			Agent:  e.Agent,
		}
		switch e.Status {
		case render.StatusError:
			p.Action = render.PlanError
			p.Error = e.Error
		case render.StatusPartial:
			p.Action = render.PlanCreate
			p.Install = e.Missing
			if len(e.Modified) > 0 {
				p.Update = e.Modified
			}
		case render.StatusDirty:
			p.Action = render.PlanUpdate
			p.Update = e.Modified
		default:
			p.Action = render.PlanNoOp
		}
		out = append(out, p)
	}
	return out
}

func planMCPs(entries []render.MCPEntry) []render.PlanMCPEntry {
	out := make([]render.PlanMCPEntry, 0, len(entries))
	for _, e := range entries {
		p := render.PlanMCPEntry{
			Name:   e.Name,
			Target: e.Target,
		}
		switch e.Status {
		case render.StatusError:
			p.Action = render.PlanError
			p.Error = e.Error
		case render.StatusAbsent:
			p.Action = render.PlanCreate
		case render.StatusDirty:
			p.Action = render.PlanUpdate
		default:
			p.Action = render.PlanNoOp
		}
		out = append(out, p)
	}
	return out
}
