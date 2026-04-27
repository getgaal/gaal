package template

import (
	"log/slog"
	"reflect"
	"strings"
)

// FieldSpec holds the structural metadata extracted from one struct field.
type FieldSpec struct {
	YAMLKey     string      // from yaml:"name" tag (first part before comma)
	Description string      // from jsonschema:"description=..." tag
	Enums       []string    // from jsonschema:"enum=val" tags
	Required    bool        // from validate:"required" tag
	OmitEmpty   bool        // from yaml:",omitempty"
	Deprecated  bool        // true if description starts with "Deprecated:"
	MaxScope    string      // from gaal:"maxscope=..." tag
	SubFields   []FieldSpec // for pointer-to-struct fields (depth 1 only)
}

// Reflect walks t (must be a struct type) and returns FieldSpecs in
// declaration order, skipping fields tagged yaml:"-" or jsonschema:"-".
// It recurses one level deep into pointer-to-struct fields.
func Reflect(t reflect.Type) []FieldSpec {
	slog.Debug("reflecting config struct", "type", t.Name())
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}
	var specs []FieldSpec
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.Tag.Get("yaml") == "-" || field.Tag.Get("jsonschema") == "-" {
			continue
		}
		spec := parseField(field)
		if spec == nil {
			continue
		}
		specs = append(specs, *spec)
	}
	return specs
}

// parseField extracts a FieldSpec from a single struct field.
func parseField(field reflect.StructField) *FieldSpec {
	yamlTag := field.Tag.Get("yaml")
	if yamlTag == "-" {
		return nil
	}

	parts := strings.SplitN(yamlTag, ",", 2)
	key := parts[0]
	if key == "" {
		key = strings.ToLower(field.Name)
	}
	omitEmpty := len(parts) > 1 && strings.Contains(parts[1], "omitempty")

	spec := &FieldSpec{
		YAMLKey:   key,
		OmitEmpty: omitEmpty,
	}

	// Parse jsonschema tag.
	jsTag := field.Tag.Get("jsonschema")
	if jsTag == "-" {
		return nil
	}
	for _, part := range strings.Split(jsTag, ",") {
		if after, ok := strings.CutPrefix(part, "description="); ok {
			spec.Description = after
			if strings.HasPrefix(after, "Deprecated:") {
				spec.Deprecated = true
			}
		} else if after, ok := strings.CutPrefix(part, "enum="); ok {
			spec.Enums = append(spec.Enums, after)
		}
	}

	// Parse validate tag.
	validateTag := field.Tag.Get("validate")
	for _, rule := range strings.Split(validateTag, ",") {
		if strings.TrimSpace(rule) == "required" {
			spec.Required = true
		}
	}

	// Parse gaal tag.
	gaalTag := field.Tag.Get("gaal")
	if after, ok := strings.CutPrefix(gaalTag, "maxscope="); ok {
		spec.MaxScope = after
	}

	// Recurse into pointer-to-struct fields (depth 1 only).
	ft := field.Type
	if ft.Kind() == reflect.Ptr {
		ft = ft.Elem()
	}
	if ft.Kind() == reflect.Struct {
		spec.SubFields = Reflect(ft)
	}

	return spec
}
