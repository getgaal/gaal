package render

import (
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
)

// summaryPlanRenderer implements PlanRenderer producing a compact one-line-per-
// change plan for `gaal sync --dry-run`. Changed resources get individual rows
// with +/~/! markers; unchanged resources collapse into a single summary line.
//
// Example output:
//
//	Plan:
//	  + clone   src/example
//	  + install code-review, refactor (2 skills)
//	  ~ update  memory-mcp config
//	  = 10 skills, 3 MCP servers unchanged
//
//	Run gaal sync to apply.
type summaryPlanRenderer struct{}

// planSummaryRow is a single rendered row in the compact plan output.
type planSummaryRow struct {
	marker string
	verb   string
	name   string
	action PlanAction
}

func (r *summaryPlanRenderer) Render(w io.Writer, report *PlanReport) error {
	slog.Debug("rendering summary plan output")

	if len(report.Repositories) == 0 && len(report.Skills) == 0 && len(report.MCPs) == 0 {
		fmt.Fprintln(w, "nothing to do")
		return nil
	}

	rows, noopCount := buildSummaryPlanRows(report)

	fmt.Fprintln(w, "Plan:")

	// Compute verb width for alignment across all changed rows.
	verbWidth := 0
	for _, row := range rows {
		if n := len(row.verb); n > verbWidth {
			verbWidth = n
		}
	}

	for _, row := range rows {
		fmt.Fprintf(w, "  %s %s  %s\n", row.marker, padText(row.verb, verbWidth), row.name)
	}

	if noopCount > 0 {
		// Pad the "=" line to match verb alignment of changed rows.
		marker := "="
		verb := padText("", verbWidth)
		_ = verb
		fmt.Fprintf(w, "  %s %s  %d %s unchanged\n", marker, padText("", verbWidth), noopCount, resourceWord(noopCount))
	}

	fmt.Fprintln(w)
	switch {
	case report.HasErrors:
		fmt.Fprintln(w, "Plan completed with errors.")
	case report.HasChanges:
		fmt.Fprintln(w, "Run gaal sync to apply.")
	default:
		fmt.Fprintln(w, "Everything up to date.")
	}
	return nil
}

// buildSummaryPlanRows builds the list of changed rows and counts no-ops.
// Skills are aggregated by name across (source, agent) entries.
func buildSummaryPlanRows(report *PlanReport) (rows []planSummaryRow, noopCount int) {
	// --- Repositories ---
	for _, e := range report.Repositories {
		switch e.Action {
		case PlanNoOp:
			noopCount++
		default:
			rows = append(rows, planSummaryRow{
				marker: planMarker(e.Action),
				verb:   planVerb(e.Action, "repo"),
				name:   e.Path,
				action: e.Action,
			})
		}
	}

	// --- Skills ---
	// Aggregate by name using skillActionIndex for changed skills; count no-ops.
	skillActions := skillActionIndex(report.Skills)
	noopSkillNames := collectNoOpSkillNames(report.Skills)

	// Build install/update groups: name -> action
	type skillGroup struct {
		names  []string
		action PlanAction
	}
	// Collect ordered unique changed skill names.
	seen := map[string]bool{}
	var installNames, updateNames []string
	for _, e := range report.Skills {
		for _, n := range e.Install {
			if !seen[n] {
				seen[n] = true
				if skillActions[n] == PlanCreate {
					installNames = append(installNames, n)
				}
			}
		}
		for _, n := range e.Update {
			if !seen[n] {
				seen[n] = true
				if skillActions[n] == PlanUpdate {
					updateNames = append(updateNames, n)
				}
			}
		}
	}

	if len(installNames) > 0 {
		rows = append(rows, planSummaryRow{
			marker: planMarker(PlanCreate),
			verb:   planVerb(PlanCreate, "skill"),
			name:   skillListName(installNames),
			action: PlanCreate,
		})
	}
	if len(updateNames) > 0 {
		rows = append(rows, planSummaryRow{
			marker: planMarker(PlanUpdate),
			verb:   planVerb(PlanUpdate, "skill"),
			name:   skillListName(updateNames),
			action: PlanUpdate,
		})
	}
	// Handle error entries
	for _, e := range report.Skills {
		if e.Action == PlanError {
			rows = append(rows, planSummaryRow{
				marker: planMarker(PlanError),
				verb:   planVerb(PlanError, "skill"),
				name:   displaySkillName(e.Source),
				action: PlanError,
			})
		}
	}
	noopCount += len(noopSkillNames)

	// --- MCPs ---
	for _, e := range report.MCPs {
		switch e.Action {
		case PlanNoOp:
			noopCount++
		default:
			name := e.Name
			if e.Action != PlanError && e.Target != "" {
				name = e.Name + " config"
			}
			rows = append(rows, planSummaryRow{
				marker: planMarker(e.Action),
				verb:   planVerb(e.Action, "mcp"),
				name:   name,
				action: e.Action,
			})
		}
	}

	// Sort: errors first, then changes, no-ops are collapsed already.
	sort.SliceStable(rows, func(i, j int) bool {
		return actionRank(rows[i].action) < actionRank(rows[j].action)
	})

	return rows, noopCount
}

// collectNoOpSkillNames returns the deduplicated set of skill names that appear
// only in NoOp entries (i.e. not in Install or Update of any entry).
func collectNoOpSkillNames(entries []PlanSkillEntry) []string {
	changed := map[string]bool{}
	for _, e := range entries {
		for _, n := range e.Install {
			changed[n] = true
		}
		for _, n := range e.Update {
			changed[n] = true
		}
	}
	seen := map[string]bool{}
	var out []string
	for _, e := range entries {
		for _, n := range e.NoOp {
			if !changed[n] && !seen[n] {
				seen[n] = true
				out = append(out, n)
			}
		}
	}
	return out
}

// skillListName formats a list of skill names for display:
// a single name is shown as-is; multiple names are comma-joined with a
// "(N skills)" count suffix.
func skillListName(names []string) string {
	if len(names) == 1 {
		return names[0]
	}
	return strings.Join(names, ", ") + " (" + pluralise(len(names), "skill", "skills") + ")"
}

// resourceWord returns "resource" or "resources" for a count of mixed unchanged items.
func resourceWord(n int) string {
	if n == 1 {
		return "resource"
	}
	return "resources"
}

// pluralise returns "N one" when n==1, else "N many".
func pluralise(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}
