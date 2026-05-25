package telemetry

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"gaal/internal/config"
	configtemplate "gaal/internal/config/template"
	"gaal/internal/core/io/secfile"
	ioyaml "gaal/internal/core/io/yaml"
)

// consentState represents the resolved telemetry consent.
type consentState struct {
	Enabled     bool
	Source      string // "GAAL_TELEMETRY=0", "DO_NOT_TRACK=1", "config", "unconfigured"
	NeedsPrompt bool
}

// resolveState checks env vars and config to determine telemetry state.
// Precedence: GAAL_TELEMETRY > DO_NOT_TRACK > config value > unconfigured.
func resolveState(cfgValue *bool) consentState {
	if v, ok := os.LookupEnv("GAAL_TELEMETRY"); ok {
		switch strings.ToLower(v) {
		case "0", "false":
			return consentState{Enabled: false, Source: "GAAL_TELEMETRY=0"}
		case "1", "true":
			return consentState{Enabled: true, Source: "GAAL_TELEMETRY=1"}
		}
	}

	if v, ok := os.LookupEnv("DO_NOT_TRACK"); ok && v == "1" {
		return consentState{Enabled: false, Source: "DO_NOT_TRACK=1"}
	}

	if cfgValue != nil {
		return consentState{Enabled: *cfgValue, Source: "config"}
	}

	return consentState{Enabled: false, Source: "unconfigured", NeedsPrompt: true}
}

// Status returns the current telemetry state as a human-readable string
// and the source that determined it. cfgValue is the Telemetry field from
// the user config (may be nil). This does not initialise the client.
func Status(cfgValue *bool) (status string, source string) {
	s := resolveState(cfgValue)
	if s.NeedsPrompt {
		return "not configured", ""
	}
	if s.Enabled {
		return "enabled", s.Source
	}
	return "disabled", s.Source
}

// persistConsent writes or updates the telemetry field in the user config file.
//
// When the file already exists it is parsed as a yaml.Node so that all
// existing comments and key ordering are preserved; only the "telemetry" key
// is touched. When the file is absent the full documented template is generated
func persistConsent(cfgPath string, enabled bool) error {
	slog.Debug("persisting telemetry consent", "path", cfgPath, "enabled", enabled)

	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	var root yaml.Node

	data, err := os.ReadFile(cfgPath)
	if err == nil {
		// File exists — parse to yaml.Node to preserve comments and ordering.
		slog.Debug("patching existing config file", "path", cfgPath)
		if parseErr := ioyaml.Unmarshal(data, &root); parseErr != nil {
			return fmt.Errorf("parsing existing config: %w", parseErr)
		}
		// An empty file yields a zero-value Node.
		if root.Kind == 0 {
			root = yaml.Node{
				Kind:    yaml.DocumentNode,
				Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}},
			}
		}
	} else if errors.Is(err, os.ErrNotExist) {
		// File absent — generate the full documented template and parse it.
		slog.Debug("creating new config from template", "path", cfgPath)
		tmplBytes, genErr := configtemplate.Generate(config.ScopeUser)
		if genErr != nil {
			return fmt.Errorf("generating config template: %w", genErr)
		}
		if parseErr := ioyaml.Unmarshal(tmplBytes, &root); parseErr != nil {
			return fmt.Errorf("parsing generated template: %w", parseErr)
		}
	} else {
		return fmt.Errorf("reading config file: %w", err)
	}

	if err := ioyaml.PatchNodeKey(&root, "telemetry", enabled); err != nil {
		return fmt.Errorf("patching telemetry key: %w", err)
	}

	out, err := ioyaml.Marshal(&root)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	return secfile.Write(cfgPath, out)
}
