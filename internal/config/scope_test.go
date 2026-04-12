package config

import "testing"

// ---------------------------------------------------------------------------
// ConfigScope.String
// ---------------------------------------------------------------------------

func TestConfigScope_String(t *testing.T) {
	tests := []struct {
		scope ConfigScope
		want  string
	}{
		{ScopeGlobal, "global"},
		{ScopeUser, "user"},
		{ScopeWorkspace, "workspace"},
		{ConfigScope(99), "ConfigScope(99)"},
	}
	for _, tt := range tests {
		got := tt.scope.String()
		if got != tt.want {
			t.Errorf("ConfigScope(%d).String() = %q, want %q", int(tt.scope), got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// ParseConfigScope
// ---------------------------------------------------------------------------

func TestParseConfigScope_Valid(t *testing.T) {
	tests := []struct {
		input string
		want  ConfigScope
	}{
		{"global", ScopeGlobal},
		{"user", ScopeUser},
		{"workspace", ScopeWorkspace},
	}
	for _, tt := range tests {
		got, err := ParseConfigScope(tt.input)
		if err != nil {
			t.Errorf("ParseConfigScope(%q) unexpected error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseConfigScope(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestParseConfigScope_Invalid(t *testing.T) {
	invalids := []string{"", "Global", "USER", "project", "system", "42"}
	for _, s := range invalids {
		_, err := ParseConfigScope(s)
		if err == nil {
			t.Errorf("ParseConfigScope(%q) expected error, got nil", s)
		}
	}
}

// ---------------------------------------------------------------------------
// Ordering invariant
// ---------------------------------------------------------------------------

func TestConfigScope_Ordering(t *testing.T) {
	if ScopeGlobal >= ScopeUser {
		t.Error("ScopeGlobal must be < ScopeUser")
	}
	if ScopeUser >= ScopeWorkspace {
		t.Error("ScopeUser must be < ScopeWorkspace")
	}
}
