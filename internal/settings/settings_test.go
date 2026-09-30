package settings

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const original = `{
  "model": "opus",
  "hooks": {
    "Stop": [
      {"hooks": [{"type": "command", "command": "echo a && echo <b>"}]}
    ],
    "PreToolUse": [
      {"matcher": "Read", "hooks": [{"type": "command", "command": "guard"}]}
    ]
  },
  "statusLine": {"type": "command", "command": "node 'x.js'", "padding": 0},
  "theme": "dark"
}`

func write(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func canon(t *testing.T, b []byte) string {
	t.Helper()
	var buf bytes.Buffer
	if err := json.Indent(&buf, b, "", "  "); err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(buf.String())
}

func isOurs(cmd string) bool { return strings.Contains(cmd, "ccshift hook ") }

func TestAddHooksKeepsEverythingElseInOrder(t *testing.T) {
	f, err := Load(write(t, original))
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range []string{"SessionStart", "Stop", "SessionEnd"} {
		added, err := f.AddHook(ev, "/bin/ccshift hook "+strings.ToLower(ev), 5)
		if err != nil || !added {
			t.Fatalf("%s: %v %v", ev, added, err)
		}
	}
	if added, _ := f.AddHook("Stop", "/bin/ccshift hook stop", 5); added {
		t.Fatal("adding the same hook twice should do nothing")
	}
	b, _ := f.Bytes()
	s := string(b)
	for _, want := range []string{`"model"`, `"hooks"`, `"statusLine"`, `"theme"`} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %s in\n%s", want, s)
		}
	}
	if !(strings.Index(s, `"model"`) < strings.Index(s, `"hooks"`) && strings.Index(s, `"hooks"`) < strings.Index(s, `"statusLine"`) && strings.Index(s, `"statusLine"`) < strings.Index(s, `"theme"`)) {
		t.Fatalf("top-level key order changed:\n%s", s)
	}
	if strings.Index(s, `"Stop"`) > strings.Index(s, `"PreToolUse"`) {
		t.Fatalf("hook event order changed:\n%s", s)
	}
	if !strings.Contains(s, "echo a && echo <b>") {
		t.Fatalf("existing command was re-escaped:\n%s", s)
	}
	if got := f.HookCommands("Stop"); len(got) != 2 || got[1] != "/bin/ccshift hook stop" {
		t.Fatalf("Stop commands = %q", got)
	}
	if got := f.HookCommands("SessionEnd"); len(got) != 1 {
		t.Fatalf("SessionEnd commands = %q", got)
	}
}

func TestRemoveHooksRestoresTheOriginal(t *testing.T) {
	f, _ := Load(write(t, original))
	for _, ev := range []string{"SessionStart", "Stop", "SessionEnd"} {
		f.AddHook(ev, "/bin/ccshift hook x", 5)
	}
	n, err := f.RemoveHooks(isOurs)
	if err != nil || n != 3 {
		t.Fatalf("removed %d, %v", n, err)
	}
	b, _ := f.Bytes()
	if canon(t, b) != canon(t, []byte(original)) {
		t.Fatalf("not restored:\n%s", b)
	}
}

func TestHooksKeyIsCreatedAndRemoved(t *testing.T) {
	f, _ := Load(write(t, `{"theme": "dark"}`))
	f.AddHook("Stop", "/bin/ccshift hook stop", 5)
	if len(f.HookCommands("Stop")) != 1 {
		t.Fatal("hook not added")
	}
	f.RemoveHooks(isOurs)
	b, _ := f.Bytes()
	if canon(t, b) != canon(t, []byte(`{"theme": "dark"}`)) {
		t.Fatalf("got %s", b)
	}
}

func TestStatusLine(t *testing.T) {
	f, _ := Load(write(t, original))
	if !strings.Contains(string(f.StatusLine()), "x.js") {
		t.Fatalf("statusLine = %s", f.StatusLine())
	}
	f.SetStatusLine(json.RawMessage(`{"type":"command","command":"ccshift statusline","padding":0}`))
	b, _ := f.Bytes()
	if !strings.Contains(string(b), "ccshift statusline") || strings.Index(string(b), `"statusLine"`) > strings.Index(string(b), `"theme"`) {
		t.Fatalf("got %s", b)
	}
	f.SetStatusLine(nil)
	if f.StatusLine() != nil {
		t.Fatal("statusLine should be gone")
	}
}

func TestMissingFileSaveAndBackup(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	f, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Backup(".bak"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p + ".bak"); err == nil {
		t.Fatal("nothing to back up for a missing file")
	}
	f.AddHook("Stop", "c", 5)
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	g, _ := Load(p)
	if len(g.HookCommands("Stop")) != 1 {
		t.Fatal("saved file does not round-trip")
	}
	if err := g.Backup(".bak"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p + ".bak"); !strings.Contains(string(b), `"Stop"`) {
		t.Fatalf("backup = %q", b)
	}
}

func TestLoadRejectsBrokenJSON(t *testing.T) {
	if _, err := Load(write(t, `{"a": `)); err == nil {
		t.Fatal("expected an error")
	}
	if _, err := Load(write(t, `[1,2]`)); err == nil {
		t.Fatal("settings must be an object")
	}
}
