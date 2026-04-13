package installscript

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// scriptPath returns the absolute path to scripts/install.sh, computed from
// this test file's own location so it works regardless of the `go test`
// working directory.
func scriptPath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	return filepath.Join(repoRoot, "scripts", "install.sh")
}

// fakeGaalBinary returns the bytes of a minimal POSIX shell script that
// behaves like `gaal version` when invoked with "version" as its first arg.
// The test uses this as the "binary" the install script will download and
// install.
func fakeGaalBinary(version string) []byte {
	return []byte(fmt.Sprintf(`#!/bin/sh
if [ "$1" = "version" ]; then
  echo "gaal %s (built: 2026-04-12T00:00:00Z)"
  exit 0
fi
echo "fake gaal" >&2
exit 0
`, version))
}

// sha256hex returns the lowercase hex-encoded SHA-256 of data.
func sha256hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// fakeReleaseServer returns an httptest.Server that serves a fake GitHub
// Releases API response, a fake binary for the given platform, and a
// SHA256SUMS file. The caller closes it with server.Close().
func fakeReleaseServer(t *testing.T, version, goos, goarch string, binary []byte) *httptest.Server {
	t.Helper()
	binName := fmt.Sprintf("gaal-%s-%s", goos, goarch)
	sumsContent := fmt.Sprintf("%s  %s\n", sha256hex(binary), binName)

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/gmg-inc/gaal-lite/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"tag_name":%q}`, version)
	})
	mux.HandleFunc(fmt.Sprintf("/releases/download/%s/%s", version, binName), func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(binary)
	})
	mux.HandleFunc(fmt.Sprintf("/releases/download/%s/SHA256SUMS", version), func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, sumsContent)
	})
	return httptest.NewServer(mux)
}

// runInstall execs scripts/install.sh with the given env overrides. It
// returns stdout, stderr, and the exec error (if any). Output is captured
// as strings for easy assertion.
func runInstall(t *testing.T, installDir string, env map[string]string) (string, string, error) {
	t.Helper()
	cmd := exec.Command("/bin/sh", scriptPath(t))
	cmd.Env = append(os.Environ(), fmt.Sprintf("INSTALL_DIR=%s", installDir))
	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// detectHostGOOS returns the current OS in the goos naming the release
// workflow uses: "linux" or "darwin". Tests that depend on matching the
// running host's binary use this.
func detectHostGOOS(t *testing.T) string {
	t.Helper()
	switch runtime.GOOS {
	case "linux", "darwin":
		return runtime.GOOS
	default:
		t.Skipf("install.sh only supports linux and darwin; host is %s", runtime.GOOS)
		return ""
	}
}

// detectHostGOARCH returns the current arch as "amd64" or "arm64".
func detectHostGOARCH(t *testing.T) string {
	t.Helper()
	switch runtime.GOARCH {
	case "amd64", "arm64":
		return runtime.GOARCH
	default:
		t.Skipf("install.sh only supports amd64 and arm64; host is %s", runtime.GOARCH)
		return ""
	}
}

func TestInstallHappyPath(t *testing.T) {
	version := "v9.9.9"
	goos := detectHostGOOS(t)
	goarch := detectHostGOARCH(t)
	binary := fakeGaalBinary(version)

	server := fakeReleaseServer(t, version, goos, goarch, binary)
	defer server.Close()

	installDir := t.TempDir()

	stdout, stderr, err := runInstall(t, installDir, map[string]string{
		"GAAL_INSTALL_BASE_URL": server.URL,
	})
	if err != nil {
		t.Fatalf("install.sh failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	installedPath := filepath.Join(installDir, "gaal")
	info, statErr := os.Stat(installedPath)
	if statErr != nil {
		t.Fatalf("expected %s to exist: %v", installedPath, statErr)
	}
	if info.Mode().Perm()&0111 == 0 {
		t.Fatalf("expected %s to be executable, got mode %v", installedPath, info.Mode())
	}

	if !strings.Contains(stdout, version) {
		t.Errorf("expected stdout to mention %s, got: %s", version, stdout)
	}
}

func TestInstallChecksumMismatch(t *testing.T) {
	version := "v9.9.9"
	goos := detectHostGOOS(t)
	goarch := detectHostGOARCH(t)
	realBinary := fakeGaalBinary(version)

	// Stand up a fake server that serves a DIFFERENT binary from the one
	// the SHA256SUMS file references. This mimics "someone tampered with
	// the release asset after the checksums were published."
	binName := fmt.Sprintf("gaal-%s-%s", goos, goarch)
	tamperedBinary := []byte("#!/bin/sh\necho tampered\n")
	sumsContent := fmt.Sprintf("%s  %s\n", sha256hex(realBinary), binName)

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/gmg-inc/gaal-lite/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"tag_name":%q}`, version)
	})
	mux.HandleFunc(fmt.Sprintf("/releases/download/%s/%s", version, binName), func(w http.ResponseWriter, r *http.Request) {
		w.Write(tamperedBinary)
	})
	mux.HandleFunc(fmt.Sprintf("/releases/download/%s/SHA256SUMS", version), func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, sumsContent)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	installDir := t.TempDir()

	_, stderr, err := runInstall(t, installDir, map[string]string{
		"GAAL_INSTALL_BASE_URL": server.URL,
	})
	if err == nil {
		t.Fatal("expected install.sh to fail on checksum mismatch, got success")
	}
	if !strings.Contains(stderr, "checksum mismatch") {
		t.Errorf("expected stderr to contain 'checksum mismatch', got: %s", stderr)
	}

	// Most important: the tampered binary must NOT have landed in INSTALL_DIR.
	if _, statErr := os.Stat(filepath.Join(installDir, "gaal")); !os.IsNotExist(statErr) {
		t.Fatalf("expected no file at %s on checksum mismatch, but it exists (err=%v)", filepath.Join(installDir, "gaal"), statErr)
	}
}

func TestInstallAlreadyInstalled(t *testing.T) {
	version := "v9.9.9"
	goos := detectHostGOOS(t)
	goarch := detectHostGOARCH(t)
	binary := fakeGaalBinary(version)

	server := fakeReleaseServer(t, version, goos, goarch, binary)
	defer server.Close()

	installDir := t.TempDir()
	env := map[string]string{"GAAL_INSTALL_BASE_URL": server.URL}

	// First run: install.
	if _, stderr, err := runInstall(t, installDir, env); err != nil {
		t.Fatalf("first install failed: %v\nstderr: %s", err, stderr)
	}

	// Record the mtime of the installed binary to prove the second run
	// does not touch the file.
	installedPath := filepath.Join(installDir, "gaal")
	info1, err := os.Stat(installedPath)
	if err != nil {
		t.Fatalf("stat after first install: %v", err)
	}

	// Second run: must short-circuit.
	stdout, stderr, err := runInstall(t, installDir, env)
	if err != nil {
		t.Fatalf("second install failed: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "already installed") {
		t.Errorf("expected stdout to mention 'already installed', got: %s", stdout)
	}

	info2, err := os.Stat(installedPath)
	if err != nil {
		t.Fatalf("stat after second install: %v", err)
	}
	if !info1.ModTime().Equal(info2.ModTime()) {
		t.Errorf("expected file mtime unchanged on no-op rerun (before=%v, after=%v)", info1.ModTime(), info2.ModTime())
	}
}

func TestInstallUnsupportedArch(t *testing.T) {
	// Use a real httptest server so we reach the script; the script should
	// fail inside detect_arch (which runs before any network calls) without
	// ever hitting the server.
	server := fakeReleaseServer(t, "v9.9.9", "linux", "amd64", fakeGaalBinary("v9.9.9"))
	defer server.Close()

	installDir := t.TempDir()
	_, stderr, err := runInstall(t, installDir, map[string]string{
		"GAAL_INSTALL_BASE_URL":      server.URL,
		"GAAL_INSTALL_ARCH_OVERRIDE": "powerpc",
	})
	if err == nil {
		t.Fatal("expected install.sh to fail on unsupported arch, got success")
	}
	if !strings.Contains(stderr, "unsupported architecture: powerpc") {
		t.Errorf("expected stderr to contain 'unsupported architecture: powerpc', got: %s", stderr)
	}
	if _, statErr := os.Stat(filepath.Join(installDir, "gaal")); !os.IsNotExist(statErr) {
		t.Fatalf("expected no file at %s on unsupported arch, but it exists (err=%v)", filepath.Join(installDir, "gaal"), statErr)
	}
}
