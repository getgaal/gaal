package config

import (
	"testing"
)

// ---------------------------------------------------------------------------
// indexOf
// ---------------------------------------------------------------------------

func TestIndexOf_Found(t *testing.T) {
	items := []string{"a", "b", "c"}
	got := indexOf(items, func(s string) bool { return s == "b" })
	if got != 1 {
		t.Errorf("got %d, want 1", got)
	}
}

func TestIndexOf_NotFound(t *testing.T) {
	items := []string{"a", "b", "c"}
	got := indexOf(items, func(s string) bool { return s == "z" })
	if got != -1 {
		t.Errorf("got %d, want -1", got)
	}
}

func TestIndexOf_ReturnsFirst(t *testing.T) {
	items := []string{"a", "b", "a"}
	got := indexOf(items, func(s string) bool { return s == "a" })
	if got != 0 {
		t.Errorf("got %d, want 0 (first match)", got)
	}
}

func TestIndexOf_EmptySlice(t *testing.T) {
	got := indexOf([]string{}, func(s string) bool { return s == "a" })
	if got != -1 {
		t.Errorf("got %d, want -1 for empty slice", got)
	}
}

// ---------------------------------------------------------------------------
// deduplicate
// ---------------------------------------------------------------------------

func TestDeduplicate_RemovesDuplicates(t *testing.T) {
	input := []string{"a", "b", "a", "c", "b"}
	got := deduplicate(input, func(s string) string { return s })
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestDeduplicate_KeepsFirstOccurrence(t *testing.T) {
	type item struct {
		key string
		val int
	}
	input := []item{{"x", 1}, {"y", 2}, {"x", 99}}
	got := deduplicate(input, func(i item) string { return i.key })
	if len(got) != 2 {
		t.Fatalf("got %d items, want 2", len(got))
	}
	if got[0].val != 1 {
		t.Errorf("first occurrence should be kept: got val=%d, want 1", got[0].val)
	}
}

func TestDeduplicate_NoDuplicates(t *testing.T) {
	input := []string{"a", "b", "c"}
	got := deduplicate(input, func(s string) string { return s })
	if len(got) != 3 {
		t.Errorf("got %d items, want 3 (no duplicates)", len(got))
	}
}

func TestDeduplicate_EmptySlice(t *testing.T) {
	got := deduplicate([]string{}, func(s string) string { return s })
	if len(got) != 0 {
		t.Errorf("got %v, want empty slice", got)
	}
}

func TestDeduplicate_AllDuplicates(t *testing.T) {
	input := []string{"a", "a", "a"}
	got := deduplicate(input, func(s string) string { return s })
	if len(got) != 1 || got[0] != "a" {
		t.Errorf("got %v, want [a]", got)
	}
}
