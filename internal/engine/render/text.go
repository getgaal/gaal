package render

import (
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
)

// textRenderer implements Renderer producing a compact, pipe-friendly text
// layout that matches the samples shown in the docs (cli/status.mdx):
//
//	repositories
//	  src/gaal               git · main · clean
//
// Sections are lowercased and indented with two spaces. Empty sections are
// omitted entirely so scripts can rely on presence-as-signal.
type textRenderer struct{}

func (tr *textRenderer) Render(w io.Writer, r *StatusReport) error {
	slog.Debug("rendering text output")

	wroteAny := false
	write := func(f func() bool) {
		if f() {
			wroteAny = true
		}
	}

	write(func() bool { return tr.repoSection(w, r.Repositories) })
	write(func() bool { return tr.skillSection(w, r.Skills) })
	write(func() bool { return tr.mcpSection(w, r.MCPs) })
	write(func() bool { return tr.agentSection(w, r.Agents) })

	if !wroteAny {
		fmt.Fprintln(w, "no managed resources")
	}
	return nil
}

func (tr *textRenderer) repoSection(w io.Writer, entries []RepoEntry) bool {
	if len(entries) == 0 {
		return false
	}
	fmt.Fprintln(w, "repositories")
	nameWidth := maxWidth(entries, func(i int) string { return entries[i].Path })
	for _, e := range entries {
		fmt.Fprintf(w, "  %s  %s\n", padText(e.Path, nameWidth), repoTextInfo(e))
	}
	fmt.Fprintln(w)
	return true
}

func repoTextInfo(e RepoEntry) string {
	parts := []string{}
	if e.Type != "" {
		parts = append(parts, e.Type)
	}
	switch e.Status {
	case StatusOK:
		if e.Current != "" {
			parts = append(parts, e.Current)
		} else if e.Want != "" {
			parts = append(parts, e.Want)
		}
		parts = append(parts, "clean")
	case StatusDirty:
		if e.Current != "" {
			parts = append(parts, e.Current)
		}
		parts = append(parts, "dirty")
	case StatusNotCloned:
		parts = append(parts, "not cloned")
		if e.URL != "" {
			parts = append(parts, e.URL)
		}
	case StatusUnmanaged:
		parts = append(parts, "unmanaged")
	case StatusError:
		if e.Error != "" {
			parts = append(parts, "error: "+e.Error)
		} else {
			parts = append(parts, "error")
		}
	default:
		parts = append(parts, string(e.Status))
	}
	return strings.Join(parts, " · ")
}

func (tr *textRenderer) skillSection(w io.Writer, entries []SkillEntry) bool {
	aggregated := aggregateSkillsByName(entries)
	if len(aggregated) == 0 {
		return false
	}
	fmt.Fprintln(w, "skills")

	nameWidth := 0
	for _, s := range aggregated {
		if n := len(displayName(s.Name)); n > nameWidth {
			nameWidth = n
		}
	}

	for _, s := range aggregated {
		name := displayName(s.Name)
		agents := "none"
		if len(s.Agents) > 0 {
			agents = strings.Join(s.Agents, ", ")
		}
		scope := "workspace"
		if s.Global {
			scope = "global"
		}
		if s.Status == StatusError && s.Error != "" {
			fmt.Fprintf(w, "  %s  error: %s\n", padText(name, nameWidth), s.Error)
			continue
		}
		fmt.Fprintf(w, "  %s  %s (%s)\n", padText(name, nameWidth), agents, scope)
	}
	fmt.Fprintln(w)
	return true
}

func displayName(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func (tr *textRenderer) mcpSection(w io.Writer, entries []MCPEntry) bool {
	if len(entries) == 0 {
		return false
	}
	fmt.Fprintln(w, "mcps")

	nameWidth := maxWidth(entries, func(i int) string { return entries[i].Name })
	for _, e := range entries {
		suffix := ""
		switch e.Status {
		case StatusDirty:
			suffix = " (dirty)"
		case StatusAbsent:
			suffix = " (absent)"
		case StatusError:
			if e.Error != "" {
				suffix = " (error: " + e.Error + ")"
			} else {
				suffix = " (error)"
			}
		case StatusUnmanaged:
			suffix = " (unmanaged)"
		}
		fmt.Fprintf(w, "  %s  %s%s\n", padText(e.Name, nameWidth), e.Target, suffix)
	}
	fmt.Fprintln(w)
	return true
}

func (tr *textRenderer) agentSection(w io.Writer, entries []AgentEntry) bool {
	if len(entries) == 0 {
		return false
	}
	installed := make([]string, 0, len(entries))
	other := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.Installed {
			installed = append(installed, e.Name)
		} else {
			other = append(other, e.Name)
		}
	}
	sort.Strings(installed)
	sort.Strings(other)

	fmt.Fprintln(w, "agents")
	if len(installed) > 0 {
		fmt.Fprintf(w, "  installed:     %s\n", strings.Join(installed, ", "))
	}
	if len(other) > 0 {
		fmt.Fprintf(w, "  not installed: %s\n", strings.Join(other, ", "))
	}
	fmt.Fprintln(w)
	return true
}

// maxWidth returns the longest visible length of the string returned by f for
// indices [0, len(slice)). It is a small helper to line up two-column text
// output without pulling in pterm's tabular layout.
func maxWidth[T any](slice []T, f func(i int) string) int {
	max := 0
	for i := range slice {
		if n := len(f(i)); n > max {
			max = n
		}
	}
	return max
}

// padText right-pads s with spaces to width. It assumes no embedded ANSI codes
// (text renderers intentionally skip colouring).
func padText(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}
