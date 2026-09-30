package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/timileyinpelumi/ccshift/internal/term"
)

// On Windows a process cannot read another's environment, so the tab a session is in comes from
// what its own hook recorded.
func TestHookRecordsTheSessionsTabForSystemsThatCannotReadIt(t *testing.T) {
	h := testApp(t, term.Exact)
	h.term.tabs = []term.Tab{{ID: "p1"}, {ID: "p2"}}
	h.sessions = []liveSession{
		{id: "s1", cwd: "/p/1", name: "one", pane: "p2", pid: 1, minute: 0},
		{id: "s2", cwd: "/p/2", name: "two", pane: "p1", pid: 2, minute: 1},
	}
	// Each session's hook runs with that session's environment.
	for _, s := range h.sessions {
		h.app.Env = map[string]string{"FAKE_PANE": s.pane, "UNRELATED": "x"}
		h.hook(t, "session-start", `{"session_id":"`+s.id+`"}`)
	}
	h.app.Env = map[string]string{}
	h.app.EnvOf = func(int) (map[string]string, error) { return nil, errors.New("not supported on this operating system") }
	out := h.must(t, "ls")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if !strings.HasPrefix(lines[1], "1  two") || !strings.HasSuffix(strings.TrimSpace(lines[1]), "fake:1") {
		t.Fatalf("tab order should come from the recorded environment:\n%s", out)
	}
	var rec map[string]map[string]string
	h.app.Store.Load("session-env.json", &rec)
	if _, leaked := rec["s1"]["UNRELATED"]; leaked || rec["s1"]["FAKE_PANE"] != "p2" {
		t.Fatalf("recorded = %v", rec)
	}
}

func TestLaunchIsolationOnWindowsGoesThroughCcshift(t *testing.T) {
	old := goos
	goos = "windows"
	defer func() { goos = old }()
	h := testApp(t, term.Exact)
	h.app.Executable = `C:/Users/u/ccshift.exe`
	got := h.app.isolated([]term.Launch{{Argv: []string{"claude", "--resume", "x"}}})[0].Argv
	want := "C:/Users/u/ccshift.exe exec -- claude --resume x"
	if strings.Join(got, " ") != want {
		t.Fatalf("argv = %q", got)
	}
}

func TestExecRunsTheCommand(t *testing.T) {
	h := testApp(t, term.Exact)
	h.must(t, "exec", "--", "claude", "--resume", "x")
	if len(h.execd) != 1 || strings.Join(h.execd[0], " ") != "claude --resume x" {
		t.Fatalf("execd = %q", h.execd)
	}
	if code := h.run("exec"); code != 2 {
		t.Fatalf("exit = %d", code)
	}
}

func TestHookCommandOnWindows(t *testing.T) {
	old := goos
	goos = "windows"
	defer func() { goos = old }()
	h := testApp(t, term.Exact)
	h.app.Executable = `C:\Users\u\AppData\Local\Programs\ccshift\ccshift.exe`
	if got := h.app.command("hook stop"); got != "C:/Users/u/AppData/Local/Programs/ccshift/ccshift.exe hook stop" {
		t.Fatalf("command = %q", got)
	}
	if !h.app.ours(h.app.command("hook stop")) || !h.app.ours(`"C:/Program Files/ccshift.exe" statusline`) {
		t.Fatal("ccshift's own Windows commands should be recognised")
	}
	h.app.Executable = `C:\Users\first last\ccshift.exe`
	if got := h.app.command("statusline"); got != `"C:/Users/first last/ccshift.exe" statusline` {
		t.Fatalf("command = %q", got)
	}
}
