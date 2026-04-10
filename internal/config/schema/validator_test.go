package schema_test

import (
	"testing"

	"gaal/internal/config/schema"
)

type validStruct struct {
	Field string `validate:"required"`
}

type invalidStruct struct {
	Field string `validate:"required"`
}

func TestDefaultValidator_Valid(t *testing.T) {
	if err := schema.Validate(&validStruct{Field: "ok"}); err != nil {
		t.Errorf("expected no error for valid struct, got: %v", err)
	}
}

func TestDefaultValidator_Invalid(t *testing.T) {
	if err := schema.Validate(&invalidStruct{}); err == nil {
		t.Error("expected validation error for empty required field")
	}
}

func TestDefaultValidator_SetValidator(t *testing.T) {
	original := schema.DefaultValidator
	t.Cleanup(func() { schema.SetValidator(original) })

	called := false
	schema.SetValidator(&mockValidator{fn: func(v any) error {
		called = true
		return nil
	}})
	_ = schema.Validate(&invalidStruct{})
	if !called {
		t.Error("custom Validator was not called after SetValidator()")
	}
}

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

type mockValidator struct {
	fn func(any) error
}

func (m *mockValidator) Validate(v any) error { return m.fn(v) }
