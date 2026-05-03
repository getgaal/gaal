//go:build e2e

package e2e

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os/exec"
	"path"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestContainer wraps the long-lived Docker container shared across the
// suite. All exec, file-IO and lifecycle operations route through it so
// tests stay declarative.
type TestContainer struct {
	id      string
	stopped atomic.Bool
}

// startContainer launches the suite container detached, waits briefly for
// it to be running, and returns a handle. The container is started with
// `--rm` so a dropped suite cleans up automatically.
func startContainer() (*TestContainer, error) {
	args := []string{"run", "-d", "--rm"}
	if p := dockerPlatform(); p != "" {
		args = append(args, "--platform", p)
	}
	args = append(args,
		// Bind-mount the fixtures dir read-only so tests can reference
		// /fixtures/skills/<name> without docker cp gymnastics.
		"--mount", fmt.Sprintf("type=bind,source=%s,target=%s,readonly", fixturesPath(), fixturesDir),
		// Telemetry off so the test suite never opens an outbound connection.
		"-e", "GAAL_TELEMETRY=0",
		"-e", "DO_NOT_TRACK=1",
		// Quiet pterm/term-detection.
		"-e", "TERM=dumb",
		imageName,
	)
	cmd := exec.Command("docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker run: %w\n%s", err, out)
	}
	id := strings.TrimSpace(string(out))
	if len(id) < 12 {
		return nil, fmt.Errorf("docker run: unexpected output %q", string(out))
	}
	c := &TestContainer{id: id[:12]}

	// `docker run -d` returns once the container is created, not once it
	// is ready to accept exec — short retry loop avoids a startup race.
	for i := 0; i < 20; i++ {
		probe := exec.Command("docker", "exec", c.id, "true")
		if err := probe.Run(); err == nil {
			return c, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil, fmt.Errorf("container %s failed to become ready", c.id)
}

// Stop kills the container. Safe to call more than once.
func (c *TestContainer) Stop() error {
	if c.stopped.Swap(true) {
		return nil
	}
	cmd := exec.Command("docker", "rm", "-f", c.id)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker rm: %w\n%s", err, out)
	}
	return nil
}

// ExecResult holds the outcome of one `docker exec` invocation.
type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Combined returns Stdout + a separator + Stderr — handy for error messages.
func (r ExecResult) Combined() string {
	if r.Stderr == "" {
		return r.Stdout
	}
	return r.Stdout + "\n--- stderr ---\n" + r.Stderr
}

// Exec runs argv inside the container with the given env and working
// directory. Tests should not fail on stderr alone — many gaal commands
// log to stderr by design — but the returned ExecResult exposes the field
// for explicit assertions.
func (c *TestContainer) Exec(t *testing.T, env Env, workdir string, argv ...string) ExecResult {
	t.Helper()
	full := []string{"exec"}
	for _, e := range env.toSlice() {
		full = append(full, "-e", e)
	}
	if workdir != "" {
		full = append(full, "-w", workdir)
	}
	full = append(full, c.id)
	full = append(full, argv...)

	cmd := exec.Command("docker", full...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	res := ExecResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			res.ExitCode = ee.ExitCode()
		} else {
			res.ExitCode = -1
			res.Stderr += "\n" + err.Error()
		}
	}
	return res
}

// MustExec is Exec + a fatal assertion that the exit code was zero.
func (c *TestContainer) MustExec(t *testing.T, env Env, workdir string, argv ...string) ExecResult {
	t.Helper()
	res := c.Exec(t, env, workdir, argv...)
	if res.ExitCode != 0 {
		t.Fatalf("exec %v failed: exit=%d\n%s", argv, res.ExitCode, res.Combined())
	}
	return res
}

// ExecTTY runs argv inside the container with `docker exec -t`, allocating
// a pseudo-TTY for the in-container process so isatty(stdout) returns true.
// pterm and similar TTY-detecting libraries will then emit ANSI escapes.
//
// Stdout and stderr are merged into the result's Stdout (the pty conflates
// them — there is no separate stderr stream over a single pty fd). Used by
// TTY rendering tests; non-TTY tests should keep using Exec.
func (c *TestContainer) ExecTTY(t *testing.T, env Env, workdir string, argv ...string) ExecResult {
	t.Helper()
	full := []string{"exec", "-t"}
	for _, e := range env.toSlice() {
		full = append(full, "-e", e)
	}
	if workdir != "" {
		full = append(full, "-w", workdir)
	}
	full = append(full, c.id)
	full = append(full, argv...)

	cmd := exec.Command("docker", full...)
	out, err := cmd.CombinedOutput()
	res := ExecResult{Stdout: string(out)}
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			res.ExitCode = ee.ExitCode()
		} else {
			res.ExitCode = -1
			res.Stderr = err.Error()
		}
	}
	return res
}

// WriteFile writes content into a file in the container, creating parent
// directories as needed. Uses `sh -c` with stdin so the body never needs
// shell-escaping.
func (c *TestContainer) WriteFile(t *testing.T, p string, content string) {
	t.Helper()
	c.WriteFileBytes(t, p, []byte(content))
}

// WriteFileBytes is the binary-safe variant of WriteFile. Use it when the
// payload may contain null bytes or other content that would not survive a
// string-typed path (e.g. tar/zip archives in #143's hermetic VCS tests).
func (c *TestContainer) WriteFileBytes(t *testing.T, p string, content []byte) {
	t.Helper()
	parent := path.Dir(p)
	if mk := c.Exec(t, nil, "", "mkdir", "-p", parent); mk.ExitCode != 0 {
		t.Fatalf("mkdir -p %s: %s", parent, mk.Combined())
	}
	args := []string{"exec", "-i", c.id, "sh", "-c", "cat > " + shellQuote(p)}
	cmd := exec.Command("docker", args...)
	cmd.Stdin = bytes.NewReader(content)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("write %s: %v\n%s", p, err, stderr.String())
	}
}

// ReadFile returns the contents of p in the container, or fails the test
// if the read errors out (a missing file is a hard fail — use FileExists
// first when you need a soft check).
func (c *TestContainer) ReadFile(t *testing.T, p string) string {
	t.Helper()
	res := c.MustExec(t, nil, "", "cat", p)
	return res.Stdout
}

// FileExists reports whether p exists in the container. It does not
// distinguish between regular files, directories and symlinks.
func (c *TestContainer) FileExists(t *testing.T, p string) bool {
	t.Helper()
	res := c.Exec(t, nil, "", "test", "-e", p)
	return res.ExitCode == 0
}

// IsDir reports whether p exists and is a directory.
func (c *TestContainer) IsDir(t *testing.T, p string) bool {
	t.Helper()
	res := c.Exec(t, nil, "", "test", "-d", p)
	return res.ExitCode == 0
}

// RemoveFile deletes p (or recursively, a directory) from the container.
func (c *TestContainer) RemoveFile(t *testing.T, p string) {
	t.Helper()
	c.MustExec(t, nil, "", "rm", "-rf", p)
}

// ListDir returns the entries of dir in the container, sorted by name.
// Returns an empty slice when the directory does not exist.
func (c *TestContainer) ListDir(t *testing.T, dir string) []string {
	t.Helper()
	if !c.FileExists(t, dir) {
		return nil
	}
	res := c.MustExec(t, nil, "", "sh", "-c", "ls -1A "+shellQuote(dir)+" 2>/dev/null | sort")
	if strings.TrimSpace(res.Stdout) == "" {
		return nil
	}
	return strings.Split(strings.TrimRight(res.Stdout, "\n"), "\n")
}

// Env is a small ordered string→string map. Maps are unordered in Go but
// the env list we hand to docker exec must be reproducible across test
// runs to keep failure messages stable.
type Env map[string]string

func (e Env) toSlice() []string {
	if len(e) == 0 {
		return nil
	}
	keys := make([]string, 0, len(e))
	for k := range e {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, fmt.Sprintf("%s=%s", k, e[k]))
	}
	return out
}

// shellQuote single-quote-escapes a path so it survives `sh -c` interpolation.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// testEnv bundles the per-test container HOME and workspace. Allocated by
// newTestEnv() and disposed via t.Cleanup.
type testEnv struct {
	home    string // e.g. /tmp/test-sync-mcp-3a8f9c
	workdir string // e.g. /tmp/test-sync-mcp-3a8f9c-work — project-scope cwd
	c       *TestContainer
}

// newTestEnv allocates a fresh HOME + workspace inside the container,
// scoped to the running test. The dirs are best-effort removed on cleanup;
// any leak is bounded to the suite container's lifetime.
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	suffix := randomSuffix()
	sanitized := sanitizeName(t.Name())
	home := fmt.Sprintf("/tmp/%s-%s", sanitized, suffix)
	workdir := home + "-work"

	suite.MustExec(t, nil, "", "mkdir", "-p", home, workdir)
	t.Cleanup(func() {
		suite.Exec(t, nil, "", "rm", "-rf", home, workdir)
	})

	return &testEnv{home: home, workdir: workdir, c: suite}
}

// gaalEnv returns the Env map every gaal exec inside a test should carry.
// HOME redirection plus empty XDG vars is enough for gaal to anchor every
// path under home (per cmd/root.go and internal/config/platform/unix.go).
func (e *testEnv) gaalEnv() Env {
	return Env{
		"HOME":            e.home,
		"GAAL_TELEMETRY":  "0",
		"DO_NOT_TRACK":    "1",
		"XDG_CONFIG_HOME": "",
		"XDG_CACHE_HOME":  "",
	}
}

// gaal runs the gaal CLI against the test env and returns the result.
// The first argv element is typically "sync"/"status"/"prune"; the test
// supplies a config path via -c. The --no-banner flag suppresses the
// ASCII art so test output stays grep-friendly.
func (e *testEnv) gaal(t *testing.T, configPath string, argv ...string) ExecResult {
	t.Helper()
	full := []string{"gaal", "--no-banner"}
	if configPath != "" {
		full = append(full, "-c", configPath)
	}
	full = append(full, argv...)
	return e.c.Exec(t, e.gaalEnv(), e.workdir, full...)
}

// mustGaal is gaal + a fatal assertion that the exit code was zero.
func (e *testEnv) mustGaal(t *testing.T, configPath string, argv ...string) ExecResult {
	t.Helper()
	res := e.gaal(t, configPath, argv...)
	if res.ExitCode != 0 {
		t.Fatalf("gaal %v failed: exit=%d\n%s", argv, res.ExitCode, res.Combined())
	}
	return res
}

// writeProjectConfig drops a gaal.yaml inside the test workdir and returns
// its absolute path so the caller can pass it to gaal -c.
func (e *testEnv) writeProjectConfig(t *testing.T, body string) string {
	t.Helper()
	p := path.Join(e.workdir, "gaal.yaml")
	e.c.WriteFile(t, p, body)
	return p
}

// writeUserConfig drops a config under $HOME/.config/gaal/config.yaml and
// returns its absolute path. This is the user-scope counterpart to
// writeProjectConfig.
func (e *testEnv) writeUserConfig(t *testing.T, body string) string {
	t.Helper()
	p := path.Join(e.home, ".config", "gaal", "config.yaml")
	e.c.WriteFile(t, p, body)
	return p
}

// sanitizeName produces a /tmp-safe slug from a Go test name. "/" and "."
// are replaced with "-" so each subtest gets a unique, readable directory.
func sanitizeName(name string) string {
	r := strings.NewReplacer("/", "-", ".", "-", " ", "_", "#", "n")
	out := r.Replace(name)
	if len(out) > 60 {
		out = out[:60]
	}
	return strings.ToLower(out)
}

// randomSuffix returns 6 hex chars used to disambiguate test temp dirs
// across re-runs of the same test name.
func randomSuffix() string {
	var b [3]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%06x", time.Now().UnixNano()&0xffffff)
	}
	return hex.EncodeToString(b[:])
}
