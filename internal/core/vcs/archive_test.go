package vcs

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// buildTarGz creates an in-memory .tar.gz with the files map (name → content).
func buildTarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	for name, content := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("tar WriteHeader: %v", err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("tar Write: %v", err)
		}
	}
	tw.Close()
	gw.Close()
	return buf.Bytes()
}

// buildZip creates an in-memory .zip with the files map.
func buildZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip Create: %v", err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("zip Write: %v", err)
		}
	}
	zw.Close()
	return buf.Bytes()
}

func TestVcsArchive_IsCloned_ExistingDir(t *testing.T) {
	a := &VcsArchive{Format: "tar"}
	dir := t.TempDir()
	if !a.IsCloned(dir) {
		t.Error("expected IsCloned=true for existing directory")
	}
}

func TestVcsArchive_IsCloned_MissingDir(t *testing.T) {
	a := &VcsArchive{Format: "tar"}
	if a.IsCloned("/no/such/directory/gaal-test") {
		t.Error("expected IsCloned=false for missing directory")
	}
}

func TestVcsArchive_CurrentVersion(t *testing.T) {
	a := &VcsArchive{Format: "tar"}
	v, err := a.CurrentVersion(context.Background(), "/any/path")
	if err != nil {
		t.Fatalf("CurrentVersion: %v", err)
	}
	if v != "archive" {
		t.Errorf("got %q, want \"archive\"", v)
	}
}

// TestVcsArchive_Update_RequiresURL guards against a future regression
// to the historical no-op behaviour: Update must surface the missing
// URL instead of silently doing nothing.
func TestVcsArchive_Update_RequiresURL(t *testing.T) {
	a := &VcsArchive{Format: "tar"}
	err := a.Update(context.Background(), "", t.TempDir(), "")
	if err == nil {
		t.Fatal("expected error when Update is called without a URL")
	}
}

// TestVcsArchive_Update_RefetchesAndReplaces verifies that a second
// Update call against a changed upstream archive produces a fresh
// extracted tree (and removes files no longer present in the new
// archive). This is the regression test for #124.
func TestVcsArchive_Update_RefetchesAndReplaces(t *testing.T) {
	v1 := buildTarGz(t, map[string]string{
		"project/keep.txt":   "v1",
		"project/old.txt":    "removed in v2",
		"project/sub/data.x": "v1",
	})
	v2 := buildTarGz(t, map[string]string{
		"project/keep.txt":   "v2",
		"project/new.txt":    "added in v2",
		"project/sub/data.x": "v2",
	})

	var serveV2 bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		if serveV2 {
			w.Write(v2)
			return
		}
		w.Write(v1)
	}))
	t.Cleanup(srv.Close)

	dest := filepath.Join(t.TempDir(), "extract")
	a := &VcsArchive{Format: "tar"}

	// First Clone — extracts v1.
	if err := a.Clone(context.Background(), srv.URL, dest, ""); err != nil {
		t.Fatalf("Clone v1: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(dest, "keep.txt")); string(got) != "v1" {
		t.Errorf("keep.txt after v1 = %q, want %q", got, "v1")
	}
	if _, err := os.Stat(filepath.Join(dest, "old.txt")); err != nil {
		t.Errorf("old.txt missing after v1 install: %v", err)
	}

	// Update against changed upstream — extracts v2, drops old.txt, adds new.txt.
	serveV2 = true
	if err := a.Update(context.Background(), srv.URL, dest, ""); err != nil {
		t.Fatalf("Update v2: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(dest, "keep.txt")); string(got) != "v2" {
		t.Errorf("keep.txt after v2 = %q, want %q", got, "v2")
	}
	if got, _ := os.ReadFile(filepath.Join(dest, "new.txt")); string(got) != "added in v2" {
		t.Errorf("new.txt after v2 = %q, want %q", got, "added in v2")
	}
	if _, err := os.Stat(filepath.Join(dest, "old.txt")); !os.IsNotExist(err) {
		t.Errorf("old.txt should be removed after v2 install (err=%v)", err)
	}
}

func TestVcsArchive_Clone_Tar(t *testing.T) {
	// Standard archive convention: all entries share a top-level directory
	// (e.g. "project-v1/"). The extractor strips this first component.
	files := map[string]string{
		"project/README.md":   "# Test",
		"project/src/main.go": "package main",
	}
	data := buildTarGz(t, files)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	}))
	defer srv.Close()

	dest := t.TempDir()
	a := &VcsArchive{Format: "tar"}
	err := a.Clone(context.Background(), srv.URL, dest, "")
	if err != nil {
		t.Fatalf("Clone tar: %v", err)
	}

	// After stripping the first component "project/", files land at dest.
	for _, name := range []string{"README.md", filepath.Join("src", "main.go")} {
		p := filepath.Join(dest, name)
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected file %q after tar extract, got: %v", p, err)
		}
	}
}

func TestVcsArchive_Clone_Zip(t *testing.T) {
	// Same convention: top-level component stripped.
	files := map[string]string{
		"project/README.md":   "# Test zip",
		"project/lib/util.go": "package lib",
	}
	data := buildZip(t, files)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	}))
	defer srv.Close()

	dest := t.TempDir()
	a := &VcsArchive{Format: "zip"}
	err := a.Clone(context.Background(), srv.URL, dest, "")
	if err != nil {
		t.Fatalf("Clone zip: %v", err)
	}

	for _, name := range []string{"README.md", filepath.Join("lib", "util.go")} {
		p := filepath.Join(dest, name)
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected file %q after zip extract, got: %v", p, err)
		}
	}
}

func TestVcsArchive_Clone_HTTP404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	a := &VcsArchive{Format: "tar"}
	err := a.Clone(context.Background(), srv.URL, t.TempDir(), "")
	if err == nil {
		t.Fatal("expected error for HTTP 404")
	}
}

func TestVcsArchive_Clone_UnsupportedFormat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("data"))
	}))
	defer srv.Close()

	a := &VcsArchive{Format: "bz2"}
	err := a.Clone(context.Background(), srv.URL, t.TempDir(), "")
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
}

func TestVcsArchive_Clone_WithStripPrefix(t *testing.T) {
	// When stripPrefix is set, filepath.Rel(stripPrefix, name) is used instead
	// of stripping only the first path component.
	files := map[string]string{
		"prefix-v1.0/README.md": "# stripped prefix",
	}
	data := buildTarGz(t, files)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(data)
	}))
	defer srv.Close()

	dest := t.TempDir()
	a := &VcsArchive{Format: "tar"}
	err := a.Clone(context.Background(), srv.URL, dest, "prefix-v1.0")
	if err != nil {
		t.Fatalf("Clone with strip prefix: %v", err)
	}
	// File should be at dest/README.md, not dest/prefix-v1.0/README.md
	if _, err := os.Stat(filepath.Join(dest, "README.md")); err != nil {
		t.Errorf("expected README.md after prefix strip, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// archiveEntryPath (unit test - unexported but same package)
// ---------------------------------------------------------------------------

func TestArchiveEntryPath_PathTraversal(t *testing.T) {
	// A path starting with ".." is now rejected outright (was: silent skip).
	_, err := archiveEntryPath("../evil/path", "/dest", "")
	if err == nil {
		t.Error("expected error for traversal entry, got nil")
	}
}

func TestArchiveEntryPath_RootEntry(t *testing.T) {
	// Single component (no slash) with no stripPrefix -> skip (returns "").
	result, err := archiveEntryPath("single", "/dest", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "" {
		t.Errorf("expected empty for single component, got %q", result)
	}
}

func TestArchiveEntryPath_WithStripPrefix_DotEntry(t *testing.T) {
	// When stripPrefix is the same as name, rel=="." -> return "".
	result, err := archiveEntryPath("prefix", "/dest", "prefix")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "" {
		t.Errorf("expected empty for dot entry, got %q", result)
	}
}

func TestArchiveEntryPath_ValidTwoComponent(t *testing.T) {
	result, err := archiveEntryPath("prefix/file.txt", "/dest", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty result for two-component path")
	}
}

// ---------------------------------------------------------------------------
// Direct extractTar / extractZip tests (same package — unexported)
// ---------------------------------------------------------------------------

// buildTar creates a plain (non-gzip) tar stream with an optional dir entry.
func buildTar(t *testing.T, files map[string]string, dirs []string) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, d := range dirs {
		tw.WriteHeader(&tar.Header{ //nolint:errcheck
			Name:     d,
			Typeflag: tar.TypeDir,
			Mode:     0o755,
		})
	}
	for name, content := range files {
		hdr := &tar.Header{
			Name:     name,
			Mode:     0o644,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("tar WriteHeader: %v", err)
		}
		tw.Write([]byte(content)) //nolint:errcheck
	}
	tw.Close()
	return buf.Bytes()
}

func TestExtractTar_DirEntry(t *testing.T) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	// Directory entry (TypeDir).
	tw.WriteHeader(&tar.Header{ //nolint:errcheck
		Name:     "root/subdir/",
		Typeflag: tar.TypeDir,
		Mode:     0o755,
	})
	// File inside the subdir.
	content := []byte("hi")
	tw.WriteHeader(&tar.Header{Name: "root/subdir/file.txt", Mode: 0o644, Size: int64(len(content))}) //nolint:errcheck
	tw.Write(content)                                                                                 //nolint:errcheck
	tw.Close()
	gw.Close()

	dest := t.TempDir()
	if err := extractTar(&buf, dest, ""); err != nil {
		t.Fatalf("extractTar with dir entry: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "subdir", "file.txt")); err != nil {
		t.Errorf("expected subdir/file.txt: %v", err)
	}
}

func TestExtractTar_PlainTarFallback(t *testing.T) {
	// Plain (non-gzip) tar: gzip.NewReader fails, code falls back to tar.NewReader(r).
	// The partial read may corrupt parsing, but the else branch is exercised.
	data := buildTar(t, map[string]string{"pfx/test.txt": "abc"}, nil)
	dest := t.TempDir()
	// We don't assert success because the plain-tar fallback over a non-seekable
	// reader may produce an error — we just ensure the branch is reachable.
	_ = extractTar(bytes.NewReader(data), dest, "")
}

func TestExtractZip_DirEntry(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	// Directory entry: name ends with "/".
	zw.Create("root/mydir/") //nolint:errcheck
	// File inside the directory.
	w, err := zw.Create("root/mydir/hello.txt")
	if err != nil {
		t.Fatalf("zip Create: %v", err)
	}
	w.Write([]byte("hello")) //nolint:errcheck
	zw.Close()

	dest := t.TempDir()
	if err := extractZip(&buf, dest, ""); err != nil {
		t.Fatalf("extractZip with dir entry: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "mydir", "hello.txt")); err != nil {
		t.Errorf("expected mydir/hello.txt: %v", err)
	}
}

func TestFetchURL_InvalidURL(t *testing.T) {
	// A URL with a null byte is invalid and causes http.NewRequestWithContext to fail.
	_, err := fetchURL(context.Background(), "http://\x00invalid")
	if err == nil {
		t.Fatal("expected error for URL with invalid characters")
	}
}

func TestVcsArchive_HasChanges_AlwaysFalse(t *testing.T) {
	a := &VcsArchive{Format: "tar"}
	dirty, err := a.HasChanges(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("HasChanges on archive: %v", err)
	}
	if dirty {
		t.Error("expected HasChanges=false for archive backend")
	}
}

// ---------------------------------------------------------------------------
// fetchURL — network and HTTP error paths
// ---------------------------------------------------------------------------

func TestFetchURL_NetworkError(t *testing.T) {
	// Close the server before making the request to get a connection refused error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()
	_, err := fetchURL(context.Background(), url)
	if err == nil {
		t.Fatal("expected error for connection refused")
	}
}

func TestFetchURL_HTTP500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()
	_, err := fetchURL(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected error for HTTP 500")
	}
}

// ---------------------------------------------------------------------------
// extractZip — corrupted data
// ---------------------------------------------------------------------------

func TestExtractZip_CorruptedData(t *testing.T) {
	// Pass random bytes that are not a valid zip archive.
	r := bytes.NewReader([]byte("this is not a zip file at all"))
	err := extractZip(r, t.TempDir(), "")
	if err == nil {
		t.Fatal("expected error for corrupted zip data")
	}
}

// ---------------------------------------------------------------------------
// Clone — destination MkdirAll failure
// ---------------------------------------------------------------------------

func TestVcsArchive_Clone_MkdirAllFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based permission enforcement is not supported on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("root bypasses permissions — skipping")
	}
	// Serve a valid tar so the HTTP part succeeds; the mkdir will fail first.
	data := buildTarGz(t, map[string]string{"p/file.txt": "hi"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(data)
	}))
	defer srv.Close()

	// Make the parent of the destination unwritable.
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(parent, 0o755) })

	a := &VcsArchive{Format: "tar"}
	err := a.Clone(context.Background(), srv.URL, filepath.Join(parent, "dest"), "")
	if err == nil {
		t.Fatal("expected error when destination parent is not writable")
	}
}

// ---------------------------------------------------------------------------
// writeFile — MkdirAll failure
// ---------------------------------------------------------------------------

func TestWriteFile_MkdirAllFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based permission enforcement is not supported on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("root bypasses permissions — skipping")
	}
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(parent, 0o755) })

	err := writeFile(bytes.NewReader([]byte("data")), filepath.Join(parent, "sub", "file.txt"), 0o644)
	if err == nil {
		t.Fatal("expected error when parent directory is not writable")
	}
}

// ---------------------------------------------------------------------------
// Adversarial archive entries (#112)
// ---------------------------------------------------------------------------

// buildTarRaw lets a test hand-craft tar headers (any Typeflag, any Name).
func buildTarRaw(t *testing.T, headers []*tar.Header, contents map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, hdr := range headers {
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		if c, ok := contents[hdr.Name]; ok {
			tw.Write([]byte(c)) //nolint:errcheck
		}
	}
	tw.Close()
	return buf.Bytes()
}

func TestExtractTar_RejectsAbsolutePathEntry(t *testing.T) {
	data := buildTarRaw(t, []*tar.Header{
		{Name: "/etc/pwned", Typeflag: tar.TypeReg, Size: 4, Mode: 0o644},
	}, map[string]string{"/etc/pwned": "evil"})

	dest := t.TempDir()
	err := extractTar(bytes.NewReader(data), dest, "")
	if err == nil {
		t.Fatal("expected error for absolute-path entry, got nil")
	}
	if _, statErr := os.Stat("/etc/pwned"); statErr == nil {
		t.Errorf("absolute-path entry was written to /etc/pwned")
	}
}

func TestExtractTar_RejectsParentTraversalEntry(t *testing.T) {
	data := buildTarRaw(t, []*tar.Header{
		{Name: "pfx/", Typeflag: tar.TypeDir, Mode: 0o755},
		{Name: "pfx/../../../tmp/gaal-pwned", Typeflag: tar.TypeReg, Size: 4, Mode: 0o644},
	}, map[string]string{"pfx/../../../tmp/gaal-pwned": "evil"})

	dest := t.TempDir()
	err := extractTar(bytes.NewReader(data), dest, "")
	if err == nil {
		t.Fatal("expected error for traversal entry, got nil")
	}
	if _, statErr := os.Stat("/tmp/gaal-pwned"); statErr == nil {
		os.Remove("/tmp/gaal-pwned")
		t.Errorf("traversal entry escaped to /tmp/gaal-pwned")
	}
}

func TestExtractTar_SkipsSymlinkEntry(t *testing.T) {
	// Symlinks are silently skipped (with a warn log) so a malicious archive
	// cannot redirect a subsequent regular-file write to a sensitive path.
	data := buildTarRaw(t, []*tar.Header{
		{Name: "pfx/", Typeflag: tar.TypeDir, Mode: 0o755},
		{Name: "pfx/link", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd", Mode: 0o777},
	}, nil)

	dest := t.TempDir()
	if err := extractTar(bytes.NewReader(data), dest, ""); err != nil {
		t.Fatalf("extractTar: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dest, "link")); err == nil {
		t.Errorf("symlink entry must not have been created in dest")
	}
}

func TestExtractTar_PerEntrySizeCap(t *testing.T) {
	// Build a tar where one entry's declared body exceeds MaxFileBytes.
	huge := MaxFileBytes + 1
	hdr := &tar.Header{Name: "pfx/", Typeflag: tar.TypeDir, Mode: 0o755}
	hdr2 := &tar.Header{Name: "pfx/big", Typeflag: tar.TypeReg, Mode: 0o644, Size: huge}

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if err := tw.WriteHeader(hdr2); err != nil {
		t.Fatal(err)
	}
	// Stream zero bytes up to the declared size — actual disk impact is
	// bounded by writeFileLimited copying maxBytes+1 then bailing.
	zeros := make([]byte, 1<<20) // 1 MiB
	written := int64(0)
	for written < huge {
		n := int64(len(zeros))
		if rem := huge - written; rem < n {
			n = rem
		}
		if _, err := tw.Write(zeros[:n]); err != nil {
			t.Fatal(err)
		}
		written += n
	}
	tw.Close()

	dest := t.TempDir()
	err := extractTar(bytes.NewReader(buf.Bytes()), dest, "")
	if err == nil {
		t.Fatal("expected per-entry size cap error, got nil")
	}
}

func TestExtractTar_GzipDetection_NonDestructive(t *testing.T) {
	// Plain (non-gzip) tar must still extract correctly — the new gzip-magic
	// peek must not consume the first two bytes of the tar stream.
	data := buildTar(t, map[string]string{
		"pfx/hello.txt": "world",
	}, []string{"pfx/"})

	dest := t.TempDir()
	if err := extractTar(bytes.NewReader(data), dest, ""); err != nil {
		t.Fatalf("extractTar: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(dest, "hello.txt"))
	if err != nil {
		t.Fatalf("read extracted: %v", err)
	}
	if string(body) != "world" {
		t.Errorf("got %q, want %q", body, "world")
	}
}

func TestExtractTar_EntryCountCap(t *testing.T) {
	if testing.Short() {
		t.Skip("skipped in -short")
	}
	// Build a tar with > MaxEntryCount tiny entries.
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for i := 0; i <= MaxEntryCount; i++ {
		hdr := &tar.Header{
			Name:     fmt.Sprintf("pfx/file-%d.txt", i),
			Typeflag: tar.TypeReg,
			Mode:     0o644,
			Size:     1,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		tw.Write([]byte("x")) //nolint:errcheck
	}
	tw.Close()

	dest := t.TempDir()
	err := extractTar(bytes.NewReader(buf.Bytes()), dest, "")
	if err == nil {
		t.Fatal("expected entry-count cap error, got nil")
	}
}
