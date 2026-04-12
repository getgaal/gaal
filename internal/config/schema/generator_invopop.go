package schema

import (
	"encoding/json"

	"github.com/invopop/jsonschema"
)

// GeneratorInvopop is the [Generator] implementation backed by [invopop/jsonschema].
// Use [NewGeneratorInvopop] to create one, or [Set] to swap it as the global default.
type GeneratorInvopop struct {
	reflector *jsonschema.Reflector
}

// NewGeneratorInvopop creates a [GeneratorInvopop] with sensible defaults:
// additional properties are rejected and all definitions are placed in $defs.
func NewGeneratorInvopop() *GeneratorInvopop {
	r := &jsonschema.Reflector{
		// Fields tagged `validate:"required"` (or json-schema required) are marked required.
		RequiredFromJSONSchemaTags: false,
		// Reject unknown keys so IDE validation catches typos.
		AllowAdditionalProperties: false,
		// Prefer inline definitions over $ref for simple types.
		DoNotReference: false,
		// Use field names from json tags, not Go identifiers.
		ExpandedStruct: false,
	}
	return &GeneratorInvopop{reflector: r}
}

// Generate reflects v and returns a pretty-printed JSON Schema (draft 2020-12).
func (p *GeneratorInvopop) Generate(v any) ([]byte, error) {
	s := p.reflector.Reflect(v)
	out, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return nil, err
	}
	return out, nil
}
