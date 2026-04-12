// Package schema provides swappable abstractions for JSON Schema generation
// and struct validation used by the config package.
package schema

// Generator generates a JSON Schema for an arbitrary Go value.
// The returned bytes are a valid JSON document conforming to JSON Schema draft 2020-12.
//
// The default implementation uses [invopop/jsonschema] ([NewGeneratorInvopop]).
// Swap it at program start-up with [Set].
type Generator interface {
	Generate(v any) ([]byte, error)
}

// Default is the active Generator implementation. Initialised with [NewGeneratorInvopop].
var Default Generator = NewGeneratorInvopop()

// Set replaces the active Generator implementation.
// Call this before any invocation of [Generate] (e.g. in main or TestMain).
func Set(g Generator) {
	Default = g
}

// Generate is a package-level convenience wrapper around [Default].
func Generate(v any) ([]byte, error) {
	return Default.Generate(v)
}
