package render

import (
	"fmt"
	"io"
	"log/slog"
	"strings"
)

// planTextRenderer implements PlanRenderer producing a nested, indented
// plan for `gaal sync --dry-run`, matching the format documented at
// https://docs.getgaal.com/cli/sync:
//
//	plan:
//	  repositories
//	    + clone     src/example          (git, main)
//	    ~ update    src/gaal             (git, main → main, 2 commits ahead)
//	    = unchanged src/dataset
//	  skills
//	    + install   code-review          → claude-code, cursor
//	    = unchanged refactor             → claude-code
//	  mcps
//	    + upsert    filesystem           → ~/.config/claude/claude_desktop_config.json
//
// Verb and name column widths are computed per-section so each block stays
// tight. Skill names use [displaySkillName]; MCP targets use [displayTarget]
// with a soft length cap.
type planTextRenderer struct{}

// planTargetSoftLimit is the maximum visual length of an MCP target before
// it gets truncated to "…/<last three segments>" in the plan renderer.
const planTargetSoftLimit = 60

func (pr *planTextRenderer) Render(w io.Writer, r *PlanReport) error {
	slog.Debug("rendering plan text output")

	if len(r.Repositories) == 0 && len(r.Skills) == 0 && len(r.MCPs) == 0 {
		fmt.Fprintln(w, "nothing to do")
		return nil
	}

	fmt.Fprintln(w, "plan:")
	pr.writeRepos(w, r.Repositories)
	pr.writeSkills(w, r.Skills)
	pr.writeMCPs(w, r.MCPs)

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

// planRow captures one rendered row: marker, verb, name, and an optional
// post-name detail clause. All strings are pre-formatted for display.
type planRow struct {
	marker string
	verb   string
	name   string
	detail string
}

// writeSection prints a section header and its rows with per-section
// alignment: each verb is padded to the longest verb length in the section,
// and each name to the longest display-name length.
func (pr *planTextRenderer) writeSection(w io.Writer, title string, rows []planRow) {
	if len(rows) == 0 {
		return
	}
	fmt.Fprintf(w, "  %s\n", title)

	verbWidth, nameWidth := 0, 0
	for _, r := range rows {
		if n := len(r.verb); n > verbWidth {
			verbWidth = n
		}
		if n := len(r.name); n > nameWidth {
			nameWidth = n
		}
	}

	for _, r := range rows {
		line := fmt.Sprintf("    %s %s %s", r.marker, padText(r.verb, verbWidth), padText(r.name, nameWidth))
		if r.detail != "" {
			line += "  " + r.detail
		}
		fmt.Fprintln(w, strings.TrimRight(line, " "))
	}
}

func (pr *planTextRenderer) writeRepos(w io.Writer, entries []PlanRepoEntry) {
	if len(entries) == 0 {
		return
	}
	rows := make([]planRow, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, planRow{
			marker: planMarker(e.Action),
			verb:   planVerb(e.Action, "repo"),
			name:   e.Path,
			detail: repoPlanDetail(e),
		})
	}
	pr.writeSection(w, "repositories", rows)
}

func (pr *planTextRenderer) writeSkills(w io.Writer, entries []PlanSkillEntry) {
	if len(entries) == 0 {
		return
	}
	rows := buildSkillPlanRows(entries)
	if len(rows) == 0 {
		return
	}
	pr.writeSection(w, "skills", rows)
}

// buildSkillPlanRows pivots plan.Skills (keyed by source+agent) into one row
// per skill name so the user sees "code-review → claude-code, cursor"
// instead of two source-level lines. PlanError entries stay ungrouped since
// no per-skill-name information is available.
func buildSkillPlanRows(entries []PlanSkillEntry) []planRow {
	type key struct {
		name   string
		action PlanAction
	}
	type bucket struct {
		agents  []string
		seen    map[string]struct{}
		updates []string // e.g. "v1 → v2" strings accumulated for PlanUpdate
	}
	byKey := map[key]*bucket{}
	var order []key

	add := func(k key, agent string) {
		b, ok := byKey[k]
		if !ok {
			b = &bucket{seen: map[string]struct{}{}}
			byKey[k] = b
			order = append(order, k)
		}
		if agent == "" {
			return
		}
		if _, dup := b.seen[agent]; dup {
			return
		}
		b.seen[agent] = struct{}{}
		b.agents = append(b.agents, agent)
	}

	rows := make([]planRow, 0, len(entries))
	for _, e := range entries {
		switch e.Action {
		case PlanError:
			rows = append(rows, planRow{
				marker: planMarker(PlanError),
				verb:   planVerb(PlanError, "skill"),
				name:   displaySkillName(e.Source),
				detail: "(" + e.Error + ")",
			})
			continue
		case PlanCreate:
			for _, n := range e.Install {
				add(key{n, PlanCreate}, e.Agent)
			}
			for _, n := range e.Update {
				add(key{n, PlanUpdate}, e.Agent)
			}
		case PlanUpdate:
			for _, n := range e.Update {
				add(key{n, PlanUpdate}, e.Agent)
			}
		case PlanNoOp:
			for _, n := range e.NoOp {
				add(key{n, PlanNoOp}, e.Agent)
			}
		}
	}

	for _, k := range order {
		b := byKey[k]
		detail := ""
		if len(b.agents) > 0 {
			detail = "→ " + strings.Join(b.agents, ", ")
		}
		rows = append(rows, planRow{
			marker: planMarker(k.action),
			verb:   planVerb(k.action, "skill"),
			name:   k.name,
			detail: detail,
		})
	}
	return rows
}

func (pr *planTextRenderer) writeMCPs(w io.Writer, entries []PlanMCPEntry) {
	if len(entries) == 0 {
		return
	}
	rows := make([]planRow, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, planRow{
			marker: planMarker(e.Action),
			verb:   planVerb(e.Action, "mcp"),
			name:   e.Name,
			detail: mcpPlanDetail(e),
		})
	}
	pr.writeSection(w, "mcps", rows)
}

// planMarker returns the one-character leading glyph for a plan action.
func planMarker(a PlanAction) string {
	switch a {
	case PlanNoOp:
		return "="
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

// planVerb returns the action verb for a plan row, specialised per resource
// kind so repositories say "clone", skills say "install", and MCPs say
// "upsert" for the create action.
func planVerb(a PlanAction, kind string) string {
	if a == PlanNoOp {
		return "unchanged"
	}
	switch kind {
	case "repo":
		switch a {
		case PlanClone:
			return "clone"
		case PlanUpdate:
			return "update"
		case PlanError:
			return "error"
		}
	case "skill":
		switch a {
		case PlanCreate:
			return "install"
		case PlanUpdate:
			return "update"
		case PlanError:
			return "error"
		}
	case "mcp":
		switch a {
		case PlanCreate:
			return "upsert"
		case PlanUpdate:
			return "update"
		case PlanError:
			return "error"
		}
	}
	return string(a)
}

func repoPlanDetail(e PlanRepoEntry) string {
	switch e.Action {
	case PlanClone:
		parts := []string{}
		if e.Type != "" {
			parts = append(parts, e.Type)
		}
		if e.Want != "" {
			parts = append(parts, e.Want)
		} else if e.URL != "" {
			parts = append(parts, e.URL)
		}
		if len(parts) == 0 {
			return ""
		}
		return "(" + strings.Join(parts, ", ") + ")"
	case PlanUpdate:
		parts := []string{}
		if e.Type != "" {
			parts = append(parts, e.Type)
		}
		if e.Current != "" || e.Want != "" {
			parts = append(parts, e.Current+" → "+e.Want)
		}
		if len(parts) == 0 {
			return ""
		}
		return "(" + strings.Join(parts, ", ") + ")"
	case PlanError:
		return "(" + e.Error + ")"
	default:
		return ""
	}
}

func mcpPlanDetail(e PlanMCPEntry) string {
	switch e.Action {
	case PlanError:
		return "(" + e.Error + ")"
	default:
		if e.Target == "" {
			return ""
		}
		return "→ " + displayTarget(e.Target, planTargetSoftLimit)
	}
}
