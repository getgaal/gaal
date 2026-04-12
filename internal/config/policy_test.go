package config

import (
	"reflect"
	"testing"
)

// ---------------------------------------------------------------------------
// buildMergePolicy
// ---------------------------------------------------------------------------

func TestBuildMergePolicy_TelemetryRestricted(t *testing.T) {
	policy := buildMergePolicy(reflect.TypeOf(Config{}))
	scope, ok := policy["Telemetry"]
	if !ok {
		t.Fatal("expected Telemetry to be present in merge policy")
	}
	if scope != ScopeUser {
		t.Errorf("expected Telemetry maxscope=user, got %v", scope)
	}
}

func TestBuildMergePolicy_UntaggedFieldAbsent(t *testing.T) {
	policy := buildMergePolicy(reflect.TypeOf(Config{}))
	// Fields without a gaal tag must not appear in the policy map.
	for _, unrestricted := range []string{"Schema", "Repositories", "Skills", "MCPs", "SourcePath"} {
		if _, ok := policy[unrestricted]; ok {
			t.Errorf("field %q should not be in merge policy (no gaal tag)", unrestricted)
		}
	}
}

// ---------------------------------------------------------------------------
// buildMergePolicy with custom structs
// ---------------------------------------------------------------------------

func TestBuildMergePolicy_ValidTag(t *testing.T) {
	type testStruct struct {
		Foo string `gaal:"maxscope=global"`
		Bar string `gaal:"maxscope=user"`
		Baz string `gaal:"maxscope=workspace"`
		Qux string // no gaal tag
	}

	policy := buildMergePolicy(reflect.TypeOf(testStruct{}))

	tests := []struct {
		field string
		want  ConfigScope
	}{
		{"Foo", ScopeGlobal},
		{"Bar", ScopeUser},
		{"Baz", ScopeWorkspace},
	}
	for _, tt := range tests {
		got, ok := policy[tt.field]
		if !ok {
			t.Errorf("field %q missing from policy", tt.field)
			continue
		}
		if got != tt.want {
			t.Errorf("policy[%q] = %v, want %v", tt.field, got, tt.want)
		}
	}
	if _, ok := policy["Qux"]; ok {
		t.Error("field Qux (no tag) should not appear in policy")
	}
}

func TestBuildMergePolicy_InvalidTagIgnored(t *testing.T) {
	type testStruct struct {
		Bad string `gaal:"maxscope=unknown"`
	}
	// Must not panic; the invalid value is silently skipped.
	policy := buildMergePolicy(reflect.TypeOf(testStruct{}))
	if _, ok := policy["Bad"]; ok {
		t.Error("field with invalid maxscope tag should not appear in policy")
	}
}

func TestBuildMergePolicy_EmptyStruct(t *testing.T) {
	type empty struct{}
	policy := buildMergePolicy(reflect.TypeOf(empty{}))
	if len(policy) != 0 {
		t.Errorf("expected empty policy for empty struct, got %v", policy)
	}
}

// ---------------------------------------------------------------------------
// tagValue
// ---------------------------------------------------------------------------

func TestTagValue_Present(t *testing.T) {
	tests := []struct {
		tag  string
		key  string
		want string
	}{
		{"maxscope=user", "maxscope", "user"},
		{"maxscope=global,other=x", "maxscope", "global"},
		{"first=a,maxscope=workspace", "maxscope", "workspace"},
	}
	for _, tt := range tests {
		got, ok := tagValue(tt.tag, tt.key)
		if !ok {
			t.Errorf("tagValue(%q, %q): expected found, got not found", tt.tag, tt.key)
			continue
		}
		if got != tt.want {
			t.Errorf("tagValue(%q, %q) = %q, want %q", tt.tag, tt.key, got, tt.want)
		}
	}
}

func TestTagValue_Absent(t *testing.T) {
	tests := []struct {
		tag string
		key string
	}{
		{"other=val", "maxscope"},
		{"", "maxscope"},
		{"maxscop=user", "maxscope"}, // near-miss
	}
	for _, tt := range tests {
		_, ok := tagValue(tt.tag, tt.key)
		if ok {
			t.Errorf("tagValue(%q, %q): expected not found, got found", tt.tag, tt.key)
		}
	}
}

// ---------------------------------------------------------------------------
// allowedAt
// ---------------------------------------------------------------------------

func TestAllowedAt_UnrestrictedField(t *testing.T) {
	// Fields not in fieldMergePolicy are always allowed.
	for _, scope := range []ConfigScope{ScopeGlobal, ScopeUser, ScopeWorkspace} {
		if !allowedAt("Repositories", scope) {
			t.Errorf("allowedAt(Repositories, %v) = false, want true", scope)
		}
	}
}

func TestAllowedAt_TelemetryMaxUser(t *testing.T) {
	tests := []struct {
		scope ConfigScope
		want  bool
	}{
		{ScopeGlobal, true},     // global ≤ user → allowed
		{ScopeUser, true},       // user ≤ user   → allowed
		{ScopeWorkspace, false}, // workspace > user → denied
	}
	for _, tt := range tests {
		got := allowedAt("Telemetry", tt.scope)
		if got != tt.want {
			t.Errorf("allowedAt(Telemetry, %v) = %v, want %v", tt.scope, got, tt.want)
		}
	}
}
