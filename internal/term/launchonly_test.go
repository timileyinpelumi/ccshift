package term

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
)

var twoLaunches = []Launch{
	{CWD: "/home/u/dev/my app", Title: "acme · PAY-2193", Argv: []string{"/usr/bin/claude", "--resume", "id-1", "-n", "acme · PAY-2193"}},
	{CWD: "/home/u/dev/cvx", Title: "it's cvx", Argv: []string{"/usr/bin/claude", "--resume", "id-2", "-n", "it's cvx"}},
}

func openWith(t *testing.T, name string) *fakeExec {
	t.Helper()
	f := &fakeExec{}
	a := ByName(All(f.exec()), name)
	if a == nil {
		t.Fatalf("no adapter %q", name)
	}
	if err := a.OpenWindow(context.Background(), "work", twoLaunches); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestLaunchOnlyCommands(t *testing.T) {
	q1 := `/usr/bin/claude --resume id-1 -n 'acme · PAY-2193'`
	q2 := `/usr/bin/claude --resume id-2 -n 'it'\''s cvx'`
	cases := map[string][][]string{
		"gnome-terminal": {
			{"gnome-terminal", "--window", "--working-directory=/home/u/dev/my app", "--", "/usr/bin/claude", "--resume", "id-1", "-n", "acme · PAY-2193"},
			{"gnome-terminal", "--tab", "--working-directory=/home/u/dev/cvx", "--", "/usr/bin/claude", "--resume", "id-2", "-n", "it's cvx"},
		},
		"ptyxis": {
			{"ptyxis", "--new-window", "-d", "/home/u/dev/my app", "-x", q1},
			{"ptyxis", "--tab", "-d", "/home/u/dev/cvx", "-x", q2},
		},

		"tilix": {
			{"tilix", "--working-directory=/home/u/dev/my app", "--command=" + q1},
			{"tilix", "--action=app-new-session", "--working-directory=/home/u/dev/cvx", "--command=" + q2},
		},
		"xfce4-terminal": {
			{"xfce4-terminal",
				"--working-directory=/home/u/dev/my app", "--title=acme · PAY-2193", "--command=" + q1,
				"--tab", "--working-directory=/home/u/dev/cvx", "--title=it's cvx", "--command=" + q2},
		},
		"ghostty": {
			{"ghostty", "--working-directory=/home/u/dev/my app", "--title=acme · PAY-2193", "-e", "/usr/bin/claude", "--resume", "id-1", "-n", "acme · PAY-2193"},
			{"ghostty", "--working-directory=/home/u/dev/cvx", "--title=it's cvx", "-e", "/usr/bin/claude", "--resume", "id-2", "-n", "it's cvx"},
		},
		"alacritty": {
			{"alacritty", "--working-directory=/home/u/dev/my app", "--title=acme · PAY-2193", "-e", "/usr/bin/claude", "--resume", "id-1", "-n", "acme · PAY-2193"},
			{"alacritty", "--working-directory=/home/u/dev/cvx", "--title=it's cvx", "-e", "/usr/bin/claude", "--resume", "id-2", "-n", "it's cvx"},
		},
		"foot": {
			{"foot", "--working-directory=/home/u/dev/my app", "--title=acme · PAY-2193", "/usr/bin/claude", "--resume", "id-1", "-n", "acme · PAY-2193"},
			{"foot", "--working-directory=/home/u/dev/cvx", "--title=it's cvx", "/usr/bin/claude", "--resume", "id-2", "-n", "it's cvx"},
		},
	}
	for name, want := range cases {
		f := openWith(t, name)
		if !reflect.DeepEqual(f.calls, want) {
			t.Errorf("%s:\n got  %q\n want %q", name, f.calls, want)
		}
	}
}

func TestLaunchOnlyHasNoTabAPI(t *testing.T) {
	a := ByName(All((&fakeExec{}).exec()), "gnome-terminal")
	if a.Tier() != LaunchOnly {
		t.Fatalf("tier = %v", a.Tier())
	}
	if _, err := a.List(context.Background()); err != ErrUnsupported {
		t.Fatalf("List err = %v", err)
	}
	if err := a.Focus(context.Background(), "1"); err != ErrUnsupported {
		t.Fatalf("Focus err = %v", err)
	}
}

func TestDetect(t *testing.T) {
	as := All((&fakeExec{}).exec())
	cases := []struct {
		env       map[string]string
		ancestors []string
		want      string
	}{
		{map[string]string{"GNOME_TERMINAL_SCREEN": "/org/x"}, nil, "gnome-terminal"},
		{nil, []string{"zsh", "ptyxis-agent", "systemd"}, "ptyxis"},
		{nil, []string{"bash", "gnome-terminal-"}, "gnome-terminal"},
		{map[string]string{"KONSOLE_VERSION": "240802"}, nil, "konsole"},
		{nil, []string{"zsh", "xfce4-terminal"}, "xfce4-terminal"},
		{nil, []string{"zsh", "sshd"}, "generic"},
	}
	for _, c := range cases {
		if got := Detect(as, c.env, c.ancestors).Name(); got != c.want {
			t.Errorf("Detect(%v, %v) = %s, want %s", c.env, c.ancestors, got, c.want)
		}
	}
}

func TestGeneric(t *testing.T) {
	// No display: print the commands instead of launching anything.
	f := &fakeExec{env: map[string]string{}}
	g := ByName(All(f.exec()), "generic")
	if err := g.OpenWindow(context.Background(), "work", isolatedForTest(twoLaunches)); err != ErrPrinted {
		t.Fatalf("printing instead of opening should be reported, got %v", err)
	}
	if strings.Contains(f.out.String(), "env -u") {
		t.Fatalf("printed commands should not carry the env prefix:\n%s", f.out.String())
	}
	if len(f.calls) != 0 {
		t.Fatalf("launched %v without a display", f.calls)
	}
	out := f.out.String()
	if !strings.Contains(out, `cd '/home/u/dev/my app' && /usr/bin/claude --resume id-1 -n 'acme · PAY-2193'`) {
		t.Fatalf("printed commands wrong:\n%s", out)
	}

	// $TERMINAL set: one window per session, cd'ing first.
	f = &fakeExec{env: map[string]string{"DISPLAY": ":0", "TERMINAL": "st"}}
	g = ByName(All(f.exec()), "generic")
	if err := g.OpenWindow(context.Background(), "work", twoLaunches[:1]); err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"st", "-e", "sh", "-c", `cd '/home/u/dev/my app' && exec /usr/bin/claude --resume id-1 -n 'acme · PAY-2193'`}}
	if !reflect.DeepEqual(f.calls, want) {
		t.Fatalf("got %q", f.calls)
	}

	// Display but no terminal found: print.
	f = &fakeExec{missing: map[string]bool{"x-terminal-emulator": true}}
	g = ByName(All(f.exec()), "generic")
	if err := g.OpenWindow(context.Background(), "work", twoLaunches[:1]); err != ErrPrinted {
		t.Fatalf("err = %v", err)
	}
	if len(f.calls) != 0 || !strings.Contains(f.out.String(), "Run these yourself") {
		t.Fatalf("calls=%q out=%q", f.calls, f.out.String())
	}

	// $TERMINAL may carry its own arguments.
	f = &fakeExec{env: map[string]string{"DISPLAY": ":0", "TERMINAL": "kitty --single-instance"}}
	g = ByName(All(f.exec()), "generic")
	if err := g.OpenWindow(context.Background(), "work", twoLaunches[:1]); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 1 || f.calls[0][0] != "kitty" || f.calls[0][1] != "--single-instance" || f.calls[0][2] != "-e" {
		t.Fatalf("calls = %q", f.calls)
	}
}

func isolatedForTest(ls []Launch) []Launch {
	out := make([]Launch, len(ls))
	for i, l := range ls {
		l.Argv = append([]string{"env", "-u", "CLAUDECODE", "-u", "CLAUDE_PID"}, l.Argv...)
		out[i] = l
	}
	return out
}

func TestKonsoleOpensOneWindowFromATabsFile(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	f := openWith(t, "konsole")
	if len(f.calls) != 1 || f.calls[0][0] != "konsole" || f.calls[0][1] != "--tabs-from-file" || f.calls[0][3] != "-e" || f.calls[0][4] != "true" {
		t.Fatalf("calls = %q", f.calls)
	}
	b, err := os.ReadFile(f.calls[0][2])
	if err != nil {
		t.Fatal(err)
	}
	want := "title: acme · PAY-2193;; workdir: /home/u/dev/my app;; command: /usr/bin/claude --resume id-1 -n 'acme · PAY-2193'\n" +
		"title: it's cvx;; workdir: /home/u/dev/cvx;; command: /usr/bin/claude --resume id-2 -n 'it'\\''s cvx'\n"
	if string(b) != want {
		t.Fatalf("tabs file:\n%s\nwant:\n%s", b, want)
	}
}
