package render

import (
	"fmt"
	"io"
	"log/slog"
	"strings"
)

// planTextRenderer implements PlanRenderer producing a compact, line-per-item
// layout matching cli/sync.mdx:
//
//	✓ src/example          cloned
//	✓ code-review          installed in claude-code, cursor
//	✓ filesystem           upserted in claude_desktop_config.json
//	sync complete in 1.2s
//
// For dry-run mode the trailing summary line is replaced with a status hint.
type planTextRenderer struct{}

func (pr *planTextRenderer) Render(w io.Writer, r *PlanReport) error {
	slog.Debug("rendering plan text output")

	items := collectPlanItems(r)
	nameWidth := 0
	for _, it := range items {
		if n := len(it.name); n > nameWidth {
			nameWidth = n
		}
	}

	for _, it := range items {
		fmt.Fprintf(w, "%s %s  %s\n", it.marker, padText(it.name, nameWidth), it.detail)
	}

	if len(items) == 0 {
		fmt.Fprintln(w, "nothing to do")
		return nil
	}

	switch {
	case r.HasErrors:
		fmt.Fprintln(w, "plan completed with errors")
	case r.HasChanges:
		fmt.Fprintln(w, "changes pending — run `gaal sync` to apply")
	default:
		fmt.Fprintln(w, "everything up to date")
	}
	return nil
}

type planTextItem struct {
	marker string
	name   string
	detail string
}

func collectPlanItems(r *PlanReport) []planTextItem {
	items := make([]planTextItem, 0,
		len(r.Repositories)+len(r.Skills)+len(r.MCPs))

	for _, e := range r.Repositories {
		items = append(items, planTextItem{
			marker: markerFor(e.Action),
			name:   e.Path,
			detail: repoPlanDetail(e),
		})
	}
	for _, e := range r.Skills {
		items = append(items, planTextItem{
			marker: markerFor(e.Action),
			name:   displaySource(e.Source),
			detail: skillPlanDetail(e),
		})
	}
	for _, e := range r.MCPs {
		items = append(items, planTextItem{
			marker: markerFor(e.Action),
			name:   e.Name,
			detail: mcpPlanDetail(e),
		})
	}
	return items
}

func markerFor(a PlanAction) string {
	switch a {
	case PlanNoOp:
		return "✓"
	case PlanClone, PlanCreate:
		return "+"
	case PlanUpdate:
		return "~"
	case PlanError:
		return "✗"
	default:
		return "·"
	}
}

func repoPlanDetail(e PlanRepoEntry) string {
	switch e.Action {
	case PlanNoOp:
		if e.Current != "" {
			return "up to date (" + e.Current + ")"
		}
		return "up to date"
	case PlanClone:
		if e.URL != "" {
			return "would clone " + e.URL
		}
		return "would clone"
	case PlanUpdate:
		return "would update " + e.Current + " → " + e.Want
	case PlanError:
		return "error: " + e.Error
	default:
		return string(e.Action)
	}
}

func skillPlanDetail(e PlanSkillEntry) string {
	switch e.Action {
	case PlanNoOp:
		return "installed in " + e.Agent
	case PlanCreate:
		names := append([]string{}, e.Install...)
		names = append(names, e.Update...)
		if len(names) == 0 {
			return "would install in " + e.Agent
		}
		return "would install " + strings.Join(names, ", ") + " in " + e.Agent
	case PlanUpdate:
		return "would update " + strings.Join(e.Update, ", ") + " in " + e.Agent
	case PlanError:
		return "error: " + e.Error
	default:
		return string(e.Action)
	}
}

func mcpPlanDetail(e PlanMCPEntry) string {
	switch e.Action {
	case PlanNoOp:
		return "upserted in " + e.Target
	case PlanCreate:
		return "would upsert in " + e.Target
	case PlanUpdate:
		return "would update in " + e.Target
	case PlanError:
		return "error: " + e.Error
	default:
		return string(e.Action)
	}
}
