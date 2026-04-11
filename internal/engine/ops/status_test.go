package ops

import "testing"

// TestOrDefault verifies the orDefault helper.
func TestOrDefault(t *testing.T) {
	if got := orDefault("", "fallback"); got != "fallback" {
		t.Errorf("orDefault empty: got %q, want fallback", got)
	}
	if got := orDefault("value", "fallback"); got != "value" {
		t.Errorf("orDefault non-empty: got %q, want value", got)
	}
}

// TestNonNil verifies the nonNil helper.
func TestNonNil(t *testing.T) {
	if got := nonNil(nil); got == nil {
		t.Error("nonNil(nil) returned nil, want empty slice")
	}
	if got := nonNil([]string{"a"}); len(got) != 1 || got[0] != "a" {
		t.Errorf("nonNil non-nil: unexpected %v", got)
	}
}
