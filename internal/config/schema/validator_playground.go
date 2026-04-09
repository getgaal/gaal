package schema

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

type PlaygroundValidator struct {
	v *validator.Validate
}

func NewPlaygroundValidator() *PlaygroundValidator {
	v := validator.New()
	// Use the "yaml" tag to derive field names in error messages so they match
	// what users see in their YAML files.
	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		tag := fld.Tag.Get("yaml")
		if tag == "" || tag == "-" {
			return fld.Tag.Get("json")
		}
		// Strip yaml options like ",omitempty".
		if idx := strings.Index(tag, ","); idx != -1 {
			tag = tag[:idx]
		}
		return tag
	})
	return &PlaygroundValidator{v: v}
}

// Validate runs all registered constraints and returns a human-readable error
// that lists every violated field.
func (p *PlaygroundValidator) Validate(v any) error {
	err := p.v.Struct(v)
	if err == nil {
		return nil
	}

	var ve validator.ValidationErrors
	if !errors.As(err, &ve) {
		return err
	}

	msgs := make([]string, 0, len(ve))
	for _, fe := range ve {
		msgs = append(msgs, fieldError(fe))
	}
	return fmt.Errorf("validation failed:\n  %s", strings.Join(msgs, "\n  "))
}

func fieldError(fe validator.FieldError) string {
	field := fe.Namespace()
	// Remove the root struct name prefix (e.g. "Config.").
	if idx := strings.Index(field, "."); idx != -1 {
		field = field[idx+1:]
	}

	switch fe.Tag() {
	case "required":
		return fmt.Sprintf("%s: required", field)
	case "oneof":
		return fmt.Sprintf("%s: must be one of [%s], got %q", field, fe.Param(), fe.Value())
	case "required_without":
		return fmt.Sprintf("%s: required when %s is absent", field, fe.Param())
	default:
		return fmt.Sprintf("%s: failed %s constraint (param=%s)", field, fe.Tag(), fe.Param())
	}
}
