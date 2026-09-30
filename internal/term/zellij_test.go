package term

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestZellijList(t *testing.T) {
	b, err := os.ReadFile("testdata/zellij-list-panes.json")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeExec{outputs: map[string]string{"zellij action list-panes": string(b)}}
	f.env = map[string]string{"ZELLIJ": "0", "ZELLIJ_SESSION_NAME": "main"}
	a := ByName(All(f.exec()), "zellij")
	tabs, err := a.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []Tab{{"0", "main", "1"}, {"1", "main", "2"}, {"2", "main", "3"}}
	if !reflect.DeepEqual(tabs, want) {
		t.Fatalf("tabs = %v", tabs)
	}
}

func TestZellijOpenInsideSession(t *testing.T) {
	f := &fakeExec{env: map[string]string{"ZELLIJ": "0", "ZELLIJ_SESSION_NAME": "main"}}
	a := ByName(All(f.exec()), "zellij")
	if err := a.OpenWindow(context.Background(), "work", twoLaunches); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 2 || f.calls[0][2] != "new-tab" || f.calls[0][3] != "--name=acme · PAY-2193" {
		t.Fatalf("calls = %q", f.calls)
	}
}

func TestZellijLayoutQuoting(t *testing.T) {
	got := tabLayout(twoLaunches[1])
	if !strings.Contains(got, `args "--resume" "id-2" "-n" "it's cvx"`) || !strings.Contains(got, `cwd="/home/u/dev/cvx"`) {
		t.Fatalf("layout:\n%s", got)
	}
	if kdlQuote(`a"b\c`) != `"a\"b\\c"` {
		t.Fatalf("kdlQuote = %s", kdlQuote(`a"b\c`))
	}
}

func TestZellijOpenOutsideSessionPrintsCommand(t *testing.T) {
	f := &fakeExec{env: map[string]string{}}
	a := ByName(All(f.exec()), "zellij")
	if err := a.OpenWindow(context.Background(), "work", twoLaunches); err != ErrPrinted {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(f.out.String(), "zellij --session work --new-session-with-layout ") {
		t.Fatalf("out = %q", f.out.String())
	}
}

func TestZellijOutsideSessionReusesOneLayoutFile(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	f := &fakeExec{env: map[string]string{}}
	a := ByName(All(f.exec()), "zellij")
	a.OpenWindow(context.Background(), "work", twoLaunches)
	first := f.out.String()
	f.out.Reset()
	a.OpenWindow(context.Background(), "work", twoLaunches[:1])
	if first != f.out.String() {
		t.Fatalf("layout path changed between runs:\n%s\n%s", first, f.out.String())
	}
	path := strings.TrimSpace(first[strings.LastIndex(first, " ")+1:])
	if b, err := os.ReadFile(path); err != nil || strings.Contains(string(b), "id-2") {
		t.Fatalf("layout file should hold the latest restore only: %v", err)
	}
}

func TestZellijTitleAndFocus(t *testing.T) {
	b, _ := os.ReadFile("testdata/zellij-list-panes.json")
	f := &fakeExec{outputs: map[string]string{"zellij action list-panes": string(b)}, env: map[string]string{"ZELLIJ": "0"}}
	a := ByName(All(f.exec()), "zellij")
	if err := a.SetTitle(context.Background(), "2", "-x"); err != nil {
		t.Fatal(err)
	}
	if err := a.Focus(context.Background(), "1"); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"zellij", "action", "list-panes", "--json"},
		{"zellij", "action", "rename-tab-by-id", "2", "--", "-x"},
		{"zellij", "action", "list-panes", "--json"},
		{"zellij", "action", "go-to-tab-by-id", "1"},
	}
	if !reflect.DeepEqual(f.calls, want) {
		t.Fatalf("calls = %q", f.calls)
	}
}
