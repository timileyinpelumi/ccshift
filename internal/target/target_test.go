package target

import (
	"strings"
	"testing"

	"github.com/timileyinpelumi/ccshift/internal/claude"
	"github.com/timileyinpelumi/ccshift/internal/layout"
)

var items = []layout.Item{
	{Session: claude.Session{ID: "aaaa1111", PID: 10, Name: "acme · PAY-2193"}},
	{Session: claude.Session{ID: "aaaa2222", PID: 20, Name: "acme · PAY-2200"}},
	{Session: claude.Session{ID: "bbbb3333", PID: 30, Name: "trueset"}},
	{Session: claude.Session{ID: "cccc4444", PID: 40, Name: "trueset docs"}},
}

func TestResolve(t *testing.T) {
	cases := map[string]string{
		"2":               "aaaa2222",
		"trueset":         "bbbb3333", // exact beats prefix
		"TRUESET DOCS":    "cccc4444",
		"acme · PAY-2193": "aaaa1111",
		"trueset d":       "cccc4444",
		"bbbb":            "bbbb3333",
	}
	for spec, want := range cases {
		got, err := Resolve(spec, items, nil)
		if err != nil || got.Session.ID != want {
			t.Errorf("Resolve(%q) = %s, %v; want %s", spec, got.Session.ID, err, want)
		}
	}
}

func TestResolveSelf(t *testing.T) {
	got, err := Resolve(".", items, []int{999, 30, 1})
	if err != nil || got.Session.ID != "bbbb3333" {
		t.Fatalf("got %s, %v", got.Session.ID, err)
	}
	if _, err := Resolve(".", items, []int{999}); err == nil {
		t.Fatal("expected error outside a Claude session")
	}
}

func TestResolveErrors(t *testing.T) {
	for _, spec := range []string{"0", "5", "nothing", "aaa"} {
		if _, err := Resolve(spec, items, nil); err == nil {
			t.Errorf("Resolve(%q) should fail", spec)
		}
	}
	_, err := Resolve("acme", items, nil)
	if err == nil || !strings.Contains(err.Error(), "PAY-2193") || !strings.Contains(err.Error(), "PAY-2200") {
		t.Fatalf("ambiguous error = %v", err)
	}
}
