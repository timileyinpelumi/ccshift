package cli

import (
	"strings"
	"testing"

	"github.com/timileyinpelumi/ccshift/internal/term"
)

func TestWs(t *testing.T) {
	h := testApp(t, term.Exact)
	h.app.Config.Workspaces = map[string][]string{"work": {"/w"}, "personal": {"/p"}}
	saveSnap(t, h, "personal", "s1", "s2")
	h.sessions = []liveSession{{id: "s1", cwd: "/p/s1", name: "a", pid: 1}}
	out := h.must(t, "ws")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[1], "personal") || !strings.HasPrefix(lines[2], "work") {
		t.Fatalf("out:\n%s", out)
	}
	if f := strings.Fields(lines[1]); f[len(f)-3] != "2" || f[len(f)-2] != "1" {
		t.Fatalf("personal row = %q", lines[1])
	}
}
