package skill

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
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

// ParseSkillMeta reads the YAML frontmatter block (--- ... ---) from the
// given SKILL.md file and returns the name and description fields.
// If name is missing from the frontmatter, the directory name is used.
//
// Uses yaml.v3 on the frontmatter slice instead of strings.Cut(":") so
// values containing ":" (e.g. `description: foo: bar`), quoted values
// (`name: "my:skill"`), and Windows CRLF line endings all parse
// correctly. #133.
func ParseSkillMeta(filePath string) (name, desc string, err error) {
	slog.Debug("parsing skill meta", "file", filePath)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", "", err
	}

	fm := extractFrontmatter(data)
	var meta struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}
	if len(fm) > 0 {
		if err := yaml.Unmarshal(fm, &meta); err != nil {
			// Bad frontmatter is degraded to "no metadata" — the caller
			// gets the dir-name fallback. Logged so the user knows.
			slog.Warn("skill: invalid YAML frontmatter", "path", filePath, "err", err)
		}
	}

	name = meta.Name
	desc = meta.Description
	if name == "" {
		name = filepath.Base(filepath.Dir(filePath))
	}
	return name, desc, nil
}

// extractFrontmatter returns the bytes between the opening and closing
// "---" markers, or nil when no frontmatter block is present. Tolerant
// of CRLF line endings (\r is stripped before the marker comparison).
func extractFrontmatter(data []byte) []byte {
	lines := bytes.Split(data, []byte("\n"))
	in := false
	var fm bytes.Buffer
	for _, raw := range lines {
		line := bytes.TrimRight(raw, "\r")
		if string(line) == "---" {
			if !in {
				in = true
				continue
			}
			return fm.Bytes()
		}
		if in {
			fm.Write(line)
			fm.WriteByte('\n')
		}
	}
	return nil
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
