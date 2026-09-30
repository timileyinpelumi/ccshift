package term

import (
	"context"
	"reflect"
	"testing"
)

func konsoleFake() *fakeExec {
	return &fakeExec{
		missing: map[string]bool{"qdbus6": true},
		env:     map[string]string{"KONSOLE_DBUS_SERVICE": ":1.0", "KONSOLE_DBUS_SESSION": "/Sessions/1", "KONSOLE_DBUS_WINDOW": "/Windows/1"},
		outputs: map[string]string{
			"qdbus :1.0 /Windows/1 sessionList": "1\n3\n2\n",
			"qdbus :1.0 /Windows/2 sessionList": "5\n",
			"qdbus :1.0 /Windows/1 newSession":  "7\n",
		},
	}
}

func TestKonsoleListFollowsTabOrder(t *testing.T) {
	f := konsoleFake()
	a := ByName(All(f.exec()), "konsole")
	if a.Tier() != Exact {
		t.Fatalf("tier = %v", a.Tier())
	}
	f.paths = "/\n/Sessions\n/Sessions/1\n/Windows\n/Windows/2\n/Windows/1\n/konsole\n"
	tabs, err := a.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []Tab{{"1", "1", "1.1"}, {"3", "1", "1.2"}, {"2", "1", "1.3"}, {"5", "2", "2.1"}}
	if !reflect.DeepEqual(tabs, want) {
		t.Fatalf("tabs = %v", tabs)
	}
	if id, ok := a.TabOf(map[string]string{"KONSOLE_DBUS_SERVICE": ":1.0", "KONSOLE_DBUS_SESSION": "/Sessions/3"}); !ok || id != "3" {
		t.Fatalf("TabOf = %q %v", id, ok)
	}
	if _, ok := a.TabOf(map[string]string{"KONSOLE_DBUS_SERVICE": ":1.9", "KONSOLE_DBUS_SESSION": "/Sessions/3"}); ok {
		t.Fatal("a session in another Konsole process must not match")
	}
}

func TestKonsoleTitleFocusAndNewTab(t *testing.T) {
	f := konsoleFake()
	f.paths = "/Windows/1\n/Windows/2\n"
	a := ByName(All(f.exec()), "konsole")
	ctx := context.Background()
	if err := a.SetTitle(ctx, "3", "50% done · it's"); err != nil {
		t.Fatal(err)
	}
	if err := a.Focus(ctx, "5"); err != nil {
		t.Fatal(err)
	}
	f.calls = nil
	l := Launch{CWD: "/w dir", Title: "api (2)", Argv: []string{"claude", "--name=api (2)"}}
	if err := a.(TabOpener).OpenTab(ctx, "3", l); err != nil {
		t.Fatal(err)
	}
	var got [][]string
	for _, c := range f.calls {
		if len(c) > 3 && (c[3] == "newSession" || c[3] == "runCommand" || c[3] == "setTabTitleFormat") {
			got = append(got, c)
		}
	}
	want := [][]string{
		{"qdbus", ":1.0", "/Windows/1", "newSession", "", "/w dir"},
		{"qdbus", ":1.0", "/Sessions/7", "runCommand", "exec claude '--name=api (2)'"},
		{"qdbus", ":1.0", "/Sessions/7", "setTabTitleFormat", "0", "api (2)"},
		{"qdbus", ":1.0", "/Sessions/7", "setTabTitleFormat", "1", "api (2)"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("calls:\n%q\nwant\n%q", got, want)
	}
}

func TestKonsoleTitleEscapesPercent(t *testing.T) {
	f := konsoleFake()
	a := ByName(All(f.exec()), "konsole")
	a.SetTitle(context.Background(), "3", "50% done")
	last := f.calls[len(f.calls)-1]
	if last[len(last)-1] != "50%% done" {
		t.Fatalf("title sent = %q", last)
	}
}

func TestKonsoleOutsideItOrWithoutQdbus(t *testing.T) {
	outside := &fakeExec{env: map[string]string{"DISPLAY": ":0"}}
	a := ByName(All(outside.exec()), "konsole")
	if _, err := a.List(context.Background()); err == nil {
		t.Fatal("List outside Konsole should fail, so callers fall back to start order")
	}
	noTool := konsoleFake()
	noTool.missing = map[string]bool{"qdbus6": true, "qdbus": true, "qdbus-qt5": true}
	a = ByName(All(noTool.exec()), "konsole")
	if err := a.(Checker).Check(context.Background()); err == nil {
		t.Fatal("Check should say qdbus is missing")
	}
}
