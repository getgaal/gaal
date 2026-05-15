package hooks

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"gaal/internal/config"
	"gaal/internal/engine/render"
)

// recorder is a test executor that captures every hook invocation in order
// and returns a configurable error.
type recorder struct {
	mu    sync.Mutex
	calls []ResolvedHook
	errs  map[string]error // keyed by hook.Name or hook.Command
}

func (r *recorder) exec(ctx context.Context, h ResolvedHook) error {
	r.mu.Lock()
	r.calls = append(r.calls, h)
	r.mu.Unlock()
	if r.errs == nil {
		return nil
	}
	if err, ok := r.errs[h.Name]; ok {
		return err
	}
	return r.errs[h.Command]
}

func (r *recorder) names() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.calls))
	for _, c := range r.calls {
		out = append(out, c.Name+"|"+c.Command)
	}
	return out
}

func newRec() *recorder { return &recorder{errs: map[string]error{}} }

func TestManager_NilConfigIsNoOp(t *testing.T) {
	m := NewManager(nil, ".", "/home", "linux")
	rec := newRec()
	m.SetExecutor(rec.exec)
	if err := m.RunPreSync(context.Background(), nil); err != nil {
		t.Fatalf("pre: %v", err)
	}
	if err := m.RunPostSync(context.Background(), nil); err != nil {
		t.Fatalf("post: %v", err)
	}
	if got := len(rec.calls); got != 0 {
		t.Fatalf("expected 0 executor calls, got %d", got)
	}
	if plan := m.Plan(); len(plan) != 0 {
		t.Fatalf("expected empty plan, got %v", plan)
	}
}

func TestPlan_OSFilterMarksSkip(t *testing.T) {
	cfg := &config.ConfigHooks{
		PreSync: []config.ConfigHook{
			{Name: "linux-only", Command: "true", OS: []string{"linux"}},
			{Name: "windows-only", Command: "true", OS: []string{"windows"}},
		},
		PostSync: []config.ConfigHook{
			{Name: "universal", Command: "true"},
		},
	}
	m := NewManager(cfg, ".", "/home", "linux")
	got := m.Plan()
	if len(got) != 3 {
		t.Fatalf("want 3 entries, got %d", len(got))
	}
	want := map[string]render.PlanAction{
		"linux-only":   render.PlanRun,
		"windows-only": render.PlanSkip,
		"universal":    render.PlanRun,
	}
	for _, e := range got {
		if want[e.Name] != e.Action {
			t.Errorf("%s: want action %s, got %s (reason=%q)", e.Name, want[e.Name], e.Action, e.Reason)
		}
		if e.Action == render.PlanSkip && e.Reason == "" {
			t.Errorf("%s: skipped entry has no reason", e.Name)
		}
	}
}

func TestRunPreSync_OSFilterSkipsExecutor(t *testing.T) {
	cfg := &config.ConfigHooks{
		PreSync: []config.ConfigHook{
			{Name: "linux-only", Command: "true", OS: []string{"linux"}},
			{Name: "windows-only", Command: "true", OS: []string{"windows"}},
		},
	}
	m := NewManager(cfg, ".", "/home", "linux")
	rec := newRec()
	m.SetExecutor(rec.exec)

	if err := m.RunPreSync(context.Background(), &render.PlanReport{}); err != nil {
		t.Fatalf("RunPreSync: %v", err)
	}
	if got := len(rec.calls); got != 1 {
		t.Fatalf("expected 1 call (windows-only filtered out), got %d", got)
	}
	if rec.calls[0].Name != "linux-only" {
		t.Fatalf("ran the wrong hook: %s", rec.calls[0].Name)
	}
}

func TestRunPreSync_FailureAborts(t *testing.T) {
	cfg := &config.ConfigHooks{
		PreSync: []config.ConfigHook{
			{Name: "first", Command: "true"},
			{Name: "boom", Command: "false"},
			{Name: "never-runs", Command: "true"},
		},
	}
	m := NewManager(cfg, ".", "/home", "linux")
	rec := newRec()
	rec.errs["boom"] = errors.New("exit 1")
	m.SetExecutor(rec.exec)

	err := m.RunPreSync(context.Background(), &render.PlanReport{})
	if err == nil {
		t.Fatal("expected error from failing hook")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error should reference hook name, got %v", err)
	}
	if got := len(rec.calls); got != 2 {
		t.Fatalf("expected exec to stop after failing hook (2 calls), got %d", got)
	}
}

func TestRunPreSync_ContinueOnErrorRunsRemaining(t *testing.T) {
	cfg := &config.ConfigHooks{
		PreSync: []config.ConfigHook{
			{Name: "first", Command: "true"},
			{Name: "soft-fail", Command: "false", ContinueOnError: true},
			{Name: "after-soft-fail", Command: "true"},
		},
	}
	m := NewManager(cfg, ".", "/home", "linux")
	rec := newRec()
	rec.errs["soft-fail"] = errors.New("exit 1")
	m.SetExecutor(rec.exec)

	err := m.RunPreSync(context.Background(), &render.PlanReport{})
	if err == nil {
		t.Fatal("expected joined error to be non-nil")
	}
	if got := len(rec.calls); got != 3 {
		t.Fatalf("expected all 3 calls (continue_on_error=true), got %d", got)
	}
}

func TestRunPostSync_PassesChangedEnv(t *testing.T) {
	cfg := &config.ConfigHooks{
		PostSync: []config.ConfigHook{
			{Name: "inspect", Command: "true"},
		},
	}
	m := NewManager(cfg, ".", "/home", "linux")
	rec := newRec()
	m.SetExecutor(rec.exec)

	plan := &render.PlanReport{
		Repositories: []render.PlanRepoEntry{
			{Path: "src/a", Action: render.PlanUpdate},
			{Path: "src/b", Action: render.PlanNoOp},
		},
		Skills: []render.PlanSkillEntry{
			{Source: "owner/repo", Agent: "claude-code", Action: render.PlanCreate},
		},
		MCPs: []render.PlanMCPEntry{
			{Name: "filesystem", Action: render.PlanUpdate},
		},
		HasChanges: true,
	}
	if err := m.RunPostSync(context.Background(), plan); err != nil {
		t.Fatalf("RunPostSync: %v", err)
	}
	if len(rec.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(rec.calls))
	}
	env := rec.calls[0].Env
	mustContain := []string{
		"GAAL_HOOK_PHASE=post-sync",
		"GAAL_CHANGED_REPOS=src/a",
		"GAAL_CHANGED_SKILLS=claude-code:owner/repo",
		"GAAL_CHANGED_MCPS=filesystem",
		"GAAL_HAS_CHANGES=1",
	}
	for _, want := range mustContain {
		if !containsEnv(env, want) {
			t.Errorf("env missing %q in %v", want, env)
		}
	}
}

func TestResolve_HomeAndEnvExpansion(t *testing.T) {
	t.Setenv("FOO_VALUE", "bar")
	cfg := &config.ConfigHooks{
		PreSync: []config.ConfigHook{
			{
				Name:    "expand",
				Command: "true",
				Args:    []string{"~/x", "$FOO_VALUE", "${FOO_VALUE}-suffix"},
				Cwd:     "~/work",
				Env:     map[string]string{"OUT": "$FOO_VALUE"},
			},
		},
	}
	m := NewManager(cfg, "/wd", "/home/test", "linux")
	rec := newRec()
	m.SetExecutor(rec.exec)
	if err := m.RunPreSync(context.Background(), &render.PlanReport{}); err != nil {
		t.Fatalf("RunPreSync: %v", err)
	}
	got := rec.calls[0]
	wantArgs := []string{"/home/test/x", "bar", "bar-suffix"}
	if !equalStrings(got.Args, wantArgs) {
		t.Errorf("args: want %v, got %v", wantArgs, got.Args)
	}
	if got.Cwd != "/home/test/work" {
		t.Errorf("cwd: want /home/test/work, got %s", got.Cwd)
	}
	if !containsEnv(got.Env, "OUT=bar") {
		t.Errorf("env missing OUT=bar: %v", got.Env)
	}
}

func TestEffectiveTimeout(t *testing.T) {
	cases := []struct {
		raw  string
		want time.Duration
	}{
		{"", config.DefaultHookTimeout},
		{"30s", 30 * time.Second},
		{"2m", 2 * time.Minute},
		{"bogus", config.DefaultHookTimeout},
	}
	for _, tc := range cases {
		got := config.ConfigHook{Timeout: tc.raw}.EffectiveTimeout()
		if got != tc.want {
			t.Errorf("Timeout=%q: want %s, got %s", tc.raw, tc.want, got)
		}
	}
}

func containsEnv(env []string, want string) bool {
	for _, e := range env {
		if e == want {
			return true
		}
	}
	return false
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
