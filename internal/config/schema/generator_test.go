package schema_test

import (
	"encoding/json"
	"testing"

	"gaal/internal/config/schema"
)

type sampleStruct struct {
	Name  string `json:"name"  jsonschema:"description=The name,required"`
	Count int    `json:"count" jsonschema:"description=A counter"`
}

func TestDefaultGenerator_ReturnsValidJSON(t *testing.T) {
	data, err := schema.Generate(&sampleStruct{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("Generate returned empty bytes")
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	// invopop/jsonschema places type definitions in $defs and adds a $ref at root.
	// Accept both layouts: top-level "properties" (ExpandedStruct) OR "$defs".
	_, hasDefs := m["$defs"]
	_, hasProps := m["properties"]
	if !hasDefs && !hasProps {
		t.Errorf("expected either %q or %q key in JSON Schema output, got keys: %v", "$defs", "properties", keys(m))
	}
}

func TestDefaultGenerator_Set(t *testing.T) {
	original := schema.Default
	t.Cleanup(func() { schema.Set(original) })

	called := false
	schema.Set(&mockGenerator{fn: func(v any) ([]byte, error) {
		called = true
		return []byte(`{}`), nil
	}})
	if _, err := schema.Generate(&sampleStruct{}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Error("custom Generator was not called after Set()")
	}
}

// ---------------------------------------------------------------------------
// Mocks / helpers
// ---------------------------------------------------------------------------

type mockGenerator struct {
	fn func(any) ([]byte, error)
}

func (m *mockGenerator) Generate(v any) ([]byte, error) { return m.fn(v) }

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
