package skill

import (
	"bufio"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// Meta holds the discovered metadata of a single skill (a directory that
// contains a SKILL.md file).
type Meta struct {
	// Name is the skill identifier, derived from frontmatter or directory name.
	Name string
	// Desc is the description from the SKILL.md frontmatter (may be empty).
	Desc string
	// Path is the absolute path to the skill directory.
	Path string
}

// ParseSkillMeta reads the YAML frontmatter block (--- ... ---) from the given
// SKILL.md file and returns the name and description fields.
// If name is not present in the frontmatter the directory name is used instead.
func ParseSkillMeta(filePath string) (name, desc string, err error) {
	slog.Debug("parsing skill meta", "file", filePath)
	f, err := os.Open(filePath)
	if err != nil {
		return "", "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	inFrontmatter := false
	for scanner.Scan() {
		line := scanner.Text()
		if line == "---" {
			if !inFrontmatter {
				inFrontmatter = true
				continue
			}
			break
		}
		if !inFrontmatter {
			continue
		}
		if k, v, ok := strings.Cut(line, ":"); ok {
			switch strings.TrimSpace(k) {
			case "name":
				name = strings.TrimSpace(v)
			case "description":
				desc = strings.TrimSpace(v)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return "", "", err
	}

	if name == "" {
		name = filepath.Base(filepath.Dir(filePath))
	}
	return name, desc, nil
}

// ScanDir scans dir at exactly 1 level deep and returns metadata for every
// sub-directory that contains a SKILL.md file. It also checks dir/SKILL.md
// itself (i.e. dir is a skill root).
func ScanDir(dir string) ([]Meta, error) {
	slog.Debug("scanning skill dir", "dir", dir)

	var metas []Meta
	seen := map[string]struct{}{}

	add := func(skillDir, mdPath string) {
		if _, ok := seen[skillDir]; ok {
			return
		}
		seen[skillDir] = struct{}{}
		name, desc, err := ParseSkillMeta(mdPath)
		if err != nil {
			slog.Warn("skipping invalid SKILL.md", "path", mdPath, "err", err)
			return
		}
		metas = append(metas, Meta{Name: name, Desc: desc, Path: skillDir})
	}

	// Check dir/SKILL.md first (dir itself is the skill root).
	rootMD := filepath.Join(dir, "SKILL.md")
	if _, err := os.Stat(rootMD); err == nil {
		add(dir, rootMD)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		// Dir may not exist on this machine — not an error for audit purposes.
		return nil, nil //nolint:nilerr
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillDir := filepath.Join(dir, e.Name())
		mdPath := filepath.Join(skillDir, "SKILL.md")
		if _, err := os.Stat(mdPath); err != nil {
			continue
		}
		add(skillDir, mdPath)
	}

	return metas, nil
}

// WalkForSkillDirs walks root recursively and returns the absolute paths of
// every directory named "skills" it finds. When such a directory is found it
// is not descended into further (its own children are left to the caller to
// scan with ScanDir).
func WalkForSkillDirs(root string) ([]string, error) {
	slog.Debug("walking for skill dirs", "root", root)

	var dirs []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			// Unreadable entry — skip it rather than aborting the whole walk.
			slog.Debug("walk error, skipping", "path", path, "err", err)
			return filepath.SkipDir
		}
		if !d.IsDir() {
			return nil
		}
		if filepath.Base(path) == "skills" {
			dirs = append(dirs, path)
			return filepath.SkipDir // do not descend inside skills/
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return dirs, nil
}
