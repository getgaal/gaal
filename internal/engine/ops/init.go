package ops

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"gaal/internal/config"
	configtemplate "gaal/internal/config/template"
)

// Init writes the documented gaal.yaml skeleton to dest.
// When force is false and dest already exists, an error is returned so the
// caller can surface an actionable message without silently overwriting work.
func Init(dest string, force bool) error {
	slog.Debug("init", "dest", dest, "force", force)

	if err := checkDestination(dest, force); err != nil {
		return err
	}

	tmpl, err := configtemplate.Generate(config.ScopeWorkspace)
	if err != nil {
		return fmt.Errorf("generating template: %w", err)
	}

	if err := ensureParentDir(dest); err != nil {
		return err
	}

	if err := os.WriteFile(dest, tmpl, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", dest, err)
	}

	slog.Info("config file created", "path", dest)
	return nil
}

// InitFromPlan writes a gaal.yaml that reuses the documented comment headers
// of the generated template but replaces the empty skills: / mcps: blocks
// with the entries carried by plan. repositories: remains empty.
//
// The force and dest-existence rules are identical to Init.
func InitFromPlan(dest string, plan Plan, force bool) error {
	slog.Debug("init from plan", "dest", dest, "force", force,
		"skills", len(plan.Skills), "mcps", len(plan.MCPs))

	if err := checkDestination(dest, force); err != nil {
		return err
	}

	content, err := renderPlanYAML(plan)
	if err != nil {
		return fmt.Errorf("rendering plan: %w", err)
	}

	if err := ensureParentDir(dest); err != nil {
		return err
	}

	if err := os.WriteFile(dest, content, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", dest, err)
	}

	slog.Info("config file created from audit plan", "path", dest,
		"skills", len(plan.Skills), "mcps", len(plan.MCPs))
	return nil
}

// ensureParentDir creates the parent directory of dest if it does not yet
// exist. This is safe for both project- and user-scope destinations: the
// user-scope path (e.g. ~/.config/gaal/config.yaml) may legitimately not
// exist yet on a fresh install.
func ensureParentDir(dest string) error {
	parent := filepath.Dir(dest)
	if parent == "" || parent == "." {
		return nil
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("creating parent directory %s: %w", parent, err)
	}
	return nil
}

func checkDestination(dest string, force bool) error {
	if force {
		return nil
	}
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("%s already exists — use --force to overwrite", dest)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("checking %s: %w", dest, err)
	}
	return nil
}

// renderPlanYAML keeps the three commented headers from the generated template
// and replaces the empty repositories: / skills: / mcps: blocks with the plan
// content serialised via yaml.v3 for safety.
func renderPlanYAML(plan Plan) ([]byte, error) {
	tmpl, err := configtemplate.Generate(config.ScopeWorkspace)
	if err != nil {
		return nil, fmt.Errorf("generating template: %w", err)
	}
	headers, err := splitTemplateHeaders(tmpl)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	buf.Write(headers.intro)

	buf.Write(headers.repositories)
	buf.WriteString("repositories: {}\n\n")

	buf.Write(headers.skills)
	if err := writeSkillsBlock(&buf, plan.Skills); err != nil {
		return nil, err
	}
	buf.WriteString("\n")

	buf.Write(headers.mcps)
	if err := writeMCPsBlock(&buf, plan.MCPs); err != nil {
		return nil, err
	}
	buf.WriteString("\n")

	return buf.Bytes(), nil
}

func writeSkillsBlock(buf *bytes.Buffer, skills []config.ConfigSkill) error {
	if len(skills) == 0 {
		buf.WriteString("skills: []\n")
		return nil
	}
	raw, err := yaml.Marshal(map[string]any{"skills": skills})
	if err != nil {
		return err
	}
	buf.Write(raw)
	return nil
}

func writeMCPsBlock(buf *bytes.Buffer, mcps []config.ConfigMcp) error {
	if len(mcps) == 0 {
		buf.WriteString("mcps: []\n")
		return nil
	}
	raw, err := yaml.Marshal(map[string]any{"mcps": mcps})
	if err != nil {
		return err
	}
	buf.Write(raw)
	return nil
}

// templateHeaders holds the comment blocks extracted from the generated template.
type templateHeaders struct {
	intro        []byte // top-of-file comments before the first section
	repositories []byte // "# ── repositories ──" block
	skills       []byte // "# ── skills ──" block
	mcps         []byte // "# ── mcps ──" block
}

// splitTemplateHeaders parses the template bytes and returns its comment
// blocks. The YAML keys themselves are excluded from each block so the
// caller can re-emit them with real content.
func splitTemplateHeaders(tmpl []byte) (templateHeaders, error) {
	idxRepos := bytes.Index(tmpl, []byte("repositories:"))
	idxSkills := bytes.Index(tmpl, []byte("skills:"))
	idxMcps := bytes.Index(tmpl, []byte("mcps:"))
	if idxRepos < 0 || idxSkills < 0 || idxMcps < 0 {
		return templateHeaders{}, fmt.Errorf("init template missing a required section key")
	}
	return templateHeaders{
		intro:        tmpl[:commentBlockStart(tmpl, idxRepos)],
		repositories: extractHeaderBlock(tmpl, idxRepos),
		skills:       extractHeaderBlock(tmpl, idxSkills),
		mcps:         extractHeaderBlock(tmpl, idxMcps),
	}, nil
}

// commentBlockStart walks backwards from keyIdx over consecutive comment /
// blank lines, returning the byte offset where the comment block starts.
func commentBlockStart(tmpl []byte, keyIdx int) int {
	lines := bytes.Split(tmpl[:keyIdx], []byte("\n"))
	start := keyIdx
	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := bytes.TrimSpace(lines[i])
		if len(trimmed) == 0 || bytes.HasPrefix(trimmed, []byte("#")) {
			// Subtract this line plus its newline.
			start -= len(lines[i]) + 1
			continue
		}
		break
	}
	if start < 0 {
		start = 0
	}
	return start
}

// extractHeaderBlock returns the comment block immediately preceding keyIdx,
// ending right before the key itself.
func extractHeaderBlock(tmpl []byte, keyIdx int) []byte {
	start := commentBlockStart(tmpl, keyIdx)
	if start < 0 {
		start = 0
	}
	return tmpl[start:keyIdx]
}
