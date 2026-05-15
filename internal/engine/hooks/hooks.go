// Package hooks runs user-defined commands around the gaal sync pipeline.
//
// Hooks let users glue cross-machine workflows into a single config — for
// example, "after every successful sync, run `git pull` in another working
// copy." The shape of a hook is intentionally minimal and exec-form: command
// plus args, no shell, so the same config works on Linux, macOS, and Windows.
package hooks

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"gaal/internal/config"
	"gaal/internal/engine/render"
)

// Manager owns the hooks declared in the merged Config and is responsible for
// planning, filtering, and executing them at the right phase of sync.
//
// Concurrency model: hooks run sequentially in declared order. This is a
// deliberate v1 choice — predictable output beats faster wall time, and many
// post-sync hooks (e.g. `git pull` on a working copy) are not safe to
// parallelise anyway.
type Manager struct {
	hooks    *config.ConfigHooks
	workDir  string
	home     string
	goos     string
	executor Executor
}

// Executor runs a hook subprocess. It is split out so tests can substitute a
// fake that records invocations without touching the host filesystem.
type Executor func(ctx context.Context, h ResolvedHook) error

// ResolvedHook is a ConfigHook after path/env expansion and timeout parsing.
// It is what an Executor actually runs.
type ResolvedHook struct {
	Phase   render.HookPhase
	Name    string
	Command string
	Args    []string
	Cwd     string
	Env     []string // full environment, ready for exec.Cmd.Env
	Timeout time.Duration
}

// Result describes the outcome of a single hook invocation.
type Result struct {
	Hook    ResolvedHook
	Err     error
	Skipped bool // true when the hook was filtered out (OS mismatch, etc.)
}

// NewManager builds a Manager. cfg may be nil, in which case all methods are
// no-ops. workDir and home are used to resolve relative paths and ~ tokens;
// goos is exposed for tests — production callers pass runtime.GOOS.
func NewManager(cfg *config.ConfigHooks, workDir, home, goos string) *Manager {
	if goos == "" {
		goos = runtime.GOOS
	}
	return &Manager{
		hooks:    cfg,
		workDir:  workDir,
		home:     home,
		goos:     goos,
		executor: defaultExecutor,
	}
}

// SetExecutor replaces the executor used by RunPreSync/RunPostSync. Intended
// for tests; production code never calls this.
func (m *Manager) SetExecutor(e Executor) {
	if e == nil {
		m.executor = defaultExecutor
		return
	}
	m.executor = e
}

// Plan returns the PlanHookEntry slice for every declared hook, marking each
// with whether it will run or be skipped on the current OS. The result is
// safe to embed in render.PlanReport without further filtering.
func (m *Manager) Plan() []render.PlanHookEntry {
	if m == nil || m.hooks == nil {
		return nil
	}
	out := make([]render.PlanHookEntry, 0, len(m.hooks.PreSync)+len(m.hooks.PostSync))
	out = append(out, m.planPhase(render.HookPreSync, m.hooks.PreSync)...)
	out = append(out, m.planPhase(render.HookPostSync, m.hooks.PostSync)...)
	return out
}

func (m *Manager) planPhase(phase render.HookPhase, hooks []config.ConfigHook) []render.PlanHookEntry {
	out := make([]render.PlanHookEntry, 0, len(hooks))
	for _, h := range hooks {
		entry := render.PlanHookEntry{
			Phase:   phase,
			Name:    h.Name,
			Command: h.Command,
			Args:    append([]string(nil), h.Args...),
			Action:  render.PlanRun,
		}
		if !osMatches(m.goos, h.OS) {
			entry.Action = render.PlanSkip
			entry.Reason = "os filter excludes " + m.goos
		}
		out = append(out, entry)
	}
	return out
}

// RunPreSync runs every pre-sync hook in order. A hook's non-zero exit
// aborts the sync (returning a non-nil error) unless the hook is marked
// continue_on_error. Hooks filtered out by their OS list are silently
// skipped. Pre-sync hooks see GAAL_PLANNED_* env vars derived from plan.
func (m *Manager) RunPreSync(ctx context.Context, plan *render.PlanReport) error {
	if m == nil || m.hooks == nil || len(m.hooks.PreSync) == 0 {
		return nil
	}
	env := planEnv(plan)
	env["GAAL_HOOK_PHASE"] = string(render.HookPreSync)
	return m.runPhase(ctx, render.HookPreSync, m.hooks.PreSync, env)
}

// RunPostSync runs every post-sync hook in order. Behaviour mirrors
// RunPreSync but the env payload describes what *did* change rather than
// what was planned. Callers must skip this call entirely when the sync
// itself failed — that policy lives in the cmd layer so the meaning of
// "successful sync" stays one decision in one place.
func (m *Manager) RunPostSync(ctx context.Context, plan *render.PlanReport) error {
	if m == nil || m.hooks == nil || len(m.hooks.PostSync) == 0 {
		return nil
	}
	env := planEnv(plan)
	env["GAAL_HOOK_PHASE"] = string(render.HookPostSync)
	return m.runPhase(ctx, render.HookPostSync, m.hooks.PostSync, env)
}

func (m *Manager) runPhase(ctx context.Context, phase render.HookPhase, hooks []config.ConfigHook, extraEnv map[string]string) error {
	var errs []error
	for i, h := range hooks {
		if !osMatches(m.goos, h.OS) {
			slog.DebugContext(ctx, "hook skipped: os filter",
				"phase", phase, "index", i, "name", h.Name, "goos", m.goos, "filter", h.OS)
			continue
		}

		resolved, err := m.resolve(phase, h, extraEnv)
		if err != nil {
			err = fmt.Errorf("hooks.%s[%d] %s: resolve: %w", phase, i, hookLabel(h), err)
			if h.ContinueOnError {
				slog.WarnContext(ctx, "hook resolve failed; continuing", "err", err)
				errs = append(errs, err)
				continue
			}
			return err
		}

		slog.InfoContext(ctx, "running hook",
			"phase", phase, "name", resolved.Name,
			"command", resolved.Command, "args", resolved.Args)

		runCtx, cancel := context.WithTimeout(ctx, resolved.Timeout)
		runErr := m.executor(runCtx, resolved)
		cancel()

		if runErr == nil {
			continue
		}
		hookErr := fmt.Errorf("hooks.%s[%d] %s: %w", phase, i, hookLabel(h), runErr)
		if h.ContinueOnError {
			slog.WarnContext(ctx, "hook failed; continue_on_error=true, continuing",
				"phase", phase, "name", resolved.Name, "err", runErr)
			errs = append(errs, hookErr)
			continue
		}
		return hookErr
	}
	return errors.Join(errs...)
}

func hookLabel(h config.ConfigHook) string {
	if h.Name != "" {
		return strconv.Quote(h.Name)
	}
	return strconv.Quote(h.Command)
}

// resolve expands ~ and env tokens, builds the env slice, and parses the
// timeout. Validation already ensured Timeout is well-formed.
func (m *Manager) resolve(phase render.HookPhase, h config.ConfigHook, extraEnv map[string]string) (ResolvedHook, error) {
	args := make([]string, len(h.Args))
	for i, a := range h.Args {
		args[i] = m.expand(a)
	}
	cwd := m.workDir
	if h.Cwd != "" {
		cwd = m.expand(h.Cwd)
		if !filepath.IsAbs(cwd) {
			cwd = filepath.Join(m.workDir, cwd)
		}
	}
	cmd := h.Command
	// Only expand path-like commands (~/bin/foo); a bare name like "git"
	// must remain bare so exec.LookPath finds it on PATH.
	if strings.HasPrefix(cmd, "~/") || strings.HasPrefix(cmd, `~\`) || strings.ContainsAny(cmd, "/\\") {
		cmd = m.expand(cmd)
	}

	env := os.Environ()
	for k, v := range extraEnv {
		env = append(env, k+"="+v)
	}
	for k, v := range h.Env {
		env = append(env, k+"="+m.expand(v))
	}

	return ResolvedHook{
		Phase:   phase,
		Name:    h.Name,
		Command: cmd,
		Args:    args,
		Cwd:     cwd,
		Env:     env,
		Timeout: h.EffectiveTimeout(),
	}, nil
}

// expand applies ~ and env expansion to s. Order matters: ~ first (so that
// a literal ~ followed by $VAR keeps its home meaning), then os.ExpandEnv.
func (m *Manager) expand(s string) string {
	if strings.HasPrefix(s, "~/") || strings.HasPrefix(s, `~\`) {
		s = filepath.Join(m.home, s[2:])
	}
	return os.ExpandEnv(s)
}

// osMatches reports whether the current GOOS satisfies a hook's optional
// OS filter. An empty filter matches every platform.
func osMatches(goos string, filter []string) bool {
	if len(filter) == 0 {
		return true
	}
	for _, f := range filter {
		if strings.EqualFold(f, goos) {
			return true
		}
	}
	return false
}

// defaultExecutor runs the hook with stdout and stderr wired straight to
// gaal's own streams. The user wrote the command; the user gets to see what
// it printed. Spinner-based capture (as in internal/runner) is the wrong
// default here — hook output is rarely incidental.
func defaultExecutor(ctx context.Context, h ResolvedHook) error {
	cmd := exec.CommandContext(ctx, h.Command, h.Args...) //nolint:gosec // hook commands are user-authored by design
	cmd.Dir = h.Cwd
	cmd.Env = h.Env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("timeout after %s", h.Timeout)
		}
		return err
	}
	return nil
}
