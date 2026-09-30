package claude

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func testReader(t *testing.T, agents []byte, agentsErr error) *Reader {
	t.Helper()
	return &Reader{
		Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name != "claude" || len(args) != 2 || args[0] != "agents" || args[1] != "--json" {
				t.Fatalf("unexpected command %s %v", name, args)
			}
			return agents, agentsErr
		},
		SessionsDir: "testdata/sessions",
		ProjectsDir: t.TempDir(),
		Alive:       func(pid int) bool { return pid != 102 },
		HasTTY:      func(pid int) bool { return pid != 103 },
		StartTime: func(pid int) (string, error) {
			if pid == 100 {
				return "5000", nil
			}
			return "1", nil
		},
	}
}

func ids(ss []Session) []string {
	var out []string
	for _, s := range ss {
		out = append(out, s.ID)
	}
	return out
}

func TestLiveFromAgents(t *testing.T) {
	b, err := os.ReadFile("testdata/agents.json")
	if err != nil {
		t.Fatal(err)
	}
	got, err := testReader(t, b, nil).Live(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"bg-1", "s-a", "s-b"}
	if g := ids(got); len(g) != len(want) || g[0] != want[0] || g[1] != want[1] || g[2] != want[2] {
		t.Fatalf("ids = %v, want %v", g, want)
	}
	if got[0].Status != "blocked" || got[1].Status != "busy" || got[1].PID != 100 || got[1].CWD != "/work/a" {
		t.Fatalf("fields not mapped: %+v", got[:2])
	}
	if got[1].StartedAt.UnixMilli() != 1790000000100 {
		t.Fatalf("StartedAt = %v", got[1].StartedAt)
	}
}

func TestLiveFallsBackToFiles(t *testing.T) {
	for name, r := range map[string]*Reader{
		"command fails": testReader(t, nil, errors.New("exit status 1")),
		"bad json":      testReader(t, []byte("{"), nil),
	} {
		got, err := r.Live(context.Background())
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		// s-reused is dropped because its recorded procStart does not match the live process.
		if g := ids(got); len(g) != 1 || g[0] != "s-a" {
			t.Fatalf("%s: ids = %v, want [s-a]", name, g)
		}
	}
}

func TestTranscriptPath(t *testing.T) {
	r := testReader(t, nil, nil)
	dir := filepath.Join(r.ProjectsDir, "-work-a")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "s-a.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if p, ok := r.TranscriptPath("s-a"); !ok || filepath.Base(p) != "s-a.jsonl" {
		t.Fatalf("TranscriptPath = %q, %v", p, ok)
	}
	if _, ok := r.TranscriptPath("missing"); ok {
		t.Fatal("found a transcript that does not exist")
	}
}
