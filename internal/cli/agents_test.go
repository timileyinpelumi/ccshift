package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/timileyinpelumi/ccshift/internal/settings"
	"github.com/timileyinpelumi/ccshift/internal/term"
)

// A Codex or Gemini session is known from its own hooks: the hook records the session against
// the agent process that ran it.
func TestOtherAgentsAreTrackedAndRestored(t *testing.T) {
	h := testApp(t, term.Exact)
	h.term.tabs = []term.Tab{{ID: "p1"}, {ID: "p2"}}
	h.app.SelfPID = 500
	// hook process 500 -> sh 400 -> codex 300
	h.app.AncestorPIDs = func(int) []int { return []int{400, 300, 1} }
	h.app.Comms = func(pids []int) []string {
		names := map[int]string{400: "sh", 300: "codex"}
		var out []string
		for _, p := range pids {
			out = append(out, names[p])
		}
		return out
	}
	alive := map[int]bool{300: true}
	h.app.Claude.Alive = func(pid int) bool { return alive[pid] }
	h.app.Env = map[string]string{"FAKE_PANE": "p2"}
	h.hook(t, "session-start", `{"session_id":"cx-1","cwd":"/w/api"}`, "--agent", "codex")
	h.app.Env = map[string]string{}

	out := h.must(t, "ls")
	if !strings.Contains(out, "codex") || !strings.Contains(out, "fake:1") && !strings.Contains(out, "fake:2") {
		t.Fatalf("ls:\n%s", out)
	}
	snap, _, _ := h.app.Store.Latest("default")
	if len(snap.Sessions) != 1 || snap.Sessions[0].Agent != "codex" {
		t.Fatalf("saved = %+v", snap.Sessions)
	}
	// After a reboot the process is gone and restore resumes it with codex.
	alive[300] = false
	h.must(t, "restore")
	if len(h.term.opened) != 1 || strings.Join(h.term.opened[0][0].Argv, " ") != "codex resume cx-1" || h.term.opened[0][0].CWD != "/w/api" {
		t.Fatalf("opened = %+v", h.term.opened)
	}
	if code := h.run("handoff", "cx-1"); code == 0 || !strings.Contains(h.err.String(), "Claude Code") {
		t.Fatalf("handoff of a Codex session: exit %d %q", code, h.err)
	}
}

func TestGeminiRestoreCommandAndExit(t *testing.T) {
	h := testApp(t, term.Exact)
	saveSnap(t, h, "work", "g-1")
	snap, _, _ := h.app.Store.Latest("work")
	snap.Sessions[0].Agent = "gemini"
	h.app.Store.SaveLatest(snap)
	h.must(t, "restore")
	if got := strings.Join(h.term.opened[0][0].Argv, " "); got != "gemini --resume g-1" {
		t.Fatalf("argv = %q", got)
	}
	h.hook(t, "session-end", `{"session_id":"g-1","reason":"exit"}`, "--agent", "gemini")
	if len(latestIDs(t, h, "work")) != 0 {
		t.Fatal("a Gemini exit should leave the save")
	}
}

func TestInitAddsHooksForInstalledAgents(t *testing.T) {
	h, _ := initApp(t, userSettings)
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".codex"), 0o755)
	os.MkdirAll(filepath.Join(home, ".gemini"), 0o755)
	os.WriteFile(filepath.Join(home, ".gemini", "settings.json"), []byte(`{"theme":"x"}`), 0o644)
	h.app.AgentHomes = map[string]string{"codex": filepath.Join(home, ".codex"), "gemini": filepath.Join(home, ".gemini")}
	out := h.must(t, "init")
	cx, _ := settings.Load(filepath.Join(home, ".codex", "hooks.json"))
	gm, _ := settings.Load(filepath.Join(home, ".gemini", "settings.json"))
	if got := cx.HookCommands("SessionStart"); len(got) != 1 || !strings.HasSuffix(got[0], "hook session-start --agent codex") {
		t.Fatalf("codex hooks = %q", got)
	}
	if got := gm.HookCommands("AfterAgent"); len(got) != 1 || !strings.HasSuffix(got[0], "hook stop --agent gemini") {
		t.Fatalf("gemini hooks = %q", got)
	}
	if !strings.Contains(out, "Codex") || !strings.Contains(out, "Gemini") {
		t.Fatalf("out = %q", out)
	}
	h.must(t, "init", "--remove")
	cx, _ = settings.Load(filepath.Join(home, ".codex", "hooks.json"))
	gm, _ = settings.Load(filepath.Join(home, ".gemini", "settings.json"))
	if len(cx.HookCommands("SessionStart"))+len(gm.HookCommands("SessionEnd")) != 0 {
		t.Fatal("init --remove left agent hooks behind")
	}
	if b, _ := os.ReadFile(filepath.Join(home, ".gemini", "settings.json")); !strings.Contains(string(b), `"theme"`) {
		t.Fatal("other Gemini settings were lost")
	}
}

func TestResumeUsesTheSessionsAgent(t *testing.T) {
	h := testApp(t, term.Exact)
	h.app.Chdir = func(string) error { return nil }
	h.app.DirExists = func(string) bool { return true }
	if err := h.app.resumeSession(t.Context(), "cx-9", "/w/api", "codex"); err != nil {
		t.Fatal(err)
	}
	if len(h.execd) != 1 || strings.Join(h.execd[0], " ") != "codex resume cx-9" {
		t.Fatalf("exec = %q", h.execd)
	}
}
