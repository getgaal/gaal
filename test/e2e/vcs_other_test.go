//go:build e2e

// Hermetic coverage for the non-git VCS backends — issue #143 follow-up.
//
//   - hg: clone via local-path URL (mercurial accepts plain paths).
//   - tar / zip: clone via http://127.0.0.1:<port>, fileserver hosted by a
//     python3 -m http.server started in the suite container.
//   - svn / bzr: skipped. svn requires a `file://` URL or an svnserve
//     daemon, both of which require either widening the urlx scheme
//     allowlist (which #117 deliberately tightened) or adding more
//     services to the fixture image. Tracked as follow-up scope under
//     #143.
package e2e

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"path"
	"sync/atomic"
	"testing"
	"time"
)

// ── hg ──────────────────────────────────────────────────────────────────────

// hgConfigEnv satisfies hg's "no user configured" check (mercurial requires
// ui.username to be set for `hg commit`).
var hgConfigEnv = Env{
	"HGUSER": "gaal-e2e <e2e@example.test>",
}

// initBareHgRepo creates a Mercurial repo at <root>/<name> with a single
// commit and returns the absolute path. hg supports cloning from a plain
// filesystem path (no scheme required) so the path itself is a valid url.
func initBareHgRepo(t *testing.T, env *testEnv, root, name string) string {
	t.Helper()
	repo := path.Join(root, name)
	env.c.MustExec(t, hgConfigEnv, "", "hg", "init", repo)
	env.c.WriteFile(t, path.Join(repo, "README.md"), "# initial hg\n")
	env.c.MustExec(t, hgConfigEnv, repo, "hg", "add", "README.md")
	env.c.MustExec(t, hgConfigEnv, repo, "hg", "commit", "-m", "initial")
	return repo
}

func TestVCS_HgBackend_CloneAndCheckout(t *testing.T) {
	if !haveBinary(t, "hg") {
		t.Skip("mercurial not available in fixture image")
	}
	env := newTestEnv(t)

	reposRoot := path.Join(env.home, "test-repos")
	env.c.MustExec(t, nil, "", "mkdir", "-p", reposRoot)
	repo := initBareHgRepo(t, env, reposRoot, "myhg")

	cfg := newConfig().
		AddRepository("src/myhg", "hg", repo, "").
		String()
	cfgPath := env.writeProjectConfig(t, cfg)
	env.mustGaal(t, cfgPath, "sync")

	dst := path.Join(env.workdir, "src", "myhg", "README.md")
	if !env.c.FileExists(t, dst) {
		t.Fatalf("expected hg-cloned README at %s", dst)
	}
	if got := env.c.ReadFile(t, dst); got != "# initial hg\n" {
		t.Errorf("hg README mismatch: got %q", got)
	}
}

// ── tar / zip via local HTTP server ────────────────────────────────────────

// httpServerPort and httpServerOnce coordinate the suite-wide python3
// -m http.server. One server is enough — every test writes its own
// uniquely-named archive into the served directory.
var (
	httpServerOnce  atomic.Bool
	httpServerRoot  = "/tmp/gaal-e2e-http-root"
	httpServerPort  = "8765"
	httpServerReady = make(chan struct{}, 1)
)

// ensureHTTPServer starts python3 -m http.server in the suite container the
// first time it is called and returns the base URL (e.g.
// "http://127.0.0.1:8765"). Idempotent.
func ensureHTTPServer(t *testing.T) string {
	t.Helper()
	base := "http://127.0.0.1:" + httpServerPort
	if !httpServerOnce.CompareAndSwap(false, true) {
		<-httpServerReady
		httpServerReady <- struct{}{}
		return base
	}
	suite.MustExec(t, nil, "", "mkdir", "-p", httpServerRoot)
	// Fire-and-forget — `nohup` keeps the process alive past the
	// `docker exec`'s lifetime. The container is torn down at suite end.
	suite.MustExec(t, nil, "", "sh", "-c",
		"nohup python3 -m http.server "+httpServerPort+
			" --directory "+httpServerRoot+" >/tmp/gaal-http.log 2>&1 & echo $! >/tmp/gaal-http.pid")

	// Spin until the server accepts connections (up to 5s).
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		probe := suite.Exec(t, nil, "", "sh", "-c",
			"wget -qO- "+base+"/ >/dev/null 2>&1 || curl -fsS -o /dev/null "+base+"/")
		if probe.ExitCode == 0 {
			httpServerReady <- struct{}{}
			return base
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("python3 http.server did not become ready on %s", base)
	return ""
}

// hostArchive writes data under httpServerRoot and returns the served URL.
// name should be unique per test (the suite shares one server).
func hostArchive(t *testing.T, name string, data []byte) string {
	t.Helper()
	base := ensureHTTPServer(t)
	dst := path.Join(httpServerRoot, name)
	// Pipe via base64 to avoid shell-quoting binary data.
	suite.WriteFileBytes(t, dst, data)
	return base + "/" + name
}

func TestVCS_TarBackend_Clone(t *testing.T) {
	if !haveBinary(t, "python3") {
		t.Skip("python3 not available in fixture image — needed for HTTP fileserver")
	}
	env := newTestEnv(t)

	// Build a tiny .tar.gz with a single top-level dir (gaal strips it on
	// extract). README ends up at <dest>/README.md after extraction.
	data := buildTarGz(t, map[string]string{
		"project/README.md": "# initial tar\n",
	})
	url := hostArchive(t, sanitizeName(t.Name())+".tar.gz", data)

	cfg := newConfig().
		AddRepository("src/mytar", "tar", url, "").
		String()
	cfgPath := env.writeProjectConfig(t, cfg)
	env.mustGaal(t, cfgPath, "sync")

	dst := path.Join(env.workdir, "src", "mytar", "README.md")
	if !env.c.FileExists(t, dst) {
		t.Fatalf("expected tar-extracted README at %s", dst)
	}
	if got := env.c.ReadFile(t, dst); got != "# initial tar\n" {
		t.Errorf("tar README mismatch: got %q", got)
	}
}

func TestVCS_ZipBackend_Clone(t *testing.T) {
	if !haveBinary(t, "python3") {
		t.Skip("python3 not available in fixture image — needed for HTTP fileserver")
	}
	env := newTestEnv(t)

	data := buildZip(t, map[string]string{
		"project/README.md": "# initial zip\n",
	})
	url := hostArchive(t, sanitizeName(t.Name())+".zip", data)

	cfg := newConfig().
		AddRepository("src/myzip", "zip", url, "").
		String()
	cfgPath := env.writeProjectConfig(t, cfg)
	env.mustGaal(t, cfgPath, "sync")

	dst := path.Join(env.workdir, "src", "myzip", "README.md")
	if !env.c.FileExists(t, dst) {
		t.Fatalf("expected zip-extracted README at %s", dst)
	}
	if got := env.c.ReadFile(t, dst); got != "# initial zip\n" {
		t.Errorf("zip README mismatch: got %q", got)
	}
}

// ── svn ─────────────────────────────────────────────────────────────────────

var (
	svnServerOnce  atomic.Bool
	svnServerRoot  = "/tmp/gaal-e2e-svn-root"
	svnServerPort  = "3690"
	svnServerReady = make(chan struct{}, 1)
)

// ensureSvnServer starts an anonymous-read svnserve daemon in the suite
// container the first time it is called and returns the base URL
// (svn://127.0.0.1:3690). Idempotent across tests.
func ensureSvnServer(t *testing.T) string {
	t.Helper()
	base := "svn://127.0.0.1:" + svnServerPort
	if !svnServerOnce.CompareAndSwap(false, true) {
		<-svnServerReady
		svnServerReady <- struct{}{}
		return base
	}
	suite.MustExec(t, nil, "", "mkdir", "-p", svnServerRoot)
	// Start daemon. -r roots the URL space at svnServerRoot; --listen-port
	// binds to 3690; nohup keeps it alive past this exec call.
	suite.MustExec(t, nil, "", "sh", "-c",
		"nohup svnserve -d -r "+svnServerRoot+" --listen-port "+svnServerPort+
			" --foreground >/tmp/gaal-svn.log 2>&1 & echo $! >/tmp/gaal-svn.pid")

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		probe := suite.Exec(t, nil, "", "sh", "-c",
			"svn info "+base+"/ >/dev/null 2>&1 || nc -z 127.0.0.1 "+svnServerPort)
		if probe.ExitCode == 0 {
			svnServerReady <- struct{}{}
			return base
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatalf("svnserve did not become ready on %s", base)
	return ""
}

// initSvnRepo creates a bare svn repo under svnServerRoot/<name>, allows
// anonymous reads via svnserve.conf, bootstraps it with one commit
// (README.md) using a file:// URL on disk (gaal never sees the file:// URL
// — only the test harness does, to bypass the daemon for the bootstrap).
// Returns the daemon URL (svn://127.0.0.1:3690/<name>) for gaal to clone.
func initSvnRepo(t *testing.T, env *testEnv, name string) string {
	t.Helper()
	ensureSvnServer(t)
	repo := path.Join(svnServerRoot, name)
	suite.MustExec(t, nil, "", "svnadmin", "create", repo)
	// Allow anonymous reads.
	suite.MustExec(t, nil, "", "sh", "-c",
		"printf '[general]\\nanon-access = read\\nauth-access = read\\n' > "+
			path.Join(repo, "conf/svnserve.conf"))

	// Bootstrap via file:// URL — local-only, never reaches gaal config.
	work := path.Join(env.home, "svn-bootstrap-"+name)
	suite.MustExec(t, nil, "", "mkdir", "-p", work)
	env.c.WriteFile(t, path.Join(work, "README.md"), "# initial svn\n")
	suite.MustExec(t, nil, "", "svn", "import", "-m", "init",
		work, "file://"+repo)
	return "svn://127.0.0.1:" + svnServerPort + "/" + name
}

func TestVCS_SvnBackend_CloneAndCheckout(t *testing.T) {
	if !haveBinary(t, "svn") || !haveBinary(t, "svnserve") {
		t.Skip("subversion + svnserve not available in fixture image")
	}
	env := newTestEnv(t)
	url := initSvnRepo(t, env, sanitizeName(t.Name()))

	cfg := newConfig().
		AddRepository("src/mysvn", "svn", url, "").
		String()
	cfgPath := env.writeProjectConfig(t, cfg)
	env.mustGaal(t, cfgPath, "sync")

	dst := path.Join(env.workdir, "src", "mysvn", "README.md")
	if !env.c.FileExists(t, dst) {
		t.Fatalf("expected svn-checked-out README at %s", dst)
	}
	if got := env.c.ReadFile(t, dst); got != "# initial svn\n" {
		t.Errorf("svn README mismatch: got %q", got)
	}
}

// ── bzr — intentionally not installed ──────────────────────────────────────

// TestVCS_BzrBackend_Skipped documents why bzr coverage is not in the
// fixture image. Breezy (the modern Bazaar fork) needs gcc + musl-dev +
// python3-dev + a Rust toolchain to build from source — alpine 3.20 has
// no pre-built wheel. That would balloon the image by ~200 MB to cover a
// backend with effectively no real-world adoption.
//
// The urlx allowlist already permits bzr:// to loopback (see scheme.go) so
// when this test is enabled (different base image, prebuilt breezy wheel,
// or the alpine package returning) the only change needed is dropping
// this skip and adding initBzrRepo + ensureBzrServer (analogous to the
// svn helpers in this file).
func TestVCS_BzrBackend_Skipped(t *testing.T) {
	t.Skip("bzr/Breezy is not in the fixture image (would need gcc + Rust toolchain) — see #143")
}

// ── helpers ────────────────────────────────────────────────────────────────

// haveBinary reports whether `name` is on PATH inside the suite container.
func haveBinary(t *testing.T, name string) bool {
	t.Helper()
	res := suite.Exec(t, nil, "", "sh", "-c", "command -v "+name+" >/dev/null")
	return res.ExitCode == 0
}

// buildTarGz returns a .tar.gz body containing the named files.
func buildTarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{
			Name: name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatalf("tar header: %v", err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("tar body: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// buildZip returns a .zip body containing the named files.
func buildZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create: %v", err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("zip write: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// fmtBytes is a no-op formatting helper kept so future tests can grow
// asserts on archive sizes; suppress unused-import bother for fmt.
var _ = fmt.Sprintf
