package config

import (
	"log/slog"
	"reflect"
	"strings"
)

// fieldMergePolicy maps a Config field name (Go identifier) to its maximum
// allowed ConfigScope. Fields absent from the map have no scope restriction
// (equivalent to ScopeWorkspace — any level may override them).
//
// Built once at package initialisation by buildMergePolicy.
var fieldMergePolicy = buildMergePolicy(reflect.TypeOf(Config{}))

// buildMergePolicy inspects the gaal struct tag on each field of t and
// extracts the "maxscope" key. Fields without the tag are unrestricted.
//
// Tag format: `gaal:"maxscope=<scope>"` where <scope> is one of the values
// accepted by ParseConfigScope ("global", "user", "workspace").
func buildMergePolicy(t reflect.Type) map[string]ConfigScope {
	policy := make(map[string]ConfigScope)
	for i := range t.NumField() {
		f := t.Field(i)
		tag := f.Tag.Get("gaal")
		if tag == "" {
			continue
		}
		maxScope, ok := tagValue(tag, "maxscope")
		if !ok {
			continue
		}
		scope, err := ParseConfigScope(maxScope)
		if err != nil {
			slog.Warn("config field has invalid maxscope tag value; ignoring restriction",
				"field", f.Name, "value", maxScope)
			continue
		}
		slog.Debug("config field scope restriction registered", "field", f.Name, "maxscope", scope)
		policy[f.Name] = scope
	}
	return policy
}

// tagValue extracts the value for key from a comma-separated tag string of the
// form "key=value,key2=value2". Returns ("", false) if the key is absent.
func tagValue(tag, key string) (string, bool) {
	prefix := key + "="
	for part := range strings.SplitSeq(tag, ",") {
		if strings.HasPrefix(part, prefix) {
			return strings.TrimPrefix(part, prefix), true
		}
	}
	return "", false
}

// allowedAt reports whether a Config field may be overridden by a config
// source operating at the given scope. Returns true when the field has no
// scope restriction, or when scope ≤ maxscope declared on the field.
func allowedAt(field string, scope ConfigScope) bool {
	max, restricted := fieldMergePolicy[field]
	if !restricted {
		return true
	}
	return scope <= max
}
