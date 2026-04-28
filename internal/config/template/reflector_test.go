package template

import (
	"reflect"
	"testing"

	"gaal/internal/config"
)

func TestReflect_ConfigRepo(t *testing.T) {
	fields := Reflect(reflect.TypeOf(config.ConfigRepo{}))
	if len(fields) != 3 {
		t.Fatalf("expected 3 fields for ConfigRepo, got %d: %+v", len(fields), fields)
	}

	tests := []struct {
		idx      int
		wantKey  string
		required bool
		enums    []string
	}{
		{0, "type", true, []string{"git", "hg", "svn", "bzr", "tar", "zip"}},
		{1, "url", true, nil},
		{2, "version", false, nil},
	}
	for _, tt := range tests {
		f := fields[tt.idx]
		if f.YAMLKey != tt.wantKey {
			t.Errorf("field[%d] YAMLKey = %q, want %q", tt.idx, f.YAMLKey, tt.wantKey)
		}
		if f.Required != tt.required {
			t.Errorf("field[%d] %q Required = %v, want %v", tt.idx, f.YAMLKey, f.Required, tt.required)
		}
		if !reflect.DeepEqual(f.Enums, tt.enums) {
			t.Errorf("field[%d] %q Enums = %v, want %v", tt.idx, f.YAMLKey, f.Enums, tt.enums)
		}
		if f.Description == "" {
			t.Errorf("field[%d] %q Description is empty", tt.idx, f.YAMLKey)
		}
	}
}

func TestReflect_ConfigMcp_TargetIsDeprecated(t *testing.T) {
	fields := Reflect(reflect.TypeOf(config.ConfigMcp{}))
	for _, f := range fields {
		if f.YAMLKey == "target" {
			if !f.Deprecated {
				t.Error("ConfigMcp.target should be marked Deprecated")
			}
			return
		}
	}
	t.Error("ConfigMcp.target field not found")
}

func TestReflect_ConfigMcp_InlineHasSubFields(t *testing.T) {
	fields := Reflect(reflect.TypeOf(config.ConfigMcp{}))
	for _, f := range fields {
		if f.YAMLKey == "inline" {
			if len(f.SubFields) == 0 {
				t.Error("inline field should have SubFields from ConfigMcpItem")
			}
			keys := make([]string, len(f.SubFields))
			for i, sf := range f.SubFields {
				keys[i] = sf.YAMLKey
			}
			for _, want := range []string{"command", "args", "env"} {
				found := false
				for _, k := range keys {
					if k == want {
						found = true
					}
				}
				if !found {
					t.Errorf("inline SubFields missing %q, got %v", want, keys)
				}
			}
			return
		}
	}
	t.Error("inline field not found in ConfigMcp")
}

func TestReflect_SkipsYAMLDash(t *testing.T) {
	fields := Reflect(reflect.TypeOf(config.Config{}))
	for _, f := range fields {
		if f.YAMLKey == "-" || f.YAMLKey == "source_path" {
			t.Errorf("field with yaml:\"-\" should be skipped, got %q", f.YAMLKey)
		}
	}
}

func TestReflect_OmitEmpty(t *testing.T) {
	// ConfigSkill.agents has yaml:"agents,omitempty"
	fields := Reflect(reflect.TypeOf(config.ConfigSkill{}))
	for _, f := range fields {
		if f.YAMLKey == "agents" {
			if !f.OmitEmpty {
				t.Error("ConfigSkill.agents should have OmitEmpty=true")
			}
			return
		}
	}
	t.Error("agents field not found in ConfigSkill")
}
