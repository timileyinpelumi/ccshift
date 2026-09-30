package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/timileyinpelumi/ccshift/internal/target"
	"github.com/timileyinpelumi/ccshift/internal/term"
)

func TestLsOrdersByTab(t *testing.T) {
	h := testApp(t, term.Exact)
	h.app.Config.Workspaces = map[string][]string{"work": {"/w"}}
	h.term.tabs = []term.Tab{{ID: "p2", Label: "work:3"}, {ID: "p1"}}
	h.sessions = []liveSession{
		{id: "s-api", cwd: "/w/api", name: "api", pane: "p1", pid: 11, minute: 0},
		{id: "s-web", cwd: "/w/web", name: "web", pane: "p2", pid: 12, minute: 5},
		{id: "s-home", cwd: "/home/u", name: "home", pid: 13, minute: 10},
	}
	out := h.must(t, "ls")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 4 {
		t.Fatalf("output:\n%s", out)
	}
	for i, want := range [][2]string{{"1", "web"}, {"2", "api"}, {"3", "home"}} {
		if f := strings.Fields(lines[i+1]); f[0] != want[0] || f[2] != want[1] {
			t.Errorf("line %d = %q, want prefix %q", i+1, lines[i+1], want)
		}
	}
	if !strings.HasSuffix(strings.TrimSpace(lines[1]), "fake:work:3") || !strings.HasSuffix(strings.TrimSpace(lines[2]), "fake:2") || !strings.HasSuffix(strings.TrimSpace(lines[3]), "-") {
		t.Errorf("tab column wrong:\n%s", out)
	}
}

func TestLsJSONAndEmpty(t *testing.T) {
	h := testApp(t, term.Exact)
	if out := h.must(t, "ls"); !strings.Contains(out, "No Claude sessions running.") {
		t.Fatalf("empty output = %q", out)
	}
	h.sessions = []liveSession{{id: "s1", cwd: "/p", name: "one", pid: 5}}
	var got struct {
		Terminal string `json:"terminal"`
		Sessions []struct {
			Name      string `json:"name"`
			SessionID string `json:"session_id"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal([]byte(h.must(t, "ls", "--json")), &got); err != nil {
		t.Fatal(err)
	}
	if got.Terminal != "fake" || len(got.Sessions) != 1 || got.Sessions[0].SessionID != "s1" {
		t.Fatalf("json = %+v", got)
	}
}

func TestLsShowsSessionCodes(t *testing.T) {
	h := testApp(t, term.Exact)
	h.term.tabs = []term.Tab{{ID: "p1"}}
	h.sessions = []liveSession{{id: "s1-abcdef", cwd: "/w/api", name: "api", pane: "p1", pid: 1}}
	out := h.must(t, "ls")
	if !strings.Contains(out, "CODE") || !strings.Contains(out, target.Code("s1-abcdef")) {
		t.Fatalf("ls:\n%s", out)
	}
	h.must(t, "focus", strings.ToLower(target.Code("s1-abcdef")))
	if len(h.term.focused) != 1 {
		t.Fatal("focus by code did not switch")
	}
}
