package vcs

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// VcsArchive implements VCS for tar and zip archives fetched over HTTP(S).
// The Version field in RepoConfig is treated as an optional sub-directory
// prefix to strip when extracting (mirrors vcstool behaviour).
type VcsArchive struct {
	Format string // "tar" or "zip"
}

func (a *VcsArchive) Clone(ctx context.Context, url, path, version string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("creating target directory: %w", err)
	}

	data, err := fetchURL(ctx, url)
	if err != nil {
		return err
	}
	defer data.Close()

	switch a.Format {
	case "tar":
		return extractTar(data, path, version)
	case "zip":
		return extractZip(data, path, version)
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
func fetchURL(ctx context.Context, rawURL string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", rawURL, err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("fetching %s: HTTP %d", rawURL, resp.StatusCode)
	}

	return resp.Body, nil
}

func extractTar(r io.Reader, dest, stripPrefix string) error {
	gr, err := gzip.NewReader(r)
	if err != nil {
		// Try plain tar (no gzip).
		gr = nil
	}

	var tr *tar.Reader
	if gr != nil {
		tr = tar.NewReader(gr)
	} else {
		tr = tar.NewReader(r)
	}

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar: %w", err)
		}

		target := archiveEntryPath(hdr.Name, dest, stripPrefix)
		if target == "" {
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := writeFile(tr, target, hdr.FileInfo().Mode()); err != nil {
				return err
			}
		}
	}
	return nil
}

func extractZip(r io.Reader, dest, stripPrefix string) error {
	// zip.NewReader needs io.ReaderAt; buffer to a temp file.
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

	zr, err := zip.NewReader(tmp, size)
	if err != nil {
		return fmt.Errorf("opening zip: %w", err)
	}

	for _, f := range zr.File {
		target := archiveEntryPath(f.Name, dest, stripPrefix)
		if target == "" {
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
		writeErr := writeFile(rc, target, f.Mode())
		rc.Close()
		if writeErr != nil {
			return writeErr
		}
	}
	return nil
}

// archiveEntryPath converts an archive entry name to its destination path,
// optionally stripping stripPrefix.
func archiveEntryPath(name, dest, stripPrefix string) string {
	// Prevent path traversal.
	name = filepath.Clean(name)
	if strings.HasPrefix(name, "..") {
		return ""
	}

	if stripPrefix != "" {
		rel, err := filepath.Rel(stripPrefix, name)
		if err != nil || strings.HasPrefix(rel, "..") {
			return ""
		}
		name = rel
	} else {
		// Strip the first path component (common archive convention).
		parts := strings.SplitN(name, string(filepath.Separator), 2)
		if len(parts) < 2 {
			return ""
		}
		name = parts[1]
	}

	if name == "" || name == "." {
		return ""
	}

	return filepath.Join(dest, name)
}

func writeFile(r io.Reader, path string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, r)
	return err
}
