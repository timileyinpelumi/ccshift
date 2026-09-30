package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/timileyinpelumi/ccshift/internal/settings"
	"github.com/timileyinpelumi/ccshift/internal/term"
)

const userSettings = `{
  "model": "opus",
  "hooks": {
    "Stop": [{"hooks": [{"type": "command", "command": "their-stop-hook"}]}]
  },
  "statusLine": {"type": "command", "command": "node 'x.js'", "padding": 0},
  "theme": "dark"
}`

func initApp(t *testing.T, settingsJSON string) (*harness, string) {
	t.Helper()
	h := testApp(t, term.Exact)
	h.app.ClaudeDir = t.TempDir()
	h.app.Executable = "/opt/my tools/ccshift"
	p := filepath.Join(h.app.ClaudeDir, "settings.json")
	if settingsJSON != "" {
		os.WriteFile(p, []byte(settingsJSON), 0o600)
	}
	return h, p
}

func canonJSON(t *testing.T, b []byte) string {
	t.Helper()
	var buf bytes.Buffer
	if err := json.Indent(&buf, b, "", "  "); err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(buf.String())
}

func TestInitInstallsHooksAndWrapsStatusline(t *testing.T) {
	h, p := initApp(t, userSettings)
	h.must(t, "init")
	f, _ := settings.Load(p)
	want := map[string]string{
		"SessionStart": "'/opt/my tools/ccshift' hook session-start",
		"Stop":         "'/opt/my tools/ccshift' hook stop",
		"SessionEnd":   "'/opt/my tools/ccshift' hook session-end",
	}
	for ev, cmd := range want {
		got := f.HookCommands(ev)
		if len(got) == 0 || got[len(got)-1] != cmd {
			t.Errorf("%s hooks = %q", ev, got)
		}
	}
	if got := f.HookCommands("Stop"); got[0] != "their-stop-hook" {
		t.Fatalf("their hook was lost: %q", got)
	}
	var sl map[string]any
	json.Unmarshal(f.StatusLine(), &sl)
	if sl["command"] != "'/opt/my tools/ccshift' statusline" || sl["padding"] != float64(0) {
		t.Fatalf("statusLine = %v", sl)
	}
	if b, _ := os.ReadFile(p + ".ccshift-bak"); string(b) != userSettings {
		t.Fatalf("backup = %q", b)
	}
	st, _ := h.app.Store.InitState()
	if !strings.Contains(string(st.PreviousStatusLine), "x.js") {
		t.Fatalf("previous statusline not kept: %s", st.PreviousStatusLine)
	}

	before, _ := os.ReadFile(p)
	out := h.must(t, "init")
	after, _ := os.ReadFile(p)
	if string(before) != string(after) || !strings.Contains(out, "already set up") {
		t.Fatalf("second init changed the file or didn't say so: %q", out)
	}
	if st, _ = h.app.Store.InitState(); !strings.Contains(string(st.PreviousStatusLine), "x.js") {
		t.Fatal("second init overwrote the saved statusline with its own")
	}
}

func TestInitRemoveRestoresSettings(t *testing.T) {
	for name, orig := range map[string]string{"with statusline": userSettings, "bare": `{"theme": "dark"}`} {
		h, p := initApp(t, orig)
		h.must(t, "init")
		h.must(t, "init", "--remove")
		b, _ := os.ReadFile(p)
		if canonJSON(t, b) != canonJSON(t, []byte(orig)) {
			t.Errorf("%s: not restored:\n%s", name, b)
		}
	}
}

func TestInitUpdatesAMovedBinary(t *testing.T) {
	h, p := initApp(t, userSettings)
	h.must(t, "init")
	h.app.Executable = "/usr/local/bin/ccshift"
	h.must(t, "init")
	f, _ := settings.Load(p)
	if got := f.HookCommands("SessionEnd"); len(got) != 1 || got[0] != "/usr/local/bin/ccshift hook session-end" {
		t.Fatalf("SessionEnd hooks = %q", got)
	}
	if st, _ := h.app.Store.InitState(); !strings.Contains(string(st.PreviousStatusLine), "x.js") {
		t.Fatal("saved statusline lost")
	}
}

func TestInitWithNoSettingsFile(t *testing.T) {
	h, p := initApp(t, "")
	h.must(t, "init")
	f, _ := settings.Load(p)
	if len(f.HookCommands("Stop")) != 1 || f.StatusLine() == nil {
		t.Fatal("settings not created")
	}
}

func TestInitRefusesBrokenSettings(t *testing.T) {
	h, p := initApp(t, `{"hooks": `)
	if code := h.run("init"); code != 1 {
		t.Fatalf("exit = %d", code)
	}
	if b, _ := os.ReadFile(p); string(b) != `{"hooks": ` {
		t.Fatal("a file that can't be parsed must be left alone")
	}
}

func TestInitImportsLegacySaves(t *testing.T) {
	h, _ := initApp(t, userSettings)
	dir := filepath.Join(h.app.ClaudeDir, "saved-sessions")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "20260913-134427.json"), []byte(`{"id":"20260913-134427","label":"","saved_at":1789300000000,"sessions":[{"session_id":"old1","cwd":"/w","name":"w-1a"}]}`), 0o644)
	os.WriteFile(filepath.Join(dir, "20260915-234309.json"), []byte(`{"id":"20260915-234309","label":"godmode","saved_at":1789512189355,"sessions":[{"session_id":"a","cwd":"/w","name":"PAY 2193"},{"session_id":"b","cwd":"/w","name":"w-41"}]}`), 0o644)
	os.WriteFile(filepath.Join(dir, "latest"), []byte("20260915-234309"), 0o644)
	os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{"), 0o644)
	out := h.must(t, "init")
	if !strings.Contains(out, "Imported 2 saves") {
		t.Fatalf("out = %q", out)
	}
	hist, _ := h.app.Store.History("default")
	if len(hist) != 2 || len(hist[0].Sessions) != 2 || hist[0].Sessions[0].Name != "PAY 2193" || hist[0].Sessions[1].Position != 2 {
		t.Fatalf("history = %+v", hist)
	}
	if _, ok, _ := h.app.Store.Latest("default"); ok {
		t.Fatal("imported saves belong in history only")
	}
	h.must(t, "init")
	if hist, _ = h.app.Store.History("default"); len(hist) != 2 {
		t.Fatalf("imported twice: %d", len(hist))
	}
}

func TestInitImportLeavesAnExistingSaveAsLatest(t *testing.T) {
	h, _ := initApp(t, userSettings)
	saveSnap(t, h, "default", "mine")
	dir := filepath.Join(h.app.ClaudeDir, "saved-sessions")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "20260915-234309.json"), []byte(`{"saved_at":1789512189355,"sessions":[{"session_id":"a","cwd":"/w"}]}`), 0o644)
	h.must(t, "init")
	if got := latestIDs(t, h, "default"); len(got) != 1 || got[0] != "mine" {
		t.Fatalf("latest = %v", got)
	}
}

type checkedTerm struct {
	*fakeTerm
	err error
}

func (c checkedTerm) Check(context.Context) error { return c.err }

func TestInitAndDoctorReportTerminalRequirements(t *testing.T) {
	h, _ := initApp(t, userSettings)
	h.app.Terms = []term.Adapter{checkedTerm{h.term, errors.New("add allow_remote_control to kitty.conf")}}
	if out := h.must(t, "init"); !strings.Contains(out, "add allow_remote_control to kitty.conf") {
		t.Fatalf("out = %q", out)
	}
	if code := h.run("doctor"); code != 1 || !strings.Contains(h.out.String(), "add allow_remote_control") {
		t.Fatalf("doctor exit %d: %s", code, h.out)
	}
}

func TestDoctor(t *testing.T) {
	h, _ := initApp(t, userSettings)
	self, _ := os.Executable()
	h.app.Executable = self
	if code := h.run("doctor"); code != 1 || !strings.Contains(h.out.String(), "ccshift init") {
		t.Fatalf("before init: exit %d\n%s", code, h.out)
	}
	h.must(t, "init")
	out := h.must(t, "doctor")
	if strings.Contains(out, "problem") {
		t.Fatalf("after init:\n%s", out)
	}
	h.app.Executable = "/gone/ccshift"
	if code := h.run("doctor"); code != 1 {
		t.Fatalf("missing binary should be a problem:\n%s", h.out)
	}
}
