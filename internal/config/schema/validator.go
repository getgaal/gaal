package schema

// Validator validates an arbitrary struct value and returns a structured error
// if any constraint is violated.
//
// The default implementation uses [go-playground/validator/v10].
// Swap it before loading any config with [SetValidator].
type Validator interface {
	Validate(v any) error
}

// DefaultValidator is the active validator. Initialised with [NewPlaygroundValidator].
var DefaultValidator Validator = NewPlaygroundValidator()

// SetValidator replaces the active validator.
func SetValidator(v Validator) {
	DefaultValidator = v
}

// Validate is a package-level convenience wrapper around [DefaultValidator].
func Validate(v any) error {
	return DefaultValidator.Validate(v)
}
