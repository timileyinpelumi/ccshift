package term

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func withOS(t *testing.T, name string) {
	t.Helper()
	old := goos
	goos = name
	t.Cleanup(func() { goos = old })
}

func lastScript(f *fakeExec) string {
	c := f.calls[len(f.calls)-1]
	if c[0] != "osascript" || c[1] != "-e" {
		return ""
	}
	return c[2]
}

func TestITermListAndTabOf(t *testing.T) {
	f := &fakeExec{outputs: map[string]string{"osascript": "101\t1\tAAAA-1\n101\t2\tBBBB-2\n101\t2\tCCCC-3\n205\t1\tDDDD-4\n"}}
	a := ByName(All(f.exec()), "iterm2")
	if a.Tier() != Exact || !a.Detect(map[string]string{"TERM_PROGRAM": "iTerm.app"}, nil) {
		t.Fatal("iTerm2 should be an exact adapter detected by TERM_PROGRAM")
	}
	tabs, err := a.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []Tab{{"AAAA-1", "101", "1.1"}, {"BBBB-2", "101", "1.2"}, {"CCCC-3", "101", "1.2"}, {"DDDD-4", "205", "2.1"}}
	if !reflect.DeepEqual(tabs, want) {
		t.Fatalf("tabs = %v", tabs)
	}
	if id, ok := a.TabOf(map[string]string{"ITERM_SESSION_ID": "w0t1p0:BBBB-2"}); !ok || id != "BBBB-2" {
		t.Fatalf("TabOf = %q %v", id, ok)
	}
}

func TestITermOpenWindowScript(t *testing.T) {
	f := &fakeExec{}
	a := ByName(All(f.exec()), "iterm2")
	if err := a.OpenWindow(context.Background(), "work", twoLaunches); err != nil {
		t.Fatal(err)
	}
	s := lastScript(f)
	for _, want := range []string{
		`create window with default profile command`,
		`create tab with default profile command`,
		`command "/bin/sh -c 'cd `,
		`&& exec /usr/bin/claude --resume id-1 -n `,
		`set name to "it's cvx"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("script missing %q:\n%s", want, s)
		}
	}
	a.SetTitle(context.Background(), "BBBB-2", `say "hi" \ there`)
	if s := lastScript(f); !strings.Contains(s, `"say \"hi\" \\ there"`) || !strings.Contains(s, `"BBBB-2"`) {
		t.Fatalf("title script not escaped:\n%s", s)
	}
}

func TestAppleTerminalMatchesByTTY(t *testing.T) {
	f := &fakeExec{outputs: map[string]string{"osascript": "11\t/dev/ttys002\n11\t/dev/ttys005\n12\t/dev/ttys001\n"}}
	a := ByName(All(f.exec()), "terminal")
	if !a.Detect(map[string]string{"TERM_PROGRAM": "Apple_Terminal"}, nil) || a.Tier() != Exact {
		t.Fatal("Terminal.app should be an exact adapter detected by TERM_PROGRAM")
	}
	tabs, _ := a.List(context.Background())
	want := []Tab{{"/dev/ttys002", "11", "1.1"}, {"/dev/ttys005", "11", "1.2"}, {"/dev/ttys001", "12", "2.1"}}
	if !reflect.DeepEqual(tabs, want) {
		t.Fatalf("tabs = %v", tabs)
	}
	if id, ok := a.TabOf(map[string]string{TTYKey: "/dev/ttys005"}); !ok || id != "/dev/ttys005" {
		t.Fatalf("TabOf = %q %v", id, ok)
	}
	if _, ok := a.TabOf(map[string]string{}); ok {
		t.Fatal("no tty, no tab")
	}
	a.OpenWindow(context.Background(), "work", twoLaunches[:1])
	if s := lastScript(f); !strings.Contains(s, `do script "cd '/home/u/dev/my app' && exec /usr/bin/claude --resume id-1 -n 'acme · PAY-2193'"`) {
		t.Fatalf("script:\n%s", s)
	}
}

func TestWindowsTerminalOpensAllTabsInOneCommand(t *testing.T) {
	f := &fakeExec{}
	a := ByName(All(f.exec()), "windows-terminal")
	if !a.Detect(map[string]string{"WT_SESSION": "abc"}, nil) {
		t.Fatal("Windows Terminal is detected by WT_SESSION")
	}
	ls := []Launch{
		{CWD: `C:\dev\my app`, Title: "one", Argv: []string{"claude", "--resume", "a;b"}},
		{CWD: `C:\dev\two`, Title: "two", Argv: []string{"claude", "--resume", "c"}},
	}
	if err := a.OpenWindow(context.Background(), "work", ls); err != nil {
		t.Fatal(err)
	}
	want := []string{"wt.exe", "-w", "new",
		"new-tab", "-d", `C:\dev\my app`, "--title", "one", "--suppressApplicationTitle", "claude", "--resume", `a\;b`, ";",
		"new-tab", "-d", `C:\dev\two`, "--title", "two", "--suppressApplicationTitle", "claude", "--resume", "c"}
	if !reflect.DeepEqual(f.calls[0], want) {
		t.Fatalf("call = %q", f.calls[0])
	}
	f.calls = nil
	a.(TabOpener).OpenTab(context.Background(), "", ls[1])
	if got := f.calls[0][:4]; !reflect.DeepEqual(got, []string{"wt.exe", "-w", "0", "new-tab"}) {
		t.Fatalf("OpenTab = %q", f.calls[0])
	}
}

func TestMacLaunchersGoThroughOpen(t *testing.T) {
	withOS(t, "darwin")
	f := &fakeExec{}
	a := ByName(All(f.exec()), "ghostty")
	a.OpenWindow(context.Background(), "w", twoLaunches[:1])
	want := []string{"open", "-na", "Ghostty", "--args", "--working-directory=/home/u/dev/my app", "--title=acme · PAY-2193", "-e", "/usr/bin/claude", "--resume", "id-1", "-n", "acme · PAY-2193"}
	if !reflect.DeepEqual(f.calls[0], want) {
		t.Fatalf("call = %q", f.calls[0])
	}
}

func TestGenericOnWindowsUsesStart(t *testing.T) {
	withOS(t, "windows")
	f := &fakeExec{env: map[string]string{}}
	g := ByName(All(f.exec()), "generic")
	if err := g.OpenWindow(context.Background(), "w", []Launch{{CWD: `C:\dev`, Title: "one", Argv: []string{"claude", "--resume", "a"}}}); err != nil {
		t.Fatal(err)
	}
	want := []string{"cmd", "/c", "start", "one", "/D", `C:\dev`, "claude", "--resume", "a"}
	if !reflect.DeepEqual(f.calls[0], want) {
		t.Fatalf("call = %q", f.calls[0])
	}
}
