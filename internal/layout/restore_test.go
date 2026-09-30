package layout

import (
	"reflect"
	"testing"

	"github.com/timileyinpelumi/ccshift/internal/store"
	"github.com/timileyinpelumi/ccshift/internal/term"
)

func TestPlanRestore(t *testing.T) {
	snap := store.Snapshot{Workspace: "work", Sessions: []store.Entry{
		{SessionID: "s3", CWD: "/w/gone", Name: "old worktree", Position: 3},
		{SessionID: "s1", CWD: "/w/api", Name: "api · PAY-1", Position: 1},
		{SessionID: "s2", CWD: "/w/api", Name: "api-3f", Position: 2},
		{SessionID: "s4", CWD: "/w/web", Name: "web", Position: 4},
		{SessionID: "s5", CWD: "/w/web", Position: 5},
		{SessionID: "s6", CWD: "/w/web", Name: "lost", Position: 6},
	}}
	running := map[string]bool{"s4": true}
	transcript := func(id string) bool { return id != "s6" }
	dir := func(p string) bool { return p != "/w/gone" }

	p := PlanRestore(snap, running, transcript, dir, "/bin/claude")

	wantLaunches := []term.Launch{
		{CWD: "/w/api", Title: "api · PAY-1", Argv: []string{"/bin/claude", "--resume", "s1", "--name=api · PAY-1"}},
		{CWD: "/w/api", Title: "api-3f", Argv: []string{"/bin/claude", "--resume", "s2"}},
		{CWD: "/w/web", Title: "s5", Argv: []string{"/bin/claude", "--resume", "s5"}},
	}
	if !reflect.DeepEqual(p.Launches, wantLaunches) {
		t.Fatalf("launches:\n%+v\nwant\n%+v", p.Launches, wantLaunches)
	}
	reasons := map[string]string{}
	for _, s := range p.Skipped {
		reasons[s.Entry.SessionID] = s.Reason
	}
	want := map[string]string{
		"s3": "directory no longer exists: /w/gone",
		"s4": "already running",
		"s6": "transcript not found",
	}
	if !reflect.DeepEqual(reasons, want) {
		t.Fatalf("skipped = %v", reasons)
	}
}
