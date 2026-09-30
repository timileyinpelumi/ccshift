package term

import (
	"context"
	"os"
	"reflect"
	"testing"
)

func TestWeztermList(t *testing.T) {
	b, err := os.ReadFile("testdata/wezterm-list.json")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeExec{outputs: map[string]string{"wezterm cli list": string(b)}}
	a := ByName(All(f.exec()), "wezterm")
	tabs, err := a.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []Tab{{"0", "0", "1.1"}, {"3", "2", "2.1"}, {"4", "2", "2.2"}}
	if !reflect.DeepEqual(tabs, want) {
		t.Fatalf("tabs = %v", tabs)
	}
	if id, ok := a.TabOf(map[string]string{"WEZTERM_PANE": "3"}); !ok || id != "3" {
		t.Fatalf("TabOf = %q %v", id, ok)
	}
}

func TestWeztermOpenWindow(t *testing.T) {
	f := &fakeExec{outputs: map[string]string{
		"wezterm cli spawn --new-window": "9\n",
		"wezterm cli spawn --window-id":  "10\n",
		"wezterm cli list":               `[{"window_id": 4, "tab_id": 7, "pane_id": 9}]`,
	}}
	a := ByName(All(f.exec()), "wezterm")
	if err := a.OpenWindow(context.Background(), "work", twoLaunches); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"wezterm", "cli", "spawn", "--new-window", "--cwd", "/home/u/dev/my app", "--", "/usr/bin/claude", "--resume", "id-1", "-n", "acme · PAY-2193"},
		{"wezterm", "cli", "set-tab-title", "--pane-id", "9", "--", "acme · PAY-2193"},
		{"wezterm", "cli", "list", "--format", "json"},
		{"wezterm", "cli", "spawn", "--window-id", "4", "--cwd", "/home/u/dev/cvx", "--", "/usr/bin/claude", "--resume", "id-2", "-n", "it's cvx"},
		{"wezterm", "cli", "set-tab-title", "--pane-id", "10", "--", "it's cvx"},
	}
	if !reflect.DeepEqual(f.calls, want) {
		t.Fatalf("calls:\n%q\nwant\n%q", f.calls, want)
	}
}

func TestWeztermFocusSwitchesTabThenPane(t *testing.T) {
	b, _ := os.ReadFile("testdata/wezterm-list.json")
	f := &fakeExec{outputs: map[string]string{"wezterm cli list": string(b)}}
	a := ByName(All(f.exec()), "wezterm")
	if err := a.Focus(context.Background(), "4"); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"wezterm", "cli", "list", "--format", "json"},
		{"wezterm", "cli", "activate-tab", "--tab-id", "4"},
		{"wezterm", "cli", "activate-pane", "--pane-id", "4"},
	}
	if !reflect.DeepEqual(f.calls, want) {
		t.Fatalf("calls = %q", f.calls)
	}
	if err := a.Focus(context.Background(), "99"); err == nil {
		t.Fatal("unknown pane should fail")
	}
}
