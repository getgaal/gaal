package ioyaml_test

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	ioyaml "gaal/internal/core/io/yaml"
)

type sample struct {
	Name  string `yaml:"name"`
	Count int    `yaml:"count"`
	On    bool   `yaml:"on"`
}

func TestUnmarshal_HappyPath(t *testing.T) {
	var s sample
	if err := ioyaml.Unmarshal([]byte("name: gaal\ncount: 3\non: true\n"), &s); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if s.Name != "gaal" || s.Count != 3 || !s.On {
		t.Errorf("decoded = %+v, want {gaal 3 true}", s)
	}
}

func TestUnmarshal_Malformed(t *testing.T) {
	var s sample
	err := ioyaml.Unmarshal([]byte("name: gaal\n  bad: indent\n"), &s)
	if err == nil {
		t.Fatal("expected error for malformed YAML, got nil")
	}
}

func TestUnmarshal_UnknownKeyTolerated(t *testing.T) {
	var s sample
	if err := ioyaml.Unmarshal([]byte("name: gaal\nmystery: 1\n"), &s); err != nil {
		t.Fatalf("Unmarshal should tolerate unknown keys, got %v", err)
	}
	if s.Name != "gaal" {
		t.Errorf("Name = %q, want %q", s.Name, "gaal")
	}
}

func TestUnmarshalStrict_HappyPath(t *testing.T) {
	var s sample
	if err := ioyaml.UnmarshalStrict([]byte("name: gaal\ncount: 3\n"), &s); err != nil {
		t.Fatalf("UnmarshalStrict: %v", err)
	}
	if s.Name != "gaal" || s.Count != 3 {
		t.Errorf("decoded = %+v", s)
	}
}

func TestUnmarshalStrict_Malformed(t *testing.T) {
	var s sample
	err := ioyaml.UnmarshalStrict([]byte("name: gaal\n  bad: indent\n"), &s)
	if err == nil {
		t.Fatal("expected error for malformed YAML, got nil")
	}
}

func TestUnmarshalStrict_RejectsUnknownKey(t *testing.T) {
	var s sample
	err := ioyaml.UnmarshalStrict([]byte("name: gaal\nmystery: 1\n"), &s)
	if err == nil {
		t.Fatal("expected error for unknown key, got nil")
	}
	if !strings.Contains(err.Error(), "mystery") {
		t.Errorf("error %q should mention the unknown field name", err)
	}
}

func TestMarshal_RoundTrip(t *testing.T) {
	in := sample{Name: "gaal", Count: 7, On: true}
	out, err := ioyaml.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var back sample
	if err := ioyaml.Unmarshal(out, &back); err != nil {
		t.Fatalf("round-trip Unmarshal: %v", err)
	}
	if back != in {
		t.Errorf("round-trip = %+v, want %+v", back, in)
	}
}

func TestPatchNodeKey_InsertWhenAbsent(t *testing.T) {
	root := yaml.Node{
		Kind:    yaml.DocumentNode,
		Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}},
	}
	if err := ioyaml.PatchNodeKey(&root, "telemetry", true); err != nil {
		t.Fatalf("PatchNodeKey: %v", err)
	}
	mapping := root.Content[0]
	if len(mapping.Content) != 2 {
		t.Fatalf("expected 2 child nodes (key+value), got %d", len(mapping.Content))
	}
	if mapping.Content[0].Value != "telemetry" {
		t.Errorf("key = %q, want telemetry", mapping.Content[0].Value)
	}
	if mapping.Content[1].Value != "true" || mapping.Content[1].Tag != "!!bool" {
		t.Errorf("value = %+v, want true with !!bool tag", mapping.Content[1])
	}
}

func TestPatchNodeKey_ReplaceWhenPresent(t *testing.T) {
	var root yaml.Node
	if err := yaml.Unmarshal([]byte("key: val\ntelemetry: false\n"), &root); err != nil {
		t.Fatalf("seed unmarshal: %v", err)
	}
	if err := ioyaml.PatchNodeKey(&root, "telemetry", true); err != nil {
		t.Fatalf("PatchNodeKey: %v", err)
	}
	out, err := yaml.Marshal(&root)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(out)
	if !strings.Contains(got, "telemetry: true") {
		t.Errorf("expected telemetry: true in %q", got)
	}
	if strings.Contains(got, "telemetry: false") {
		t.Errorf("old value not replaced in %q", got)
	}
	if !strings.Contains(got, "key: val") {
		t.Errorf("sibling key dropped from %q", got)
	}
}

func TestPatchNodeKey_AcceptsBareMapping(t *testing.T) {
	mapping := yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	if err := ioyaml.PatchNodeKey(&mapping, "k", "v"); err != nil {
		t.Fatalf("PatchNodeKey on bare mapping: %v", err)
	}
	if len(mapping.Content) != 2 {
		t.Fatalf("expected 2 children, got %d", len(mapping.Content))
	}
}

func TestPatchNodeKey_RejectsNonMapping(t *testing.T) {
	root := yaml.Node{
		Kind:    yaml.DocumentNode,
		Content: []*yaml.Node{{Kind: yaml.SequenceNode}},
	}
	err := ioyaml.PatchNodeKey(&root, "k", "v")
	if err == nil {
		t.Fatal("expected error for non-mapping root, got nil")
	}
}

func TestPatchNodeKey_ValueTagging(t *testing.T) {
	cases := []struct {
		name string
		val  any
		tag  string
	}{
		{"bool", true, "!!bool"},
		{"int", 42, "!!int"},
		{"string", "hello", "!!str"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := yaml.Node{
				Kind:    yaml.DocumentNode,
				Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}},
			}
			if err := ioyaml.PatchNodeKey(&root, "k", tc.val); err != nil {
				t.Fatalf("PatchNodeKey: %v", err)
			}
			got := root.Content[0].Content[1].Tag
			if got != tc.tag {
				t.Errorf("tag = %q, want %q", got, tc.tag)
			}
		})
	}
}
