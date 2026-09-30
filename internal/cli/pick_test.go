package cli

import (
	"strings"
	"testing"

	"github.com/timileyinpelumi/ccshift/internal/target"
	"github.com/timileyinpelumi/ccshift/internal/term"
)

func TestPickModel(t *testing.T) {
	m := &pickModel{items: []pickItem{
		{name: "api · PAY-2193", detail: "running"},
		{name: "web · checkout-v2", detail: "running"},
		{name: "docs · Install guide", detail: "saved"},
	}}
	if got := m.visible(); len(got) != 3 {
		t.Fatalf("visible = %v", got)
	}
	for _, k := range []string{"c", "h", "k"} {
		m.key(k)
	}
	if got := m.visible(); len(got) != 1 || m.items[got[0]].name != "web · checkout-v2" {
		t.Fatalf("filter 'chk' = %v", got)
	}
	m.key(keyBackspace)
	m.key(keyBackspace)
	m.key(keyBackspace)
	m.key(keyDown)
	m.key(keyDown)
	m.key(keyDown) // stays on the last
	if a, it := m.key(keyEnter); a != pickOpen || it.name != "docs · Install guide" {
		t.Fatalf("enter = %v %v", a, it.name)
	}
	m.key(keyUp)
	if a, it := m.key(keyCtrlX); a != pickHandoff || it.name != "web · checkout-v2" {
		t.Fatalf("ctrl+x = %v %v", a, it.name)
	}
	if a, _ := m.key(keyEsc); a != pickQuit {
		t.Fatal("esc should quit")
	}
	m.query = "zzz"
	if a, _ := m.key(keyEnter); a != pickNone {
		t.Fatal("enter on an empty list does nothing")
	}
}

func TestParseKeys(t *testing.T) {
	got := parseKeys([]byte("a\x1b[A\x1b[Bb\x7f\r\x18\x1b"))
	want := []string{"a", keyUp, keyDown, "b", keyBackspace, keyEnter, keyCtrlX, keyEsc}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("keys = %q", got)
	}
	if got := parseKeys([]byte("é")); len(got) != 1 || got[0] != "é" {
		t.Fatalf("utf-8 = %q", got)
	}
}

func TestPickItemsIncludeSavedSessions(t *testing.T) {
	h := testApp(t, term.Exact)
	h.term.tabs = []term.Tab{{ID: "p1"}}
	h.sessions = []liveSession{{id: "s1", cwd: "/p/1", name: "one", pane: "p1", pid: 1}}
	saveSnap(t, h, "work", "s1", "s9")
	v, _ := h.app.view(t.Context(), "")
	items := h.app.pickItems(v)
	if len(items) != 2 || items[0].id != "s1" || !strings.Contains(items[0].detail, "running") || items[1].id != "s9" || !strings.Contains(items[1].detail, "saved") {
		t.Fatalf("items = %+v", items)
	}
}

func TestPickFindsASessionByCode(t *testing.T) {
	m := &pickModel{items: []pickItem{{id: "s1-abcdef", name: "api"}, {id: "s2-abcdef", name: "web"}}}
	m.query = strings.ToLower(target.Code("s2-abcdef"))
	if v := m.visible(); len(v) != 1 || m.items[v[0]].name != "web" {
		t.Fatalf("visible = %v", v)
	}
}
