package content

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gaal/internal/config"
	"gaal/internal/core/agent"
	"gaal/internal/core/vcs"
	"gaal/internal/discover"
	"gaal/internal/urlx"
)

type PathStatus struct {
	Source  string
	Target  string
	Present bool
	Dirty   bool
	Error   string
}

type Status struct {
	Source string
	Agent  string
	Scope  string
	Root   string
	Paths  []PathStatus
	Err    error
}

type Manager struct {
	entries  []config.ConfigContent
	cacheDir string
	home     string
	workDir  string
	stateDir string
	force    bool
}

func NewManager(entries []config.ConfigContent, cacheDir, home, workDir, stateDir string, force bool) *Manager {
	slog.Debug("creating content manager", "entries", len(entries), "cacheDir", cacheDir)
	return &Manager{entries: entries, cacheDir: cacheDir, home: home, workDir: workDir, stateDir: stateDir, force: force}
}

func (m *Manager) Sync(ctx context.Context) error {
	slog.DebugContext(ctx, "syncing content entries", "count", len(m.entries))
	var errs []error
	for _, entry := range m.entries {
		if err := m.syncOne(ctx, entry); err != nil {
			errs = append(errs, fmt.Errorf("content %q: %w", entry.Source, err))
		}
	}
	return errors.Join(errs...)
}

func (m *Manager) Status(ctx context.Context) []Status {
	slog.DebugContext(ctx, "collecting content status", "count", len(m.entries))
	var statuses []Status
	for _, entry := range m.entries {
		statuses = append(statuses, m.statusOne(ctx, entry)...)
	}
	return statuses
}

func (m *Manager) syncOne(ctx context.Context, entry config.ConfigContent) error {
	slog.DebugContext(ctx, "syncing content source", "source", entry.Source)
	sourceRoot, err := m.resolveSource(ctx, entry.Source)
	if err != nil {
		return fmt.Errorf("resolving source: %w", err)
	}

	var errs []error
	for _, target := range expandTargets(entry) {
		for _, agentName := range m.resolveAgents(target) {
			root, err := m.targetRoot(agentName, target)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			for _, mapping := range sortedMappings(target.Paths) {
				src := filepath.Join(sourceRoot, filepath.FromSlash(strings.TrimSuffix(mapping.Source, "/")))
				dst := filepath.Join(root, filepath.FromSlash(strings.TrimSuffix(mapping.Dest, "/")))
				if err := copyPath(src, dst); err != nil {
					errs = append(errs, fmt.Errorf("%s -> %s: %w", mapping.Source, dst, err))
					continue
				}
				m.writeContentSnapshot(dst)
			}
		}
	}
	return errors.Join(errs...)
}

func (m *Manager) statusOne(ctx context.Context, entry config.ConfigContent) []Status {
	slog.DebugContext(ctx, "collecting content source status", "source", entry.Source)
	sourceRoot, err := cachedSourcePath(m.cacheDir, entry.Source)
	var out []Status
	for _, target := range expandTargets(entry) {
		for _, agentName := range m.resolveAgents(target) {
			st := Status{Source: entry.Source, Agent: agentName, Scope: targetScope(target), Root: targetRootKind(target)}
			root, rootErr := m.targetRoot(agentName, target)
			if err != nil || sourceRoot == "" {
				st.Err = fmt.Errorf("source not cached yet")
				out = append(out, st)
				continue
			}
			if rootErr != nil {
				st.Err = rootErr
				out = append(out, st)
				continue
			}
			for _, mapping := range sortedMappings(target.Paths) {
				src := filepath.Join(sourceRoot, filepath.FromSlash(strings.TrimSuffix(mapping.Source, "/")))
				dst := filepath.Join(root, filepath.FromSlash(strings.TrimSuffix(mapping.Dest, "/")))
				ps := PathStatus{Source: mapping.Source, Target: dst}
				if _, statErr := os.Stat(dst); statErr != nil {
					if os.IsNotExist(statErr) {
						st.Paths = append(st.Paths, ps)
						continue
					}
					ps.Error = statErr.Error()
					st.Paths = append(st.Paths, ps)
					continue
				}
				ps.Present = true
				ps.Dirty = pathModified(src, dst)
				st.Paths = append(st.Paths, ps)
			}
			out = append(out, st)
		}
	}
	return out
}

func expandTargets(entry config.ConfigContent) []config.ConfigContentTarget {
	slog.Debug("expanding content targets", "source", entry.Source, "targets", len(entry.Targets))
	if len(entry.Targets) > 0 {
		return entry.Targets
	}
	scope := "project"
	if entry.Global {
		scope = "global"
	}
	return []config.ConfigContentTarget{{
		Agents: entry.Agents,
		Scope:  scope,
		Root:   entry.Root,
		Paths:  entry.Paths,
	}}
}

func targetScope(target config.ConfigContentTarget) string {
	slog.Debug("resolving content target scope", "scope", target.Scope)
	if target.Scope == "" {
		return "project"
	}
	return target.Scope
}

func targetRootKind(target config.ConfigContentTarget) string {
	slog.Debug("resolving content target root kind", "root", target.Root)
	if target.Root == "" {
		return "agent"
	}
	return target.Root
}

func (m *Manager) resolveAgents(target config.ConfigContentTarget) []string {
	slog.Debug("resolving content target agents", "agents", target.Agents, "force", m.force)
	if len(target.Agents) == 1 && target.Agents[0] == "*" {
		if m.force {
			return agent.Names()
		}
		var found []string
		for _, name := range agent.Names() {
			if _, err := m.targetRoot(name, target); err == nil {
				found = append(found, name)
			}
		}
		return found
	}
	return target.Agents
}

func (m *Manager) targetRoot(agentName string, target config.ConfigContentTarget) (string, error) {
	slog.Debug("resolving content target root", "agent", agentName, "scope", target.Scope, "root", target.Root)
	if targetRootKind(target) == "workspace" {
		return m.workDir, nil
	}
	global := targetScope(target) == "global"
	dir, ok := agent.SkillDir(agentName, global, m.home)
	if !ok {
		return "", fmt.Errorf("unknown agent %q", agentName)
	}
	if !global && !filepath.IsAbs(dir) {
		dir = filepath.Join(m.workDir, dir)
	}
	return filepath.Dir(dir), nil
}

type pathMapping struct {
	Source string
	Dest   string
}

func sortedMappings(paths map[string]string) []pathMapping {
	slog.Debug("sorting content path mappings", "count", len(paths))
	out := make([]pathMapping, 0, len(paths))
	for src, dst := range paths {
		out = append(out, pathMapping{Source: src, Dest: dst})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Source == out[j].Source {
			return out[i].Dest < out[j].Dest
		}
		return out[i].Source < out[j].Source
	})
	return out
}

func (m *Manager) resolveSource(ctx context.Context, source string) (string, error) {
	slog.DebugContext(ctx, "resolving content source", "source", source)
	if isLocalPath(source) {
		return expandHome(source, m.home), nil
	}

	cloneURL := toCloneURL(source)
	vcsType := vcs.DetectType(cloneURL)
	localPath := filepath.Join(m.cacheDir, urlToCacheKey(cloneURL))
	backend, err := vcs.NewShallow(vcsType)
	if err != nil {
		return "", err
	}
	if !backend.IsCloned(localPath) {
		if err := backend.Clone(ctx, cloneURL, localPath, ""); err != nil {
			return "", fmt.Errorf("cloning %s: %w", urlx.Redact(cloneURL), err)
		}
	} else if err := backend.Update(ctx, "", localPath, ""); err != nil {
		slog.Warn("could not update content source", "path", localPath, "err", err)
	}
	return localPath, nil
}

func cachedSourcePath(cacheDir, source string) (string, error) {
	slog.Debug("resolving cached content source path", "source", source)
	if isLocalPath(source) {
		return source, nil
	}
	path := filepath.Join(cacheDir, urlToCacheKey(toCloneURL(source)))
	if _, err := os.Stat(path); err != nil {
		return "", nil
	}
	return path, nil
}

func isLocalPath(source string) bool {
	slog.Debug("checking content source locality", "source", source)
	return filepath.IsAbs(source) ||
		strings.HasPrefix(source, ".") ||
		strings.HasPrefix(source, "~/") ||
		strings.HasPrefix(source, `~\`) ||
		strings.Contains(source, string(os.PathSeparator))
}

func expandHome(path, home string) string {
	slog.Debug("expanding content source home", "path", path)
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		return filepath.Join(home, path[2:])
	}
	return path
}

func toCloneURL(source string) string {
	slog.Debug("resolving content clone url", "source", source)
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") || strings.HasPrefix(source, "git@") || strings.Contains(source, "://") {
		return source
	}
	return "https://github.com/" + source
}

func urlToCacheKey(rawURL string) string {
	slog.Debug("building content cache key", "url", urlx.Redact(rawURL))
	sum := sha1.Sum([]byte(rawURL))
	return hex.EncodeToString(sum[:])
}

func copyPath(src, dst string) error {
	slog.Debug("copying content path", "src", src, "dst", dst)
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return fmt.Errorf("symlink sources are not supported")
	}
	if info.IsDir() {
		return copyDirReplace(src, dst)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("source is not a regular file or directory")
	}
	return copyFileAtomic(src, dst, info.Mode().Perm())
}

func copyDirReplace(src, dst string) error {
	slog.Debug("copying content directory", "src", src, "dst", dst)
	parent := filepath.Dir(dst)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(parent, ".gaal-content-tmp-*")
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(staging)
		}
	}()
	if err := copyDirContents(src, staging); err != nil {
		return err
	}
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	if err := os.Rename(staging, dst); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func copyDirContents(src, dst string) error {
	slog.Debug("copying content directory contents", "src", src, "dst", dst)
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && path != src {
			if _, skip := vcsMetaDirs[d.Name()]; skip {
				return filepath.SkipDir
			}
		}
		rel, _ := filepath.Rel(src, path)
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsRegular() {
			slog.Warn("content: skipping non-regular source entry", "path", path)
			return nil
		}
		return copyFileAtomic(path, target, info.Mode().Perm())
	})
}

var vcsMetaDirs = map[string]struct{}{
	".git": {},
	".hg":  {},
	".svn": {},
	".bzr": {},
}

func copyFileAtomic(src, dst string, mode os.FileMode) error {
	slog.Debug("copying content file", "src", src, "dst", dst)
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".gaal-content-file-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(sanitizedMode(mode)); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, dst); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func sanitizedMode(mode os.FileMode) os.FileMode {
	slog.Debug("sanitizing content file mode", "mode", mode)
	if mode&0o111 != 0 {
		return 0o755
	}
	return 0o644
}

func pathModified(src, dst string) bool {
	slog.Debug("checking content path drift", "src", src, "dst", dst)
	srcInfo, err := os.Stat(src)
	if err != nil {
		return true
	}
	dstInfo, err := os.Stat(dst)
	if err != nil {
		return true
	}
	if srcInfo.IsDir() != dstInfo.IsDir() {
		return true
	}
	if srcInfo.IsDir() {
		return dirModified(src, dst)
	}
	return fileModified(src, dst)
}

func dirModified(src, dst string) bool {
	slog.Debug("checking content directory drift", "src", src, "dst", dst)
	modified := false
	seen := map[string]struct{}{}
	_ = filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil || modified {
			return err
		}
		if d.IsDir() && path != src {
			if _, skip := vcsMetaDirs[d.Name()]; skip {
				return filepath.SkipDir
			}
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil
		}
		rel, _ := filepath.Rel(src, path)
		seen[rel] = struct{}{}
		if fileModified(path, filepath.Join(dst, rel)) {
			modified = true
		}
		return nil
	})
	if modified {
		return true
	}
	_ = filepath.WalkDir(dst, func(path string, d fs.DirEntry, err error) error {
		if err != nil || modified || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dst, path)
		if _, ok := seen[rel]; !ok {
			modified = true
		}
		return nil
	})
	return modified
}

func fileModified(src, dst string) bool {
	slog.Debug("checking content file drift", "src", src, "dst", dst)
	a, err := os.ReadFile(src)
	if err != nil {
		return true
	}
	b, err := os.ReadFile(dst)
	if err != nil {
		return true
	}
	return !bytes.Equal(a, b)
}

func (m *Manager) writeContentSnapshot(dest string) {
	slog.Debug("writing content snapshot", "dest", dest)
	if m.stateDir == "" {
		return
	}
	snap, err := snapshotPath(dest)
	if err != nil {
		slog.Warn("content snapshot failed", "dest", dest, "err", err)
		return
	}
	key := "content-" + discover.WorkdirKey(dest)
	if err := discover.Save(discover.SnapshotPath(m.stateDir, key), snap); err != nil {
		slog.Warn("content snapshot save failed", "dest", dest, "err", err)
	}
}

func snapshotPath(path string) (discover.Snapshot, error) {
	slog.Debug("snapshotting content path", "path", path)
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return discover.SnapshotDir(path)
	}
	rec, err := discover.Record(path)
	if err != nil {
		return nil, err
	}
	return discover.Snapshot{filepath.Base(path): rec}, nil
}
