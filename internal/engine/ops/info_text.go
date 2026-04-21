package ops

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"gaal/internal/config"
	"gaal/internal/core/agent"
	"gaal/internal/engine/render"
)

// ── Text renderers ───────────────────────────────────────────────────────────
//
// The functions below produce a compact line-based layout matching the
// samples in docs/content/cli/info.mdx. Each entry is one card of the form:
//
//	<type>: <name>
//	  key:       value
//	  key:       value
//
// Empty fields are omitted, and filtering is handled by the caller.

const infoTextKeyWidth = 13

// infoTextKV formats a single key/value line with a fixed-width key column.
func infoTextKV(key, value string) string {
	pad := infoTextKeyWidth - len(key)
	if pad < 1 {
		pad = 1
	}
	return fmt.Sprintf("  %s:%s%s", key, strings.Repeat(" ", pad), value)
}

func renderRepoInfoText(w io.Writer, cfgRepos map[string]config.ConfigRepo, entries []render.RepoEntry, filter string) error {
	filtered := make([]render.RepoEntry, 0, len(entries))
	for _, e := range entries {
		if matchFilter(e.Path, filter) {
			filtered = append(filtered, e)
		}
	}
	if len(filtered) == 0 {
		if filter != "" {
			fmt.Fprintf(w, "no repository matches %q\n", filter)
			return nil
		}
		fmt.Fprintln(w, "no repositories configured")
		return nil
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].Path < filtered[j].Path })

	for i, e := range filtered {
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintf(w, "repo: %s\n", e.Path)
		fmt.Fprintln(w, infoTextKV("type", e.Type))
		if e.URL != "" {
			fmt.Fprintln(w, infoTextKV("url", e.URL))
		}
		cfg := cfgRepos[e.Path]
		version := orDefault(cfg.Version, "default (HEAD)")
		if e.Current != "" {
			version = fmt.Sprintf("%s  →  current: %s", version, e.Current)
		}
		fmt.Fprintln(w, infoTextKV("version", version))
		fmt.Fprintln(w, infoTextKV("status", repoStatusLabel(e)))
		if e.Error != "" {
			fmt.Fprintln(w, infoTextKV("error", e.Error))
		}
	}
	return nil
}

func repoStatusLabel(e render.RepoEntry) string {
	switch e.Status {
	case render.StatusOK:
		return "clean"
	case render.StatusDirty:
		return "dirty (local changes)"
	case render.StatusNotCloned:
		return "not cloned"
	case render.StatusError:
		return "error"
	case render.StatusUnmanaged:
		return "unmanaged"
	default:
		return string(e.Status)
	}
}

func renderSkillInfoText(w io.Writer, cfgSkills []config.ConfigSkill, entries []render.SkillEntry, filter string, home, workDir string) error {
	filtered := make([]config.ConfigSkill, 0, len(cfgSkills))
	for _, sc := range cfgSkills {
		if matchFilter(sc.Source, filter) {
			filtered = append(filtered, sc)
		}
	}
	if len(filtered) == 0 {
		if filter != "" {
			fmt.Fprintf(w, "no skill matches %q\n", filter)
			return nil
		}
		fmt.Fprintln(w, "no skills configured")
		return nil
	}

	bySource := make(map[string][]render.SkillEntry, len(entries))
	for _, e := range entries {
		bySource[e.Source] = append(bySource[e.Source], e)
	}

	for i, sc := range filtered {
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintf(w, "skill: %s\n", sc.Source)

		selection := "all"
		if len(sc.Select) > 0 {
			selection = "select [" + strings.Join(sc.Select, ", ") + "]"
		}
		fmt.Fprintln(w, infoTextKV("selection", selection))

		scope := "project"
		if sc.Global {
			scope = "global"
		}
		fmt.Fprintln(w, infoTextKV("scope", scope))

		agents := "all detected"
		if len(sc.Agents) > 0 && !(len(sc.Agents) == 1 && sc.Agents[0] == "*") {
			agents = strings.Join(sc.Agents, ", ")
		}
		fmt.Fprintln(w, infoTextKV("agents", agents))

		skillEntries := bySource[sc.Source]
		if len(skillEntries) > 0 {
			fmt.Fprintln(w, "  installed at:")
			for _, e := range skillEntries {
				dir, ok := skillInstallDir(e.Agent, e.Global, home, workDir)
				if !ok {
					continue
				}
				for _, name := range e.Installed {
					path := filepath.Join(dir, name) + string(filepath.Separator)
					fmt.Fprintf(w, "    %s   (%s)\n", path, skillStateDesc(e, name))
				}
				for _, name := range e.Missing {
					path := filepath.Join(dir, name) + string(filepath.Separator)
					fmt.Fprintf(w, "    %s   (missing)\n", path)
				}
			}
		}
	}
	return nil
}

// skillInstallDir resolves the directory gaal writes to for the given agent
// and scope, falling back to workDir-relative when the computed path is not
// absolute. Returns false when the agent has no registered skills dir.
func skillInstallDir(agentName string, global bool, home, workDir string) (string, bool) {
	dir, ok := agent.SkillDir(agentName, global, home)
	if !ok {
		return "", false
	}
	if !global && !filepath.IsAbs(dir) {
		dir = filepath.Join(workDir, dir)
	}
	return filepath.Clean(dir), true
}

func skillStateDesc(e render.SkillEntry, name string) string {
	for _, m := range e.Modified {
		if m == name {
			return "modified"
		}
	}
	if e.Error != "" {
		return "error: " + e.Error
	}
	return "fresh"
}

func renderMCPInfoText(w io.Writer, cfgMCPs []config.ConfigMcp, entries []render.MCPEntry, filter string) error {
	filtered := make([]config.ConfigMcp, 0, len(cfgMCPs))
	for _, mc := range cfgMCPs {
		if matchFilter(mc.Name, filter) {
			filtered = append(filtered, mc)
		}
	}
	if len(filtered) == 0 {
		if filter != "" {
			fmt.Fprintf(w, "no MCP matches %q\n", filter)
			return nil
		}
		fmt.Fprintln(w, "no MCP configs configured")
		return nil
	}

	byName := make(map[string]render.MCPEntry, len(entries))
	for _, e := range entries {
		byName[e.Name] = e
	}

	for i, mc := range filtered {
		if i > 0 {
			fmt.Fprintln(w)
		}
		e := byName[mc.Name]
		fmt.Fprintf(w, "mcp: %s\n", mc.Name)
		fmt.Fprintln(w, infoTextKV("target", mc.Target))
		if mc.Source != "" {
			fmt.Fprintln(w, infoTextKV("source", mc.Source))
		}
		merge := "false"
		if mc.MergeEnabled() {
			merge = "true"
		}
		fmt.Fprintln(w, infoTextKV("merge", merge))

		if mc.Inline != nil {
			fmt.Fprintln(w, "  inline:")
			fmt.Fprintf(w, "    command: %s\n", mc.Inline.Command)
			if len(mc.Inline.Args) > 0 {
				fmt.Fprintf(w, "    args:    [%s]\n", strings.Join(mc.Inline.Args, ", "))
			}
			if len(mc.Inline.Env) > 0 {
				envPairs := make([]string, 0, len(mc.Inline.Env))
				for k, v := range mc.Inline.Env {
					envPairs = append(envPairs, k+"="+v)
				}
				sort.Strings(envPairs)
				fmt.Fprintf(w, "    env:     %s\n", strings.Join(envPairs, ", "))
			}
		}

		fmt.Fprintln(w, infoTextKV("state", mcpStateLabel(e)))
		if e.Error != "" {
			fmt.Fprintln(w, infoTextKV("error", e.Error))
		}
	}
	return nil
}

func mcpStateLabel(e render.MCPEntry) string {
	switch e.Status {
	case render.StatusPresent:
		return "upserted, hash matches config"
	case render.StatusDirty:
		return "upserted with local changes"
	case render.StatusAbsent:
		return "absent (would be created on next sync)"
	case render.StatusError:
		return "error"
	case render.StatusUnmanaged:
		return "unmanaged"
	default:
		return string(e.Status)
	}
}

func renderAgentInfoText(w io.Writer, entries []render.AgentEntry, filter string) error {
	filtered := make([]render.AgentEntry, 0, len(entries))
	for _, e := range entries {
		if matchFilter(e.Name, filter) {
			filtered = append(filtered, e)
		}
	}
	if len(filtered) == 0 {
		if filter != "" {
			fmt.Fprintf(w, "no agent matches %q\n", filter)
			return nil
		}
		fmt.Fprintln(w, "no agents registered")
		return nil
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].Installed != filtered[j].Installed {
			return filtered[i].Installed
		}
		return filtered[i].Name < filtered[j].Name
	})

	for i, e := range filtered {
		if i > 0 {
			fmt.Fprintln(w)
		}
		label := "not installed"
		if e.Installed {
			label = "installed"
		}
		fmt.Fprintf(w, "agent: %s (%s)\n", e.Name, label)

		source := e.Source
		if source == "" {
			source = "builtin"
		}
		fmt.Fprintln(w, infoTextKV("source", source))

		if e.ProjectSkillsDir != "" {
			fmt.Fprintln(w, infoTextKV("project", e.ProjectSkillsDir))
		}
		if e.GlobalSkillsDir != "" {
			fmt.Fprintln(w, infoTextKV("global", e.GlobalSkillsDir))
		}
		if e.ProjectMCPConfigFile != "" {
			fmt.Fprintln(w, infoTextKV("mcp config", e.ProjectMCPConfigFile))
		}
	}
	return nil
}
