package term

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

func TestTmuxListAndTabOf(t *testing.T) {
	f := &fakeExec{outputs: map[string]string{
		"tmux list-panes": "work\t0\t0\t%0\nwork\t1\t0\t%4\npersonal\t0\t0\t%2\n",
	}}
	a := ByName(All(f.exec()), "tmux")
	tabs, err := a.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []Tab{{"%0", "work", "work:0"}, {"%4", "work", "work:1"}, {"%2", "personal", "personal:0"}}
	if !reflect.DeepEqual(tabs, want) {
		t.Fatalf("tabs = %v", tabs)
	}
	if id, ok := a.TabOf(map[string]string{"TMUX_PANE": "%4"}); !ok || id != "%4" {
		t.Fatalf("TabOf = %q %v", id, ok)
	}
	if !a.Detect(map[string]string{"TMUX": "/tmp/tmux-1000/default,1,0"}, nil) {
		t.Fatal("Detect should match TMUX")
	}
}

func TestTmuxIntegration(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	sock := fmt.Sprintf("ccshift-test-%d", os.Getpid())
	t.Cleanup(func() { exec.Command("tmux", "-L", sock, "kill-server").Run() })
	x := SystemExec()
	x.Env = map[string]string{}
	x.Out = io.Discard
	tm := &tmux{x: x, socket: sock}
	ctx := context.Background()

	err := tm.OpenWindow(ctx, "work.main", []Launch{
		{CWD: t.TempDir(), Title: "one", Argv: []string{"sleep", "60"}},
		{CWD: t.TempDir(), Title: "two · x", Argv: []string{"sleep", "60"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	tabs, err := tm.List(ctx)
	if err != nil || len(tabs) != 2 || tabs[0].Window != "work-main" {
		t.Fatalf("tabs = %v, %v", tabs, err)
	}
	if err := tm.SetTitle(ctx, tabs[1].ID, "-renamed"); err != nil {
		t.Fatal(err)
	}
	out, _ := exec.Command("tmux", "-L", sock, "list-windows", "-a", "-F", "#{window_name}").Output()
	if got := strings.Fields(string(out)); !reflect.DeepEqual(got, []string{"one", "-renamed"}) {
		t.Fatalf("window names = %q", got)
	}

	if err := tm.OpenWindow(ctx, "work.main", []Launch{{CWD: t.TempDir(), Title: "three", Argv: []string{"sleep", "60"}}}); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("tmux", "-L", sock, "has-session", "-t", "=work-main-2").Run(); err == nil {
		t.Fatal("a second restore should add to work-main, not create work-main-2")
	}
	if tabs, _ = tm.List(ctx); len(tabs) != 3 || tabs[2].Window != "work-main" {
		t.Fatalf("tabs after second restore = %v", tabs)
	}
	if tabs[0].Label != "work-main:0" || tabs[2].Label != "work-main:2" {
		t.Fatalf("labels = %q %q", tabs[0].Label, tabs[2].Label)
	}
}

func TestTmuxOpenAndCloseTab(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	sock := fmt.Sprintf("ccshift-tab-%d", os.Getpid())
	t.Cleanup(func() { exec.Command("tmux", "-L", sock, "kill-server").Run() })
	x := SystemExec()
	x.Env = map[string]string{}
	x.Out = io.Discard
	tm := &tmux{x: x, socket: sock}
	ctx := context.Background()
	tm.OpenWindow(ctx, "w", []Launch{
		{CWD: t.TempDir(), Title: "one", Argv: []string{"sleep", "60"}},
		{CWD: t.TempDir(), Title: "two", Argv: []string{"sleep", "60"}},
	})
	tabs, _ := tm.List(ctx)
	if err := tm.OpenTab(ctx, tabs[0].ID, Launch{CWD: t.TempDir(), Title: "one (2)", Argv: []string{"sleep", "60"}}); err != nil {
		t.Fatal(err)
	}
	names := func() string {
		out, _ := exec.Command("tmux", "-L", sock, "list-windows", "-a", "-F", "#{window_name}").Output()
		return strings.Join(strings.Split(strings.TrimSpace(string(out)), "\n"), ",")
	}
	if got := names(); got != "one,one (2),two" {
		t.Fatalf("after OpenTab: %s", got)
	}
	// A window with another pane in it keeps that pane.
	exec.Command("tmux", "-L", sock, "split-window", "-t", string(tabs[1].ID), "sleep", "60").Run()
	if err := tm.CloseTab(ctx, tabs[1].ID); err != nil {
		t.Fatal(err)
	}
	if got := names(); got != "one,one (2),two" {
		t.Fatalf("closing one pane took the whole window: %s", got)
	}
	if err := tm.CloseTab(ctx, tabs[0].ID); err != nil {
		t.Fatal(err)
	}
	if got := names(); got != "one (2),two" {
		t.Fatalf("after CloseTab: %s", got)
	}
}
