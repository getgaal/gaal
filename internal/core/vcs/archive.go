package vcs

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"gaal/internal/httpx"
	"gaal/internal/urlx"
)

// VcsArchive implements VCS for tar and zip archives fetched over HTTP(S).
// The Version field in RepoConfig is treated as an optional sub-directory
// prefix to strip when extracting (mirrors vcstool behaviour).
type VcsArchive struct {
	Format string // "tar" or "zip"
}

// Resource limits applied during extraction. These defend against zip/tar
// bombs and malicious archives whose stated entry list dwarfs the user's
// disk. The values are deliberately generous enough for realistic skill /
// repo bundles but tight enough to fail fast on adversarial inputs.
const (
	// MaxArchiveBytes caps the raw network body we will buffer or stream.
	MaxArchiveBytes int64 = 1 << 30 // 1 GiB

	// MaxFileBytes caps the decompressed size of any single entry. This is
	// the primary defense against gzip bombs (a 1 KiB tar.gz that expands
	// to many GiB of zeros).
	MaxFileBytes int64 = 256 << 20 // 256 MiB

	// MaxEntryCount caps the number of entries we will process. Defends
	// against archives that exhaust inodes / process limits with millions
	// of empty files.
	MaxEntryCount = 50_000
)

// errEntryTooLarge is returned when an archive entry exceeds MaxFileBytes.
var errEntryTooLarge = errors.New("archive entry exceeds per-file size cap")

func (a *VcsArchive) Clone(ctx context.Context, url, path, version string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("creating target directory: %w", err)
	}

	data, err := fetchURL(ctx, url)
	if err != nil {
		return err
	}
	defer data.Close()

	// Cap the raw body up front so that zip-buffer-to-tempfile and tar
	// streaming both inherit the limit without needing per-format wiring.
	limited := io.LimitReader(data, MaxArchiveBytes+1)

	switch a.Format {
	case "tar":
		return extractTar(limited, path, version)
	case "zip":
		return extractZip(limited, path, version)
	default:
		return fmt.Errorf("unsupported archive format: %q", a.Format)
	}
}

// Update re-downloads and extracts the archive (no incremental update possible).
func (a *VcsArchive) Update(ctx context.Context, path, version string) error {
	// Archives have no "version" concept — just re-extract.
	// We need the URL: it is not stored in the struct, so Update is a no-op
	// for archives. The manager should call Clone instead.
	return nil
}

func (a *VcsArchive) IsCloned(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

func (a *VcsArchive) CurrentVersion(_ context.Context, _ string) (string, error) {
	return "archive", nil
}

// HasChanges always returns false for archives: extracted files carry no VCS
// metadata, so change detection is not possible.
func (a *VcsArchive) HasChanges(_ context.Context, _ string) (bool, error) {
	return false, nil
}

// fetchURL performs a GET request and returns the response body.
// Body-size enforcement is the caller's responsibility — extractors wrap
// the returned ReadCloser in io.LimitReader(MaxArchiveBytes+1).
func fetchURL(ctx context.Context, rawURL string) (io.ReadCloser, error) {
	if err := urlx.ValidateRemoteFetchURL(rawURL); err != nil {
		return nil, err
	}
	req, err := httpx.NewRequest(ctx, http.MethodGet, rawURL)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}

	resp, err := httpx.Client().Do(req)
	safeURL := urlx.Redact(rawURL)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", safeURL, err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("fetching %s: HTTP %d", safeURL, resp.StatusCode)
	}

	return resp.Body, nil
}

func extractTar(r io.Reader, dest, stripPrefix string) error {
	// Detect gzip non-destructively: peek the first two bytes for the magic
	// (1F 8B) instead of letting gzip.NewReader consume them on a non-gzip
	// stream (which would leave tar.NewReader with a truncated source).
	br := bufio.NewReader(r)
	magic, _ := br.Peek(2)
	var src io.Reader = br
	if len(magic) == 2 && magic[0] == 0x1f && magic[1] == 0x8b {
		gr, err := gzip.NewReader(br)
		if err != nil {
			return fmt.Errorf("opening gzip: %w", err)
		}
		defer gr.Close()
		src = gr
	}

	tr := tar.NewReader(src)
	entries := 0
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar: %w", err)
		}
		entries++
		if entries > MaxEntryCount {
			return fmt.Errorf("archive exceeds entry-count cap (%d)", MaxEntryCount)
		}

		target, err := archiveEntryPath(hdr.Name, dest, stripPrefix)
		if err != nil {
			return err
		}
		if target == "" {
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := writeFileLimited(tr, target, hdr.FileInfo().Mode(), MaxFileBytes); err != nil {
				return err
			}
		default:
			// Reject symlinks, hardlinks, devices, FIFOs, sockets and the
			// PAX-extended types. Allowing TypeSymlink in particular would
			// re-enable the classic "symlink-then-overwrite-symlink-target"
			// race; the safer and simpler thing is to drop them with a warn.
			slog.Warn("archive: skipping entry with disallowed type",
				"name", hdr.Name, "typeflag", hdr.Typeflag)
		}
	}
	return nil
}

func extractZip(r io.Reader, dest, stripPrefix string) error {
	// zip.NewReader needs io.ReaderAt; buffer to a temp file. The body was
	// already wrapped in LimitReader(MaxArchiveBytes+1) by the caller; if we
	// hit that limit, abort.
	tmp, err := os.CreateTemp("", "gaal-zip-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	size, err := io.Copy(tmp, r)
	if err != nil {
		return fmt.Errorf("buffering zip: %w", err)
	}
	if size > MaxArchiveBytes {
		return fmt.Errorf("archive exceeds size cap (%d bytes)", MaxArchiveBytes)
	}

	zr, err := zip.NewReader(tmp, size)
	if err != nil {
		return fmt.Errorf("opening zip: %w", err)
	}

	if len(zr.File) > MaxEntryCount {
		return fmt.Errorf("archive exceeds entry-count cap (%d)", MaxEntryCount)
	}

	for _, f := range zr.File {
		target, err := archiveEntryPath(f.Name, dest, stripPrefix)
		if err != nil {
			return err
		}
		if target == "" {
			continue
		}

		// Skip non-regular entries explicitly. Zip's symlinks (encoded via
		// extra-fields) and devices have no business in a skill bundle.
		if !f.FileInfo().Mode().IsRegular() && !f.FileInfo().IsDir() {
			slog.Warn("archive: skipping zip entry with disallowed mode",
				"name", f.Name, "mode", f.Mode())
			continue
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}
		writeErr := writeFileLimited(rc, target, f.Mode(), MaxFileBytes)
		rc.Close()
		if writeErr != nil {
			return writeErr
		}
	}
	return nil
}

// archiveEntryPath converts an archive entry name to its destination path,
// optionally stripping stripPrefix. Returns ("", nil) for benign entries that
// should be skipped (root, "."), and ("", err) for malicious entries that
// attempt to escape dest.
//
// Containment is enforced via filepath.Rel + HasPrefix, not just a leading
// "..": that would miss absolute paths and Windows-separator tricks where
// filepath.Clean only normalises with the OS-native separator.
func archiveEntryPath(name, dest, stripPrefix string) (string, error) {
	if name == "" {
		return "", nil
	}
	// Reject absolute paths in archive entries (POSIX or Windows-rooted).
	if filepath.IsAbs(name) || strings.HasPrefix(name, "/") || strings.HasPrefix(name, `\`) {
		return "", fmt.Errorf("archive entry %q has an absolute path", name)
	}

	cleaned := filepath.Clean(name)
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) ||
		strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("archive entry %q escapes via ..", name)
	}

	if stripPrefix != "" {
		rel, err := filepath.Rel(stripPrefix, cleaned)
		if err != nil || strings.HasPrefix(rel, "..") {
			return "", nil
		}
		cleaned = rel
	} else {
		// Strip the first path component (common archive convention).
		parts := strings.SplitN(cleaned, string(filepath.Separator), 2)
		if len(parts) < 2 {
			return "", nil
		}
		cleaned = parts[1]
	}

	if cleaned == "" || cleaned == "." {
		return "", nil
	}

	target := filepath.Join(dest, cleaned)

	// Final containment check: target must stay under dest. Defends against
	// the case where the post-strip name still contains traversal that
	// filepath.Clean(dest+...) would resolve outside dest.
	absDest, derr := filepath.Abs(dest)
	absTgt, terr := filepath.Abs(target)
	if derr != nil || terr != nil {
		return "", fmt.Errorf("computing absolute path: dest=%v target=%v", derr, terr)
	}
	rel, rerr := filepath.Rel(absDest, absTgt)
	if rerr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("archive entry %q escapes destination", name)
	}

	return target, nil
}

// writeFile is the unbounded write used by tests that exercise filesystem
// permission paths only. Production extractors use writeFileLimited.
func writeFile(r io.Reader, path string, mode os.FileMode) error {
	return writeFileLimited(r, path, mode, MaxFileBytes)
}

// writeFileLimited writes at most maxBytes from r to path, returning
// errEntryTooLarge if the source exceeds the cap. Used by extractTar and
// extractZip so a single oversize entry cannot fill the disk.
func writeFileLimited(r io.Reader, path string, mode os.FileMode, maxBytes int64) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer f.Close()

	// Copy maxBytes+1 to detect overflow without reading more than needed.
	n, copyErr := io.CopyN(f, r, maxBytes+1)
	if copyErr != nil && copyErr != io.EOF {
		return copyErr
	}
	if n > maxBytes {
		return fmt.Errorf("%w: %s (>= %d bytes)", errEntryTooLarge, path, maxBytes)
	}
	return nil
}
