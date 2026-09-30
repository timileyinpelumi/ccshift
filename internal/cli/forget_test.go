package cli

import (
	"strings"
	"testing"

	"github.com/timileyinpelumi/ccshift/internal/term"
)

func TestForget(t *testing.T) {
	h := testApp(t, term.Exact)
	saveSnap(t, h, "work", "s1", "s2", "s3")
	h.must(t, "forget", "name s2")
	snap, _, _ := h.app.Store.Latest("work")
	if len(snap.Sessions) != 2 || snap.Sessions[1].SessionID != "s3" || snap.Sessions[1].Position != 2 {
		t.Fatalf("snapshot = %+v", snap)
	}
	h.sessions = []liveSession{{id: "s1", cwd: "/p/s1", name: "name s1", pid: 3}}
	if code := h.run("forget", "name s1"); code != 1 || !strings.Contains(h.err.String(), "still running") {
		t.Fatalf("exit %d, stderr %q", code, h.err)
	}
	if code := h.run("forget", "name"); code != 1 || !strings.Contains(h.err.String(), "more than one") {
		t.Fatalf("ambiguous: exit %d, stderr %q", code, h.err)
	}
}
