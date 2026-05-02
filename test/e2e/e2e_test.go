//go:build e2e

// Package e2e contains the gaal end-to-end test suite.
//
// The suite is gated by the `e2e` build tag so the day-to-day `go test ./...`
// run never picks it up. Run it with:
//
//	make test-e2e                            # fast filesystem layer only
//	GAAL_E2E_CLI=1 make test-e2e             # also exercise agent CLIs
//
// All tests share one Docker container per `go test` invocation. Each test
// gets a unique HOME (a fresh /tmp/<test>-XXXXXX directory inside the
// container) so writes from one test never leak into another.
package e2e

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	// imageName tags the Docker image built once per `go test` invocation.
	// Bumping the suffix forces a rebuild for everyone on the next run.
	imageName = "gaal-e2e:dev"

	// fixturesDir is the in-container path under which the host-side
	// fixtures directory is bind-mounted (read-only).
	fixturesDir = "/fixtures"
)

// suite is the package-global TestContainer initialised by TestMain and
// reused by every test in the suite. Tests that need filesystem isolation
// allocate their own HOME under it via newTestEnv().
var suite *TestContainer

// envCLI is true when GAAL_E2E_CLI=1 is set. The cli_verify_test.go layer
// keys off this flag and skips itself otherwise.
var envCLI = os.Getenv("GAAL_E2E_CLI") == "1"

// TestMain orchestrates suite-wide setup:
//  1. compile the gaal binary for linux/amd64 if it isn't already in
//     fixtures/gaal (the Makefile target builds it before invoking go test;
//     this is a safety net for ad-hoc invocations from an editor).
//  2. build the Docker image (re-uses the layer cache so iterative runs
//     only redo the cheap final COPY).
//  3. start one long-lived container, bind-mounting the fixtures dir.
//  4. run the tests.
//  5. tear down the container regardless of test outcome.
func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		fmt.Println("e2e: skipped under -short")
		os.Exit(0)
	}

	if err := ensureBinary(); err != nil {
		log.Fatalf("e2e: building gaal binary: %v", err)
	}

	if err := ensureImage(); err != nil {
		log.Fatalf("e2e: building docker image: %v", err)
	}

	c, err := startContainer()
	if err != nil {
		log.Fatalf("e2e: starting container: %v", err)
	}
	suite = c

	code := m.Run()

	if err := c.Stop(); err != nil {
		log.Printf("e2e: stopping container: %v", err)
	}
	os.Exit(code)
}

// fixturesPath returns the absolute host path to the fixtures directory.
// Resolved from the runtime caller location so the suite works regardless
// of the current working directory at test invocation time.
func fixturesPath() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		log.Fatal("e2e: cannot resolve caller path")
	}
	return filepath.Join(filepath.Dir(file), "fixtures")
}

// ensureBinary builds the gaal binary into fixtures/gaal when missing or
// when GAAL_E2E_REBUILD=1 is set. The Makefile normally pre-builds it; this
// fallback exists for direct `go test -tags e2e ./test/e2e/...` invocations.
func ensureBinary() error {
	dest := filepath.Join(fixturesPath(), "gaal")
	if os.Getenv("GAAL_E2E_REBUILD") != "1" {
		if _, err := os.Stat(dest); err == nil {
			return nil
		}
	}

	repoRoot, err := repoRoot()
	if err != nil {
		return err
	}

	cmd := exec.Command("go", "build", "-trimpath", "-o", dest, ".")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(),
		"CGO_ENABLED=0",
		"GOOS=linux",
		"GOARCH=amd64",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("go build: %w\n%s", err, out)
	}
	return nil
}

// repoRoot walks up from the current source file looking for go.mod.
func repoRoot() (string, error) {
	dir := fixturesPath()
	for i := 0; i < 8; i++ {
		dir = filepath.Dir(dir)
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
	}
	return "", fmt.Errorf("could not locate repo root from %s", fixturesPath())
}

// ensureImage runs `docker build` with the fixtures directory as context.
// The build args are derived from envCLI so layer 2 runs install the agent
// CLIs while layer 1 runs stay slim.
func ensureImage() error {
	args := []string{
		"build",
		"--quiet",
		"--tag", imageName,
		"--build-arg", fmt.Sprintf("INSTALL_AGENT_CLIS=%s", boolToZeroOne(envCLI)),
	}
	args = append(args, fixturesPath())

	cmd := exec.Command("docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker build: %w\n%s", err, out)
	}
	if trimmed := strings.TrimSpace(string(out)); trimmed != "" {
		log.Printf("e2e: image built (%s)", trimmed)
	}
	return nil
}

func boolToZeroOne(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
