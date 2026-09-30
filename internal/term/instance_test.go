package term

import (
	"reflect"
	"strings"
	"testing"
)

func TestTabOfIgnoresOtherInstances(t *testing.T) {
	cases := []struct {
		name  string
		own   map[string]string
		same  map[string]string
		other map[string]string
	}{
		{"tmux",
			map[string]string{"TMUX": "/tmp/tmux-1000/default,100,0"},
			map[string]string{"TMUX": "/tmp/tmux-1000/default,100,3", "TMUX_PANE": "%1"},
			map[string]string{"TMUX": "/tmp/tmux-1000/other,200,0", "TMUX_PANE": "%1"}},
		{"kitty",
			map[string]string{"KITTY_PID": "10", "KITTY_WINDOW_ID": "5"},
			map[string]string{"KITTY_PID": "10", "KITTY_WINDOW_ID": "1"},
			map[string]string{"KITTY_PID": "11", "KITTY_WINDOW_ID": "1"}},
		{"wezterm",
			map[string]string{"WEZTERM_UNIX_SOCKET": "/run/a", "WEZTERM_PANE": "5"},
			map[string]string{"WEZTERM_UNIX_SOCKET": "/run/a", "WEZTERM_PANE": "1"},
			map[string]string{"WEZTERM_UNIX_SOCKET": "/run/b", "WEZTERM_PANE": "1"}},
		{"konsole",
			map[string]string{"KONSOLE_DBUS_SERVICE": ":1.0"},
			map[string]string{"KONSOLE_DBUS_SERVICE": ":1.0", "KONSOLE_DBUS_SESSION": "/Sessions/2"},
			map[string]string{"KONSOLE_DBUS_SERVICE": ":1.7", "KONSOLE_DBUS_SESSION": "/Sessions/2"}},
		{"zellij",
			map[string]string{"ZELLIJ": "0", "ZELLIJ_SESSION_NAME": "a"},
			map[string]string{"ZELLIJ_SESSION_NAME": "a", "ZELLIJ_PANE_ID": "1"},
			map[string]string{"ZELLIJ_SESSION_NAME": "b", "ZELLIJ_PANE_ID": "1"}},
	}
	for _, c := range cases {
		f := &fakeExec{env: c.own}
		a := ByName(All(f.exec()), c.name)
		if _, ok := a.TabOf(c.same); !ok {
			t.Errorf("%s: session in the same instance should match", c.name)
		}
		if _, ok := a.TabOf(c.other); ok {
			t.Errorf("%s: session in another instance should not match", c.name)
		}
		// Run from outside the terminal: nothing to compare against, so accept.
		outside := ByName(All((&fakeExec{env: map[string]string{}}).exec()), c.name)
		if _, ok := outside.TabOf(c.other); !ok {
			t.Errorf("%s: from outside, sessions should match", c.name)
		}
	}
}

func TestRequirementChecks(t *testing.T) {
	f := &fakeExec{fail: map[string]string{"kitten @ ls": "remote control is disabled"}}
	k := ByName(All(f.exec()), "kitty").(Checker)
	err := k.Check(t.Context())
	if err == nil || !strings.Contains(err.Error(), "allow_remote_control") || !strings.Contains(err.Error(), "listen_on") {
		t.Fatalf("kitty check = %v", err)
	}
	if err := ByName(All((&fakeExec{}).exec()), "kitty").(Checker).Check(t.Context()); err != nil {
		t.Fatalf("working kitty: %v", err)
	}
	for version, ok := range map[string]bool{"zellij 0.45.1": true, "zellij 0.44.0": true, "zellij 0.43.1": false, "zellij 1.0.0": true, "garbage": false} {
		f := &fakeExec{outputs: map[string]string{"zellij --version": version + "\n"}}
		err := ByName(All(f.exec()), "zellij").(Checker).Check(t.Context())
		if (err == nil) != ok {
			t.Errorf("%s: err = %v", version, err)
		}
	}
}

func TestKittyAndWeztermTabCommands(t *testing.T) {
	l := Launch{CWD: "/w", Title: "api (2)", Argv: []string{"claude", "--resume", "x"}}
	f := &fakeExec{}
	k := ByName(All(f.exec()), "kitty")
	k.(TabOpener).OpenTab(t.Context(), "7", l)
	k.(TabCloser).CloseTab(t.Context(), "7")
	want := [][]string{
		{"kitten", "@", "launch", "--type=tab", "--match=window_id:7", "--cwd=/w", "--tab-title=api (2)", "--", "claude", "--resume", "x"},
		{"kitten", "@", "close-window", "--match=id:7"},
	}
	if !reflect.DeepEqual(f.calls, want) {
		t.Fatalf("kitty calls = %q", f.calls)
	}
	f = &fakeExec{outputs: map[string]string{"wezterm cli spawn": "12\n"}}
	w := ByName(All(f.exec()), "wezterm")
	w.(TabOpener).OpenTab(t.Context(), "3", l)
	w.(TabCloser).CloseTab(t.Context(), "3")
	want = [][]string{
		{"wezterm", "cli", "spawn", "--pane-id", "3", "--cwd", "/w", "--", "claude", "--resume", "x"},
		{"wezterm", "cli", "set-tab-title", "--pane-id", "12", "--", "api (2)"},
		{"wezterm", "cli", "kill-pane", "--pane-id", "3"},
	}
	if !reflect.DeepEqual(f.calls, want) {
		t.Fatalf("wezterm calls = %q", f.calls)
	}
	if _, ok := ByName(All((&fakeExec{}).exec()), "zellij").(TabOpener); !ok {
		t.Fatal("zellij should open tabs")
	}
	if _, ok := ByName(All((&fakeExec{}).exec()), "gnome-terminal").(TabOpener); ok {
		t.Fatal("gnome-terminal cannot place a tab")
	}
}

func TestWeztermOpenTabSucceedsEvenIfTheTitleFails(t *testing.T) {
	f := &fakeExec{outputs: map[string]string{"wezterm cli spawn": "12\n"}, fail: map[string]string{"wezterm cli set-tab-title": "no such pane"}}
	w := ByName(All(f.exec()), "wezterm")
	l := Launch{CWD: "/w", Title: "t", Argv: []string{"claude"}}
	if err := w.(TabOpener).OpenTab(t.Context(), "3", l); err != nil {
		t.Fatalf("the session is running; a failed title is not a failed open: %v", err)
	}
	f.outputs["wezterm cli list"] = `[{"window_id": 4, "tab_id": 7, "pane_id": 12}]`
	if err := w.OpenWindow(t.Context(), "ws", []Launch{l}); err != nil {
		t.Fatalf("OpenWindow: %v", err)
	}
}
