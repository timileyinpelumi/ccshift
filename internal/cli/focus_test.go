package cli

import (
	"strings"
	"testing"

	"github.com/timileyinpelumi/ccshift/internal/term"
)

func TestFocus(t *testing.T) {
	h := testApp(t, term.Exact)
	h.term.tabs = []term.Tab{{ID: "p1"}, {ID: "p2"}}
	h.sessions = []liveSession{
		{id: "s1", cwd: "/p", name: "api", pane: "p1", pid: 1},
		{id: "s2", cwd: "/p", name: "web", pane: "p2", pid: 2},
	}
	h.must(t, "focus", "web")
	h.must(t, "focus", "1")
	if len(h.term.focused) != 2 || h.term.focused[0] != "p2" || h.term.focused[1] != "p1" {
		t.Fatalf("focused = %v", h.term.focused)
	}
}

func TestFocusUnsupported(t *testing.T) {
	h := testApp(t, term.LaunchOnly)
	h.sessions = []liveSession{{id: "s1", cwd: "/p", name: "api", pid: 1}}
	if code := h.run("focus", "api"); code != 1 || !strings.Contains(h.err.String(), "focus isn't supported on fake") {
		t.Fatalf("exit %d, stderr %q", code, h.err)
	}
}
