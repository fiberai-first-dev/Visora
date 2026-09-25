package db

import "testing"

func TestNewPublicID(t *testing.T) {
	seen := map[string]struct{}{}
	for i := 0; i < 200; i++ {
		id := NewPublicID()
		if len(id) != 12 {
			t.Fatalf("id %q has length %d, want 12", id, len(id))
		}
		if _, ok := seen[id]; ok {
			t.Fatalf("duplicate public id %q", id)
		}
		seen[id] = struct{}{}
		for _, ch := range id {
			if !containsRune(publicIDAlphabet, ch) {
				t.Fatalf("id %q has invalid character %q", id, ch)
			}
		}
	}
}

func containsRune(s string, r rune) bool {
	for _, ch := range s {
		if ch == r {
			return true
		}
	}
	return false
}
