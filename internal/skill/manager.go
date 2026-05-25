package skill

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"gaal/internal/config"
	"gaal/internal/core/agent"
	"gaal/internal/core/vcs"
	"gaal/internal/discover"
	"gaal/internal/urlx"
)

// buildDiscoveryDirs returns the deduplicated list of subdirectories to scan
// for SKILL.md files within a source repository. It combines a small set of
// generic paths with every agent's skill directories derived from the registry,
// so that new agents are picked up automatically without any changes here.
func buildDiscoveryDirs() []string {
	seen := map[string]struct{}{}
	dirs := make([]string, 0)
	add := func(d string) {
		if d == "" {
			return
		}
		if _, ok := seen[d]; !ok {
			seen[d] = struct{}{}
			dirs = append(dirs, d)
		}
	}

	// Generic / common layout paths.
	for _, d := range []string{".", "skills", "skills/.curated", "skills/.experimental", "skills/.system"} {
		add(d)
	}

	// Agent-specific paths from the registry.
	for _, name := range AgentNames() {
		info, ok := Lookup(name)
		if !ok {
			continue
		}
		// Project-relative install dir (e.g. ".claude/skills").
		add(info.ProjectSkillsDir)
		// Global install dir is home-relative (e.g. "~/.cursor/skills") —
		// strip the leading "~/" or "~\" to obtain the bare relative path
		// a source repository might use to organise agent-specific skills.
		g := info.GlobalSkillsDir
		if strings.HasPrefix(g, "~/") || strings.HasPrefix(g, `~\`) {
			add(g[2:])
		}
	}

	return dirs
}

// SkillMeta holds the parsed frontmatter of a SKILL.md file.
type SkillMeta struct {
	Name        string
	Description string
	Dir         string // absolute path to the skill directory
}

// Status describes the installation state of a skill configuration entry.
type Status struct {
	Source       string
	AgentName    string
	Global       bool
	TargetSubdir string
	Installed    []string // names of installed skills
	Missing      []string // names of skills not yet installed
	Modified     []string // names of installed skills whose local files differ from source
	Err          error
}

// Manager handles skill installation and updates.
type Manager struct {
	skills   []config.ConfigSkill
	cacheDir string // root of the local skill cache
	home     string // expanded user home directory
	workDir  string // project working directory (for project-scoped installs)
	stateDir string // gaal state root (~/.cache/gaal/state) for snapshot writing
	force    bool   // when true, wildcard agent lists target all known agents
	warnOnce sync.Once
}

// NewManager creates a new skill Manager.
// cacheDir is where remote sources are cloned/downloaded (e.g. ~/.cache/gaal/skills).
// stateDir is where post-sync snapshots are persisted (e.g. ~/.cache/gaal/state).
// force makes wildcard agent lists install into every registered agent,
// creating directories as needed instead of restricting to already-installed agents.
func NewManager(skills []config.ConfigSkill, cacheDir, home, workDir, stateDir string, force bool) *Manager {
	return &Manager{
		skills:   skills,
		cacheDir: cacheDir,
		home:     home,
		workDir:  workDir,
		stateDir: stateDir,
		force:    force,
	}
}

// SourcePaths returns the absolute, home-expanded local paths of every
// configured skill source. Remote sources (URLs and GitHub shorthands) are
// reported by their on-disk cache path. The result lets callers identify
// directories that hold *source* skill content rather than installed copies,
// e.g. so the FS scan does not surface them as unmanaged installs (#88).
func (m *Manager) SourcePaths() []string {
	out := make([]string, 0, len(m.skills))
	for _, sc := range m.skills {
		if isLocalPath(sc.Source) {
			out = append(out, expandHome(sc.Source, m.home))
			continue
		}
		cloneURL := toCloneURL(sc.Source)
		out = append(out, filepath.Join(m.cacheDir, urlToCacheKey(cloneURL)))
	}
	return out
}

// Sync installs or updates every skill in the configuration. A failure on one
// source does not abort the rest — every entry is attempted, and the
// per-entry errors are joined via errors.Join so callers can inspect each
// underlying cause via errors.As / errors.Is.
func (m *Manager) Sync(ctx context.Context) error {
	m.emitConfigWarnings()
	var errs []error
	for _, sc := range m.skills {
		if err := m.syncOne(ctx, sc); err != nil {
			errs = append(errs, fmt.Errorf("skill %q: %w", sc.Source, err))
		}
	}
	return errors.Join(errs...)
}

// emitConfigWarnings surfaces issues that depend on the user's skill config
// but aren't tied to a single source. Runs at most once per Manager (gated
// by warnOnce) so the messages don't repeat across Sync and Status.
//
// Per-agent behavioural constraints (skills unsupported, platform
// restrictions) come from [agent.CollectWarnings] — no hard-coded agent
// names live here.
func (m *Manager) emitConfigWarnings() {
	m.warnOnce.Do(m.emitBehaviorWarnings)
}

// emitBehaviorWarnings groups the configured skill entries by scope and
// asks the agent registry for the behaviour mismatches that apply. One
// log line per unique (Code, Agent), so wildcard expansion does not
// produce one warning per matching agent.
func (m *Manager) emitBehaviorWarnings() {
	var projectAgents, globalAgents []string
	for _, sc := range m.skills {
		if sc.Global {
			globalAgents = append(globalAgents, sc.Agents...)
		} else {
			projectAgents = append(projectAgents, sc.Agents...)
		}
	}
	warnings := agent.CollectWarnings(runtime.GOOS,
		agent.Group{Scope: agent.ScopeSkillProject, Agents: projectAgents},
		agent.Group{Scope: agent.ScopeSkillGlobal, Agents: globalAgents},
	)
	for _, w := range warnings {
		slog.Warn("skill: "+w.Msg,
			"agent", w.Agent, "scope", w.Scope, "code", w.Code, "hint", w.Hint)
	}
}

func (m *Manager) syncOne(ctx context.Context, sc config.ConfigSkill) error {
	slog.DebugContext(ctx, "syncing skill source", "source", sc.Source, "global", sc.Global)
	// 1. Resolve the source to a local directory.
	sourceDir, err := m.resolveSource(ctx, sc.Source)
	if err != nil {
		return fmt.Errorf("resolving source: %w", err)
	}

	// 2. Discover available skills in the source.
	available, err := discoverSkills(sourceDir)
	if err != nil {
		return fmt.Errorf("discovering skills: %w", err)
	}

	// 3. Filter by the "select" list.
	selected := filterSkills(available, sc.Select)
	if len(selected) == 0 {
		slog.Warn("no skills found in source", "source", sc.Source)
		return nil
	}

	// 4. Determine target agents — only those actually installed on this
	//    machine. Uninstalled entries are dropped with a warning so sync
	//    never creates agent-owned directories as a side effect.
	agents := m.syncAgents(sc)
	slog.DebugContext(ctx, "resolved sync agents", "source", sc.Source, "agents", agents)

	// 5. Install each skill to each agent. Per-agent and per-skill errors are
	//    accumulated so a single failure (e.g. one agent's dir is read-only)
	//    does not skip every later install.
	var errs []error
	for _, agent := range agents {
		skillsDir, ok, err := m.targetSkillsDir(agent, sc)
		if err != nil {
			errs = append(errs, fmt.Errorf("resolving skills dir for agent %q: %w", agent, err))
			continue
		}
		if !ok {
			slog.Warn("unknown agent, skipping", "agent", agent)
			continue
		}

		// Create the skills directory if it does not exist yet.
		if err := os.MkdirAll(skillsDir, 0o755); err != nil {
			errs = append(errs, fmt.Errorf("creating skills dir for agent %q: %w", agent, err))
			continue
		}

		for _, sk := range selected {
			dest := filepath.Join(skillsDir, filepath.Base(sk.Dir))
			if err := installSkill(sk.Dir, dest); err != nil {
				errs = append(errs, fmt.Errorf("installing skill %q to agent %q: %w", sk.Name, agent, err))
				continue
			}
			slog.Debug("installed skill", "name", sk.Name, "agent", agent, "dest", dest)
			m.writeSkillSnapshot(dest)
		}
	}

	return errors.Join(errs...)
}

func (m *Manager) targetSkillsDir(agentName string, sc config.ConfigSkill) (string, bool, error) {
	slog.Debug("resolving target skills dir", "agent", agentName, "global", sc.Global, "targetSubdir", sc.TargetSubdir)
	skillsDir, ok := SkillDir(agentName, sc.Global, m.home)
	if !ok {
		return "", false, nil
	}
	if !sc.Global && !filepath.IsAbs(skillsDir) {
		skillsDir = filepath.Join(m.workDir, skillsDir)
	}
	if sc.TargetSubdir != "" {
		slashed := strings.ReplaceAll(sc.TargetSubdir, `\`, "/")
		if strings.HasPrefix(slashed, "/") || strings.Contains(slashed, ":") {
			return "", true, fmt.Errorf("unsafe target_subdir %q", sc.TargetSubdir)
		}
		for _, seg := range strings.Split(slashed, "/") {
			if seg == "" || seg == "." || seg == ".." {
				return "", true, fmt.Errorf("unsafe target_subdir %q", sc.TargetSubdir)
			}
		}
		clean := filepath.Clean(sc.TargetSubdir)
		if filepath.IsAbs(sc.TargetSubdir) || clean == "." || clean == ".." || strings.HasPrefix(filepath.ToSlash(clean), "../") {
			return "", true, fmt.Errorf("unsafe target_subdir %q", sc.TargetSubdir)
		}
		skillsDir = filepath.Join(skillsDir, sc.TargetSubdir)
	}
	return skillsDir, true, nil
}

// resolveSource ensures the source is available locally and returns its path.
func (m *Manager) resolveSource(ctx context.Context, source string) (string, error) {
	if isLocalPath(source) {
		expanded := source
		if strings.HasPrefix(source, "~/") || strings.HasPrefix(source, `~\`) {
			expanded = filepath.Join(m.home, source[2:])
		}
		// If the local path is itself a VCS repository, refresh it so that
		// skills stay up-to-date even when the source is a sibling checkout.
		vcsType := vcs.DetectType(expanded)
		backend, err := vcs.NewShallow(vcsType)
		if err == nil && backend.IsCloned(expanded) {
			slog.DebugContext(ctx, "updating local source", "path", expanded, "vcs", vcsType)
			if err := backend.Update(ctx, "", expanded, ""); err != nil {
				slog.Warn("could not update local source", "path", expanded, "err", err)
			}
		}
		return expanded, nil
	}

	cloneURL := toCloneURL(source)
	vcsType := vcs.DetectType(cloneURL)
	cacheKey := urlToCacheKey(cloneURL)
	localPath := filepath.Join(m.cacheDir, cacheKey)

	backend, err := vcs.NewShallow(vcsType)
	if err != nil {
		return "", fmt.Errorf("creating VCS backend for %q: %w", source, err)
	}

	if !backend.IsCloned(localPath) {
		safeURL := urlx.Redact(cloneURL)
		slog.Debug("cloning skill source", "url", safeURL, "path", localPath)
		if err := backend.Clone(ctx, cloneURL, localPath, ""); err != nil {
			return "", fmt.Errorf("cloning %s: %w", safeURL, err)
		}
	} else {
		slog.Debug("updating skill source", "path", localPath)
		if err := backend.Update(ctx, "", localPath, ""); err != nil {
			slog.Warn("could not update skill source", "path", localPath, "err", err)
		}
	}

	return localPath, nil
}

// resolveAgents returns the list of agents named by a skill config. The
// wildcard "*" expands to every installed agent; explicit lists are returned
// as-is. This is the "as-configured" view used by Status to surface
// misconfiguration to the user (e.g. unknown agent names).
func (m *Manager) resolveAgents(sc config.ConfigSkill) []string {
	if len(sc.Agents) == 0 || (len(sc.Agents) == 1 && sc.Agents[0] == "*") {
		return m.detectInstalledAgents(sc.Global)
	}
	return sc.Agents
}

// syncAgents returns the agents to target during sync.
//
// For the wildcard ("*" or empty list):
//   - Normal mode: only already-installed agents (safe default, never creates dirs).
//   - Force mode (--force): all registered agents, creating directories as needed.
//
// For explicitly named agents, all entries are returned regardless of
// whether the agent directory exists yet: sync creates it as needed.
func (m *Manager) syncAgents(sc config.ConfigSkill) []string {
	// Wildcard: respect force flag.
	if len(sc.Agents) == 0 || (len(sc.Agents) == 1 && sc.Agents[0] == "*") {
		if m.force {
			slog.Debug("force mode: targeting all registered agents", "source", sc.Source)
			return AgentNames()
		}
		return m.detectInstalledAgents(sc.Global)
	}
	// Explicit list: return all named agents; sync creates directories as needed.
	return sc.Agents
}

// isAgentInstalled reports whether the directory that would own the agent's
// skills on this machine already exists. This is the single "installed?"
// signal used by sync: we never create agent-owned directories as a side
// effect of a sync run.
func (m *Manager) isAgentInstalled(name string, global bool) bool {
	return IsAgentInstalled(name, global, m.home, m.workDir)
}

// detectInstalledAgents returns every registered agent whose config-owning
// directory is present on this machine. Used for the `agents: ["*"]` wildcard.
func (m *Manager) detectInstalledAgents(global bool) []string {
	slog.Debug("detecting installed agents", "global", global)
	var found []string
	for _, name := range AgentNames() {
		if m.isAgentInstalled(name, global) {
			slog.Debug("agent detected", "name", name, "global", global)
			found = append(found, name)
		}
	}
	return found
}

// Status returns the installation status for every skill config.
func (m *Manager) Status(ctx context.Context) []Status {
	m.emitConfigWarnings()
	statuses := make([]Status, 0, len(m.skills))

	for _, sc := range m.skills {
		agents := m.resolveAgents(sc)
		for _, agent := range agents {
			st := Status{Source: sc.Source, AgentName: agent, Global: sc.Global, TargetSubdir: sc.TargetSubdir}

			skillsDir, ok, err := m.targetSkillsDir(agent, sc)
			if err != nil {
				st.Err = err
				statuses = append(statuses, st)
				continue
			}
			if !ok {
				st.Err = fmt.Errorf("unknown agent %q", agent)
				statuses = append(statuses, st)
				continue
			}

			// Resolve local source (may not be downloaded yet).
			sourceDir, err := cachedSourcePath(m.cacheDir, sc.Source)
			if err != nil || sourceDir == "" {
				st.Err = fmt.Errorf("source not cached yet")
				statuses = append(statuses, st)
				continue
			}

			available, _ := discoverSkills(sourceDir)
			selected := filterSkills(available, sc.Select)

			for _, sk := range selected {
				dest := filepath.Join(skillsDir, filepath.Base(sk.Dir))
				if _, err := os.Stat(dest); err == nil {
					st.Installed = append(st.Installed, sk.Name)
					if skillDirModified(sk.Dir, dest) {
						st.Modified = append(st.Modified, sk.Name)
					}
				} else {
					st.Missing = append(st.Missing, sk.Name)
				}
			}

			statuses = append(statuses, st)
		}
	}

	return statuses
}

// discoverSkills finds all SKILL.md files under root using standard locations.
func discoverSkills(root string) ([]SkillMeta, error) {
	slog.Debug("discovering skills", "root", root)
	seen := map[string]struct{}{}
	var skills []SkillMeta

	for _, subdir := range buildDiscoveryDirs() {
		base := filepath.Join(root, subdir)
		entries, err := os.ReadDir(base)
		if err != nil {
			continue
		}

		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			// Reject directory names that would let a malicious skill
			// source escape the install root via filepath.Join — `..`,
			// `.`, empty, or names containing path separators after
			// cleaning. The install path uses filepath.Base(sk.Dir) so
			// these would otherwise walk above the agent skill dir.
			if !isSafeSkillDirName(e.Name()) {
				slog.Warn("skill: refusing unsafe directory name",
					"name", e.Name(), "parent", base)
				continue
			}
			skillDir := filepath.Join(base, e.Name())
			mdPath := filepath.Join(skillDir, "SKILL.md")
			if _, err := os.Stat(mdPath); err != nil {
				continue
			}
			if _, ok := seen[skillDir]; ok {
				continue
			}
			seen[skillDir] = struct{}{}

			meta, err := parseSkillMeta(mdPath)
			if err != nil {
				slog.Warn("skipping invalid SKILL.md", "path", mdPath, "err", err)
				continue
			}
			// Frontmatter `name:` is also subject to the same containment
			// rule — a malicious source could ship `name: ../escape` and
			// the install layer uses Name to compose select-list matches.
			if meta.Name != "" && !isSafeSkillDirName(meta.Name) {
				slog.Warn("skill: refusing unsafe frontmatter name",
					"name", meta.Name, "path", mdPath)
				continue
			}
			meta.Dir = skillDir
			skills = append(skills, meta)
		}
	}

	// Also check if root itself contains SKILL.md.
	rootMD := filepath.Join(root, "SKILL.md")
	if _, err := os.Stat(rootMD); err == nil {
		if _, ok := seen[root]; !ok {
			meta, err := parseSkillMeta(rootMD)
			if err == nil {
				meta.Dir = root
				skills = append(skills, meta)
			}
		}
	}

	return skills, nil
}

// parseSkillMeta reads the YAML frontmatter from a SKILL.md file.
// Delegates to the exported ParseSkillMeta helper in scan.go.
func parseSkillMeta(path string) (SkillMeta, error) {
	name, desc, err := ParseSkillMeta(path)
	if err != nil {
		return SkillMeta{}, err
	}
	return SkillMeta{Name: name, Description: desc}, nil
}

// filterSkills returns the skills whose names match the select list.
// An empty select list means "all".
func filterSkills(all []SkillMeta, selectNames []string) []SkillMeta {
	if len(selectNames) == 0 {
		return all
	}
	set := make(map[string]struct{}, len(selectNames))
	for _, n := range selectNames {
		set[n] = struct{}{}
	}
	var out []SkillMeta
	for _, sk := range all {
		if _, ok := set[sk.Name]; ok {
			out = append(out, sk)
		}
	}
	return out
}

// vcsMetaDirs lists the on-disk metadata directories that gaal must never
// copy when installing a skill. They belong to the source's VCS bookkeeping,
// not the skill itself, and re-syncing them surfaces two problems:
//   - they bloat the install with megabytes of pack files;
//   - go-git creates pack files with mode 0o444, which prevents the next
//     sync from overwriting them (the second os.WriteFile cannot truncate
//     a read-only file). See issue #86.
var vcsMetaDirs = map[string]struct{}{
	".git": {},
	".hg":  {},
	".svn": {},
	".bzr": {},
}

// installSkill copies the skill directory content from src to dst.
// VCS metadata directories at the top level of src are skipped — see
// [vcsMetaDirs] for the rationale.
//
// Symlinks (and other non-regular entries: devices, FIFOs, sockets) are
// explicitly skipped instead of being dereferenced. Without this guard, a
// malicious skill source containing e.g. `secret.md -> /etc/passwd` would
// have copyFile read /etc/passwd and write its contents into the agent
// skill directory under the gaal-managed name. See #113.
func installSkill(src, dst string) error {
	slog.Debug("installing skill", "src", src, "dst", dst)

	// Stage in a sibling temp dir then atomically swap. A crash mid-copy
	// leaves the previous dst untouched (or the new content fully
	// installed) instead of a half-written skill that the next sync would
	// treat as up-to-date. Stale files (helpers removed upstream) are
	// gone after the swap because dst is replaced wholesale, not
	// overlay-merged. #121.
	parent := filepath.Dir(dst)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("creating skill parent dir: %w", err)
	}
	staging, err := os.MkdirTemp(parent, ".gaal-skill-tmp-*")
	if err != nil {
		return fmt.Errorf("creating skill staging dir: %w", err)
	}
	cleanupStage := true
	defer func() {
		if cleanupStage {
			_ = os.RemoveAll(staging)
		}
	}()

	walkErr := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if _, skip := vcsMetaDirs[d.Name()]; skip && path != src {
				return filepath.SkipDir
			}
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(staging, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		// Skip symlinks (and other non-regular entries) — see func docstring.
		if d.Type()&fs.ModeSymlink != 0 {
			slog.Warn("skill: skipping symlink in source",
				"path", path, "rel", rel,
				"reason", "symlinks are not dereferenced — would leak the link target's content into the install dir")
			return nil
		}
		if !d.Type().IsRegular() {
			slog.Warn("skill: skipping non-regular file in source",
				"path", path, "rel", rel, "mode", d.Type().String())
			return nil
		}

		return copyFile(path, target)
	})
	if walkErr != nil {
		return walkErr
	}

	// Replace dst atomically. RemoveAll + Rename — Go's os.Rename onto an
	// existing directory fails on POSIX (EEXIST), so we have to remove
	// first. The window is brief; with secfile/atomic semantics on
	// individual config files and the staging-then-rename here, a sync
	// in flight is still safer than the previous overlay copy.
	if err := os.RemoveAll(dst); err != nil {
		return fmt.Errorf("removing previous skill dir: %w", err)
	}
	if err := os.Rename(staging, dst); err != nil {
		return fmt.Errorf("renaming staging skill dir: %w", err)
	}
	cleanupStage = false
	return nil
}

// copyFile copies a single file from src to dst. Callers must have already
// filtered out symlinks (see installSkill) — copyFile uses os.ReadFile which
// follows them.
//
// File mode is sanitised: the executable bit is preserved when present in
// the source, but read/write bits for owner are forced on (0o644 base, or
// 0o755 when exec). Without this a source shipping a 0o000 / 0o400 file
// would land unwritable on disk and break the next sync (#132 — same
// class as the .git pack-file regression in #86).
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	fi, err := os.Stat(src)
	if err != nil {
		return err
	}

	mode := os.FileMode(0o644)
	if fi.Mode().Perm()&0o111 != 0 {
		mode = 0o755
	}
	return os.WriteFile(dst, data, mode)
}

// errDiffer is a sentinel used inside skillDirModified to stop the walk early.
var errDiffer = errors.New("differ")

// skillDirModified returns true when the installed copy at dst differs from
// the source skill at src. It compares every file's byte content; a missing
// or unreadable destination file is treated as a modification.
func skillDirModified(src, dst string) bool {
	slog.Debug("comparing skill directories", "src", src, "dst", dst)
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		// Symlinks are not installed (see installSkill), so skip them here too
		// — otherwise we'd dereference into the link target via os.ReadFile.
		if d.Type()&fs.ModeSymlink != 0 || !d.Type().IsRegular() {
			return nil
		}
		rel, _ := filepath.Rel(src, path)
		dstPath := filepath.Join(dst, rel)

		srcData, err := os.ReadFile(path)
		if err != nil {
			return nil // unreadable source — skip
		}
		dstData, err := os.ReadFile(dstPath)
		if err != nil {
			return errDiffer // file missing at destination
		}
		if !bytes.Equal(srcData, dstData) {
			return errDiffer
		}
		return nil
	})
	return errors.Is(err, errDiffer)
}

// isSafeSkillDirName rejects directory or frontmatter names that would
// allow a malicious skill source to escape the install root via
// filepath.Join. Disallows: empty, ".", "..", path separators, and any
// name whose filepath.Clean form differs from itself (catches embedded
// "./" tricks). #131.
func isSafeSkillDirName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	if strings.ContainsAny(name, `/\`) {
		return false
	}
	if filepath.Clean(name) != name {
		return false
	}
	return true
}

// isLocalPath reports whether source is a local filesystem path.
// It recognises both POSIX and Windows path conventions so that config files
// written on one OS can be used on the other.
func isLocalPath(source string) bool {
	if filepath.IsAbs(source) {
		return true
	}
	// Windows drive-letter absolute path (e.g. C:\Users\foo or C:/Users/foo).
	// filepath.IsAbs returns false for these on non-Windows hosts.
	if len(source) >= 3 && source[1] == ':' && (source[2] == '\\' || source[2] == '/') {
		return true
	}
	return strings.HasPrefix(source, "/") || // POSIX absolute on Windows host
		strings.HasPrefix(source, "./") || strings.HasPrefix(source, `.\`) || // current-dir relative
		strings.HasPrefix(source, "../") || strings.HasPrefix(source, `..\`) || // parent relative
		strings.HasPrefix(source, "~/") || strings.HasPrefix(source, `~\`) // home-dir relative
}

// toCloneURL converts a GitHub shorthand (owner/repo) or any URL to a clone URL.
func toCloneURL(source string) string {
	if strings.HasPrefix(source, "http://") ||
		strings.HasPrefix(source, "https://") ||
		strings.HasPrefix(source, "git@") ||
		strings.HasPrefix(source, "ssh://") {
		return source
	}
	// GitHub shorthand: owner/repo
	parts := strings.SplitN(source, "/", 2)
	if len(parts) == 2 {
		return "https://github.com/" + source
	}
	return source
}

// urlToCacheKey converts a URL to a safe filesystem path component.
func urlToCacheKey(url string) string {
	r := strings.NewReplacer(
		"https://", "",
		"http://", "",
		"git@", "",
		":", "/",
		".git", "",
	)
	return filepath.Clean(r.Replace(url))
}

// cachedSourcePath returns the local cache path for a source without cloning.
func cachedSourcePath(cacheDir, source string) (string, error) {
	if isLocalPath(source) {
		return source, nil
	}
	cloneURL := toCloneURL(source)
	cacheKey := urlToCacheKey(cloneURL)
	path := filepath.Join(cacheDir, cacheKey)
	if _, err := os.Stat(path); err != nil {
		return "", nil // not yet cached
	}
	return path, nil
}

// Prune removes skill directories that are present on disk under managed agent
// skill directories but are no longer declared in any config entry. It is safe
// to call after Sync: sources are already cached so no network access occurs.
func (m *Manager) Prune(ctx context.Context) error {
	slog.DebugContext(ctx, "pruning orphan skills")

	// Build the set of expected skill dir-names per absolute skills directory.
	expected := make(map[string]map[string]struct{})

	for _, sc := range m.skills {
		sourceDir, err := cachedSourcePath(m.cacheDir, sc.Source)
		if err != nil || sourceDir == "" {
			continue // source not yet cached — nothing to prune against it
		}
		available, err := discoverSkills(sourceDir)
		if err != nil {
			continue
		}
		selected := filterSkills(available, sc.Select)

		for _, agent := range m.syncAgents(sc) {
			skillsDir, ok, err := m.targetSkillsDir(agent, sc)
			if err != nil || !ok {
				continue
			}
			if expected[skillsDir] == nil {
				expected[skillsDir] = make(map[string]struct{})
			}
			for _, sk := range selected {
				expected[skillsDir][filepath.Base(sk.Dir)] = struct{}{}
			}
		}
	}

	// Walk each managed skills directory and remove entries absent from expected.
	// Real directories AND symlinks are candidates: agents like the user's
	// shared `~/.agents/skills/` setup install via symlinks under each
	// agent's skills dir, and a stale symlink is just as orphaned as a
	// stale dir (#96).
	for skillsDir, keep := range expected {
		entries, err := os.ReadDir(skillsDir)
		if err != nil {
			continue // directory may not exist
		}
		for _, entry := range entries {
			if !isSkillEntry(entry) {
				continue
			}
			if _, ok := keep[entry.Name()]; ok {
				continue
			}
			target := filepath.Join(skillsDir, entry.Name())
			if err := os.RemoveAll(target); err != nil {
				slog.Warn("failed to prune skill", "path", target, "err", err)
				continue
			}
			slog.Info("pruned orphan skill", "path", target)
			m.pruneSkillSnapshot(target)
		}
	}

	return nil
}

// isSkillEntry reports whether a ReadDir entry is a candidate for pruning.
// Real directories, and symlinks (which a sibling tool may have created to
// point at a shared skills tree), both count. Plain files are skipped.
func isSkillEntry(e fs.DirEntry) bool {
	if e.IsDir() {
		return true
	}
	return e.Type()&fs.ModeSymlink != 0
}

// pruneSkillSnapshot removes the snapshot file for a skill directory that has
// been deleted by Prune. Errors are logged but never returned.
func (m *Manager) pruneSkillSnapshot(dest string) {
	if m.stateDir == "" {
		return
	}
	key := "skill-" + discover.WorkdirKey(dest)
	path := discover.SnapshotPath(m.stateDir, key)
	slog.Debug("removing stale skill snapshot", "path", path)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		slog.Warn("failed to remove stale skill snapshot", "path", path, "err", err)
	}
}

// writeSkillSnapshot snapshots the installed skill directory at dest and
// persists it to stateDir so that discover.DiffPath can use the fast path on
// subsequent status checks. Errors are logged but never returned — snapshot
// failures must never break the sync.
func (m *Manager) writeSkillSnapshot(dest string) {
	if m.stateDir == "" {
		return
	}
	slog.Debug("writing skill snapshot", "dest", dest)
	snap, err := discover.SnapshotDir(dest)
	if err != nil {
		slog.Warn("skill snapshot failed", "dest", dest, "err", err)
		return
	}
	key := "skill-" + discover.WorkdirKey(dest)
	if err := discover.Save(discover.SnapshotPath(m.stateDir, key), snap); err != nil {
		slog.Warn("skill snapshot save failed", "dest", dest, "err", err)
	}
}
