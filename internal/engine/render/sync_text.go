package render

import (
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// RenderSyncSummary writes the compact per-resource sync summary documented
// at https://docs.getgaal.com/cli/sync. Each line carries a ✓ (or ✗ on
// error) marker, the resource name padded to the longest in the summary,
// and a past-tense action description derived from the pre-sync plan. A
// final "sync complete in <d>" line closes the output.
//
// Plan provides the "what sync did" verbs (cloned / updated / up to date /
// installed / upserted). Status provides the post-sync state used to pick
// the marker and — for skills — expand per-(source, agent) entries into
// per-skill-name rows with agent lists.
func RenderSyncSummary(w io.Writer, plan *PlanReport, status *StatusReport, duration time.Duration) error {
	slog.Debug("rendering sync summary")

	rows := collectSyncRows(plan, status)
	if len(rows) == 0 {
		fmt.Fprintln(w, "nothing to sync")
	} else {
		nameWidth := 0
		for _, r := range rows {
			if n := len(r.name); n > nameWidth {
				nameWidth = n
			}
		}
		for _, r := range rows {
			fmt.Fprintf(w, "%s %s  %s\n", r.marker, padText(r.name, nameWidth), r.detail)
		}
	}
	fmt.Fprintf(w, "sync complete in %s\n", duration.Round(time.Millisecond))
	return nil
}

type syncRow struct {
	marker string
	name   string
	detail string
	action PlanAction // retained so rows sort with no-ops at the bottom
}

func collectSyncRows(plan *PlanReport, status *StatusReport) []syncRow {
	if plan == nil {
		plan = &PlanReport{}
	}
	if status == nil {
		status = &StatusReport{}
	}

	// Each resource type is built and sorted independently so no-ops sink
	// to the bottom of their own group without interleaving across groups.
	repoRows := buildRepoSyncRows(plan, status)
	skillRows := buildSkillSyncRows(plan, status)
	mcpRows := buildMCPSyncRows(plan, status)

	sortByAction := func(rows []syncRow) {
		sort.SliceStable(rows, func(i, j int) bool {
			return actionRank(rows[i].action) < actionRank(rows[j].action)
		})
	}
	sortByAction(repoRows)
	sortByAction(skillRows)
	sortByAction(mcpRows)

	out := make([]syncRow, 0, len(repoRows)+len(skillRows)+len(mcpRows))
	out = append(out, repoRows...)
	out = append(out, skillRows...)
	out = append(out, mcpRows...)
	return out
}

func buildRepoSyncRows(plan *PlanReport, status *StatusReport) []syncRow {
	actions := make(map[string]PlanAction, len(plan.Repositories))
	errs := make(map[string]string, len(plan.Repositories))
	for _, r := range plan.Repositories {
		actions[r.Path] = r.Action
		errs[r.Path] = r.Error
	}
	entries := append([]RepoEntry(nil), status.Repositories...)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })

	rows := make([]syncRow, 0, len(entries))
	for _, e := range entries {
		action := actions[e.Path]
		if action == "" {
			action = PlanNoOp
		}
		rows = append(rows, syncRow{
			marker: summaryMarker(e.Status),
			name:   e.Path,
			detail: repoSyncDetail(action, e, errs[e.Path]),
			action: action,
		})
	}
	return rows
}

func buildSkillSyncRows(plan *PlanReport, status *StatusReport) []syncRow {
	skillActions := skillActionIndex(plan.Skills)
	aggregated := aggregateSkillsByName(status.Skills)
	rows := make([]syncRow, 0, len(aggregated))
	for _, s := range aggregated {
		action := skillActions[s.Name]
		if action == "" {
			action = PlanNoOp
		}
		rows = append(rows, syncRow{
			marker: summaryMarker(s.Status),
			name:   displayName(s.Name),
			detail: skillSyncDetail(action, s),
			action: action,
		})
	}
	return rows
}

func buildMCPSyncRows(plan *PlanReport, status *StatusReport) []syncRow {
	actions := make(map[string]PlanAction, len(plan.MCPs))
	errs := make(map[string]string, len(plan.MCPs))
	for _, m := range plan.MCPs {
		actions[m.Name] = m.Action
		errs[m.Name] = m.Error
	}
	entries := append([]MCPEntry(nil), status.MCPs...)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })

	rows := make([]syncRow, 0, len(entries))
	for _, e := range entries {
		action := actions[e.Name]
		if action == "" {
			action = PlanNoOp
		}
		rows = append(rows, syncRow{
			marker: summaryMarker(e.Status),
			name:   e.Name,
			detail: mcpSyncDetail(action, e, errs[e.Name]),
			action: action,
		})
	}
	return rows
}

// skillActionIndex flattens plan.Skills into a per-skill-name action lookup.
// A skill in Install -> PlanCreate; a skill in Update -> PlanUpdate (which
// overrides an earlier Create recorded from a different (source, agent)
// entry). Skills that appear only in no-op plan entries are absent from the
// index, which callers treat as PlanNoOp.
func skillActionIndex(entries []PlanSkillEntry) map[string]PlanAction {
	actions := make(map[string]PlanAction)
	for _, e := range entries {
		for _, n := range e.Install {
			if _, seen := actions[n]; !seen {
				actions[n] = PlanCreate
			}
		}
		for _, n := range e.Update {
			actions[n] = PlanUpdate
		}
	}
	return actions
}

func summaryMarker(s StatusCode) string {
	if s == StatusError {
		return "✗"
	}
	return "✓"
}

func repoSyncDetail(a PlanAction, e RepoEntry, planErr string) string {
	switch a {
	case PlanClone:
		return "cloned"
	case PlanUpdate:
		return "updated"
	case PlanError:
		return "error: " + firstNonEmpty(e.Error, planErr)
	}
	if e.Status == StatusError {
		return "error: " + firstNonEmpty(e.Error, planErr)
	}
	return "up to date"
}

func skillSyncDetail(a PlanAction, s aggregatedSkill) string {
	agents := "no agents"
	if len(s.Agents) > 0 {
		agents = strings.Join(s.Agents, ", ")
	}
	switch a {
	case PlanCreate:
		return "installed in " + agents
	case PlanUpdate:
		return "updated in " + agents
	case PlanError:
		return "error: " + s.Error
	}
	if s.Status == StatusError {
		return "error: " + s.Error
	}
	return "up to date in " + agents
}

func mcpSyncDetail(a PlanAction, e MCPEntry, planErr string) string {
	target := filepath.Base(e.Target)
	if target == "" {
		target = e.Target
	}
	switch a {
	case PlanCreate:
		return "upserted in " + target
	case PlanUpdate:
		return "updated in " + target
	case PlanError:
		return "error: " + firstNonEmpty(e.Error, planErr)
	}
	if e.Status == StatusError {
		return "error: " + firstNonEmpty(e.Error, planErr)
	}
	return "up to date in " + target
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
