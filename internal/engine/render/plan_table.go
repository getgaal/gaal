package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/pterm/pterm"
)

// actionLabel maps a PlanAction to a human-readable display string.
var actionLabel = map[PlanAction]string{
	PlanNoOp:   "no change",
	PlanClone:  "clone",
	PlanUpdate: "update",
	PlanCreate: "create",
	PlanError:  "error",
}

// actionCell renders a PlanAction as a coloured pterm string.
func actionCell(action PlanAction, errMsg string) string {
	label, ok := actionLabel[action]
	if !ok {
		label = string(action)
	}
	switch action {
	case PlanNoOp:
		return pterm.FgGreen.Sprint("  " + label)
	case PlanClone, PlanCreate:
		return pterm.FgCyan.Sprint("+ " + label)
	case PlanUpdate:
		return pterm.FgYellow.Sprint("~ " + label)
	case PlanError:
		msg := label
		if errMsg != "" {
			msg = errMsg
		}
		return pterm.FgRed.Sprint("! " + msg)
	default:
		return label
	}
}

type planTableRenderer struct{}

func (pr *planTableRenderer) Render(w io.Writer, r *PlanReport) error {
	tr := &tableRenderer{}
	termW := pterm.GetTerminalWidth()
	if termW < 60 {
		termW = 120
	}

	if err := pr.repoTable(w, tr, r.Repositories, termW); err != nil {
		return err
	}
	if err := pr.skillTable(w, tr, r.Skills, termW); err != nil {
		return err
	}
	if err := pr.mcpTable(w, tr, r.MCPs, termW); err != nil {
		return err
	}

	fmt.Fprintln(w)
	if r.HasErrors {
		fmt.Fprintln(w, pterm.FgRed.Sprint("Plan completed with errors."))
	} else if r.HasChanges {
		fmt.Fprintln(w, pterm.FgYellow.Sprint("Sync would make changes. Run 'gaal sync' to apply."))
	} else {
		fmt.Fprintln(w, pterm.FgGreen.Sprint("Everything is up to date. Nothing to sync."))
	}

	return nil
}

func (pr *planTableRenderer) repoTable(w io.Writer, tr *tableRenderer, entries []PlanRepoEntry, termW int) error {
	tr.section(w, "Repositories", len(entries))
	vw := varColWidth(termW, 4, 2, 22)
	pathMax := vw * 55 / 100
	infoMax := vw * 45 / 100
	if pathMax < 15 {
		pathMax = 15
	}
	if infoMax < 15 {
		infoMax = 15
	}

	data := pterm.TableData{{"PATH", "TYPE", "ACTION", "DETAIL"}}
	for _, e := range entries {
		var detail string
		switch e.Action {
		case PlanClone:
			detail = e.URL
		case PlanUpdate:
			detail = e.Current + " -> " + e.Want
		case PlanError:
			detail = e.Error
		}
		data = append(data, []string{
			trunc(e.Path, pathMax),
			e.Type,
			actionCell(e.Action, e.Error),
			trunc(detail, infoMax),
		})
	}
	return tr.ptermTable(w, data)
}

func (pr *planTableRenderer) skillTable(w io.Writer, tr *tableRenderer, entries []PlanSkillEntry, termW int) error {
	tr.section(w, "Skills", len(entries))
	vw := varColWidth(termW, 4, 3, 14)
	if vw < 12 {
		vw = 12
	}

	data := pterm.TableData{{"SOURCE", "AGENT", "ACTION", "DETAIL"}}
	for _, e := range entries {
		var detail string
		switch e.Action {
		case PlanCreate:
			parts := append([]string{}, e.Install...)
			if len(e.Update) > 0 {
				parts = append(parts, e.Update...)
			}
			detail = "install: " + strings.Join(parts, ", ")
		case PlanUpdate:
			detail = "update: " + strings.Join(e.Update, ", ")
		case PlanError:
			detail = e.Error
		}
		data = append(data, []string{
			trunc(e.Source, vw),
			trunc(e.Agent, vw),
			actionCell(e.Action, e.Error),
			trunc(detail, vw),
		})
	}
	return tr.ptermTable(w, data)
}

func (pr *planTableRenderer) mcpTable(w io.Writer, tr *tableRenderer, entries []PlanMCPEntry, termW int) error {
	tr.section(w, "MCP Configs", len(entries))
	vw := varColWidth(termW, 3, 1, 34)
	if vw < 20 {
		vw = 20
	}

	data := pterm.TableData{{"NAME", "ACTION", "TARGET"}}
	for _, e := range entries {
		data = append(data, []string{
			e.Name,
			actionCell(e.Action, e.Error),
			trunc(e.Target, vw),
		})
	}
	return tr.ptermTable(w, data)
}
