package config

// indexOf returns the index of the first element in items for which match
// returns true, or -1 if none is found.
func indexOf[T any](items []T, match func(T) bool) int {
	for i, item := range items {
		if match(item) {
			return i
		}
	}
	return -1
}

// deduplicate returns a copy of items with duplicate entries removed, keeping
// the first occurrence. key extracts the deduplication key from each element.
func deduplicate[T any](items []T, key func(T) string) []T {
	seen := make(map[string]struct{}, len(items))
	out := make([]T, 0, len(items))
	for _, item := range items {
		k := key(item)
		if _, dup := seen[k]; !dup {
			seen[k] = struct{}{}
			out = append(out, item)
		}
	}
	return out
}
