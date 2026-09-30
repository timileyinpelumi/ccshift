package target

import (
	"strings"
	"testing"
)

func TestCodeIsStableAndTypeable(t *testing.T) {
	c := Code("7f3c2a10-5b1e-4c8a-9d2f-1a2b3c4d5e6f")
	if c != Code("7f3c2a10-5b1e-4c8a-9d2f-1a2b3c4d5e6f") || len(c) != 3 || strings.ContainsAny(c, "AEIOUY0123456789") {
		t.Fatalf("code = %q", c)
	}
	seen := map[string]bool{}
	for _, id := range []string{"aaaa1111", "aaaa2222", "bbbb3333", "cccc4444"} {
		seen[Code(id)] = true
	}
	if len(seen) != 4 {
		t.Fatalf("codes collide: %v", seen)
	}
}

func TestResolveByCode(t *testing.T) {
	for _, it := range items {
		got, err := Resolve(strings.ToLower(Code(it.Session.ID)), items, nil)
		if err != nil || got.Session.ID != it.Session.ID {
			t.Errorf("code %s: got %s, %v", Code(it.Session.ID), got.Session.ID, err)
		}
	}
}
