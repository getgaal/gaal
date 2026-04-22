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
}

func collectSyncRows(plan *PlanReport, status *StatusReport) []syncRow {
	if plan == nil {
		plan = &PlanReport{}
	}
	if status == nil {
		status = &StatusReport{}
	}

	var rows []syncRow

	// Repositories — one row per repo, alphabetised by path.
	repoActions := make(map[string]PlanAction, len(plan.Repositories))
	repoErrors := make(map[string]string, len(plan.Repositories))
	for _, r := range plan.Repositories {
		repoActions[r.Path] = r.Action
		repoErrors[r.Path] = r.Error
	}
	repoEntries := append([]RepoEntry(nil), status.Repositories...)
	sort.Slice(repoEntries, func(i, j int) bool { return repoEntries[i].Path < repoEntries[j].Path })
	for _, e := range repoEntries {
		action := repoActions[e.Path]
		if action == "" {
			action = PlanNoOp
		}
		rows = append(rows, syncRow{
			marker: summaryMarker(e.Status),
			name:   e.Path,
			detail: repoSyncDetail(action, e, repoErrors[e.Path]),
		})
	}

	// Skills — aggregate by name and pair with the plan's per-skill action.
	skillActions := skillActionIndex(plan.Skills)
	for _, s := range aggregateSkillsByName(status.Skills) {
		rows = append(rows, syncRow{
			marker: summaryMarker(s.Status),
			name:   displayName(s.Name),
			detail: skillSyncDetail(skillActions[s.Name], s),
		})
	}

	// MCPs — alphabetised by name, basename-only for the target path.
	mcpActions := make(map[string]PlanAction, len(plan.MCPs))
	mcpErrors := make(map[string]string, len(plan.MCPs))
	for _, m := range plan.MCPs {
		mcpActions[m.Name] = m.Action
		mcpErrors[m.Name] = m.Error
	}
	mcpEntries := append([]MCPEntry(nil), status.MCPs...)
	sort.Slice(mcpEntries, func(i, j int) bool { return mcpEntries[i].Name < mcpEntries[j].Name })
	for _, e := range mcpEntries {
		action := mcpActions[e.Name]
		if action == "" {
			action = PlanNoOp
		}
		rows = append(rows, syncRow{
			marker: summaryMarker(e.Status),
			name:   e.Name,
			detail: mcpSyncDetail(action, e, mcpErrors[e.Name]),
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
