// Package tools checks whether CLI executables declared in gaal.yaml are
// present on the user's PATH. It is intentionally scope-limited to detection:
// installation, version parsing, and per-OS install hints are out of scope
// for now (see issue #49 discussion).
package tools

import (
	"fmt"
	"os/exec"

	"gaal/internal/config"
)

// SourceWorkspace is the attribution string used for tools declared at the
// top level of gaal.yaml.
const SourceWorkspace = "workspace"

// Entry pairs a declared tool with a human-readable attribution string
// describing where the declaration came from ("workspace" or
// "skill: <source>").
type Entry struct {
	Tool   config.ConfigTool
	Source string
}

// Status is the result of a presence check for a single Entry.
type Status struct {
	Entry    Entry
	Found    bool
	Resolved string // absolute path from exec.LookPath when Found; empty otherwise
}

// Collect flattens top-level and per-skill tools into a deduplicated slice of
// Entry, keyed on Tool.Name. Top-level declarations are collected first, so a
// workspace-level entry (including its hint) wins attribution over any
// per-skill mention of the same name. Per-skill entries appearing before a
// top-level declaration of the same name are still shadowed by the top-level
// one when ordering is preserved in input — the rule is simply first-seen
// wins, and Collect walks top-level before skills.
func Collect(cfg *config.Config) []Entry {
	if cfg == nil {
		return nil
	}

	out := make([]Entry, 0, len(cfg.Tools))
	seen := make(map[string]struct{}, len(cfg.Tools))

	add := func(tool config.ConfigTool, source string) {
		if tool.Name == "" {
			return
		}
		if _, dup := seen[tool.Name]; dup {
			return
		}
		seen[tool.Name] = struct{}{}
		out = append(out, Entry{Tool: tool, Source: source})
	}

	for _, tl := range cfg.Tools {
		add(tl, SourceWorkspace)
	}
	for _, sk := range cfg.Skills {
		source := fmt.Sprintf("skill: %s", sk.Source)
		for _, tl := range sk.Tools {
			add(tl, source)
		}
	}
	return out
}

// Check runs exec.LookPath for each entry and returns one Status per input,
// preserving order. It performs no network I/O and no concurrency — tool
// counts are small in practice.
func Check(entries []Entry) []Status {
	out := make([]Status, len(entries))
	for i, e := range entries {
		resolved, err := exec.LookPath(e.Tool.Name)
		out[i] = Status{Entry: e, Found: err == nil, Resolved: resolved}
	}
	return out
}
