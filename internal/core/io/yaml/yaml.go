// Package ioyaml is the canonical YAML codec for gaal. Every production
// yaml.Unmarshal / yaml.Marshal call routes through here so that:
//
//   - strict-decode (KnownFields(true)) has a single home and cannot be
//     forgotten at a new call site;
//   - the PatchNodeKey helper used by telemetry consent persistence is
//     reusable across packages;
//   - direct gopkg.in/yaml.v3 imports remain only in files that
//     genuinely need yaml.Node types for custom unmarshalers or node
//     construction.
//
// For yaml.Node-level work, callers still import gopkg.in/yaml.v3
// directly — this package does not re-export those types.
package ioyaml

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

// Unmarshal decodes YAML bytes into v. Unknown keys are silently
// ignored — same behaviour as a bare yaml.Unmarshal. Use this for
// documents whose schema may legitimately carry extra fields (e.g.
// SKILL.md frontmatter, third-party YAML).
func Unmarshal(data []byte, v any) error {
	return yaml.Unmarshal(data, v)
}

// UnmarshalStrict decodes YAML bytes into v and rejects any field not
// declared on v. Implemented via yaml.NewDecoder + KnownFields(true).
// Use this for documents whose schema is owned by gaal so that typos and
// experimental keys surface as load errors rather than silent drops.
func UnmarshalStrict(data []byte, v any) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(v); err != nil {
		return err
	}
	return nil
}

// Marshal encodes v to YAML bytes. Thin wrapper around yaml.Marshal,
// kept here so call sites do not need to import gopkg.in/yaml.v3 just
// to serialise.
func Marshal(v any) ([]byte, error) {
	return yaml.Marshal(v)
}

// PatchNodeKey sets or replaces key in a yaml.Node mapping.
// Accepts either a DocumentNode wrapping a single mapping or a bare
// MappingNode. The value is encoded via yaml.Node.Encode so the
// correct YAML type tag (!!bool, !!int, !!str, …) is applied
// automatically.
//
// Moved verbatim from internal/telemetry/state.go:patchYAMLNodeKey
// (#211).
func PatchNodeKey(root *yaml.Node, key string, value any) error {
	mapping := root
	if mapping.Kind == yaml.DocumentNode && len(mapping.Content) == 1 {
		mapping = mapping.Content[0]
	}
	if mapping.Kind != yaml.MappingNode {
		return fmt.Errorf("yaml root is not a mapping node (kind=%v)", mapping.Kind)
	}

	var valDoc yaml.Node
	if err := valDoc.Encode(value); err != nil {
		return fmt.Errorf("encoding value for key %q: %w", key, err)
	}
	var valNode *yaml.Node
	if valDoc.Kind == yaml.DocumentNode && len(valDoc.Content) > 0 {
		valNode = valDoc.Content[0]
	} else {
		valNode = &valDoc
	}

	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content[i+1] = valNode
			return nil
		}
	}

	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	mapping.Content = append(mapping.Content, keyNode, valNode)
	return nil
}
