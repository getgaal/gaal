package hooks

import (
	"strings"

	"gaal/internal/engine/render"
)

// planEnv builds the map of GAAL_* variables describing what sync touched.
// Variables are intentionally newline-separated lists so shells can iterate
// them with `IFS=$'\n' for x in $GAAL_CHANGED_REPOS; do ...`. Pre-sync uses
// "PLANNED" framing; post-sync replays the same plan because plan == reality
// when the sync completed successfully, and re-collecting status here would
// require I/O the caller has already done.
//
// An empty plan still sets each variable to the empty string so hooks can
// detect "ran with no changes" by checking [[ -z "$GAAL_CHANGED_REPOS" ]].
func planEnv(plan *render.PlanReport) map[string]string {
	env := map[string]string{
		"GAAL_CHANGED_REPOS":  "",
		"GAAL_CHANGED_SKILLS": "",
		"GAAL_CHANGED_MCPS":   "",
		"GAAL_PLANNED_REPOS":  "",
		"GAAL_PLANNED_SKILLS": "",
		"GAAL_PLANNED_MCPS":   "",
		"GAAL_HAS_CHANGES":    "0",
		"GAAL_HAS_ERRORS":     "0",
	}
	if plan == nil {
		return env
	}
	if plan.HasChanges {
		env["GAAL_HAS_CHANGES"] = "1"
	}
	if plan.HasErrors {
		env["GAAL_HAS_ERRORS"] = "1"
	}

	repos := changedRepos(plan)
	skills := changedSkills(plan)
	mcps := changedMCPs(plan)

	joined := func(xs []string) string { return strings.Join(xs, "\n") }
	env["GAAL_CHANGED_REPOS"] = joined(repos)
	env["GAAL_CHANGED_SKILLS"] = joined(skills)
	env["GAAL_CHANGED_MCPS"] = joined(mcps)
	env["GAAL_PLANNED_REPOS"] = joined(repos)
	env["GAAL_PLANNED_SKILLS"] = joined(skills)
	env["GAAL_PLANNED_MCPS"] = joined(mcps)
	return env
}

func changedRepos(p *render.PlanReport) []string {
	out := make([]string, 0, len(p.Repositories))
	for _, e := range p.Repositories {
		if e.Action == render.PlanClone || e.Action == render.PlanUpdate {
			out = append(out, e.Path)
		}
	}
	return out
}

// changedSkills returns "agent:source" rows for every skill entry whose
// planned action is not a no-op or error. This format is unambiguous in a
// shell loop and stays stable across deduping in the plan layer.
func changedSkills(p *render.PlanReport) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(p.Skills))
	for _, e := range p.Skills {
		if e.Action != render.PlanCreate && e.Action != render.PlanUpdate {
			continue
		}
		key := e.Agent + ":" + e.Source
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}

func changedMCPs(p *render.PlanReport) []string {
	out := make([]string, 0, len(p.MCPs))
	for _, e := range p.MCPs {
		if e.Action == render.PlanCreate || e.Action == render.PlanUpdate {
			out = append(out, e.Name)
		}
	}
	return out
}
