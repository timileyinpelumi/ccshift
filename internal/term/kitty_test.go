package term

import (
	"context"
	"os"
	"reflect"
	"testing"
)

func TestKittyList(t *testing.T) {
	ls, err := os.ReadFile("testdata/kitty-ls.json")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeExec{outputs: map[string]string{"kitten @ ls": string(ls)}}
	a := ByName(All(f.exec()), "kitty")
	tabs, err := a.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []Tab{{"1", "1", "1.1"}, {"2", "2", "2.1"}, {"3", "2", "2.2"}}
	if !reflect.DeepEqual(tabs, want) {
		t.Fatalf("tabs = %v", tabs)
	}
	if id, ok := a.TabOf(map[string]string{"KITTY_WINDOW_ID": "3"}); !ok || id != "3" {
		t.Fatalf("TabOf = %q %v", id, ok)
	}
}

func TestKittyOpenWindow(t *testing.T) {
	f := &fakeExec{outputs: map[string]string{"kitten @ launch --type=os-window": "12\n"}}
	a := ByName(All(f.exec()), "kitty")
	if err := a.OpenWindow(context.Background(), "work", twoLaunches); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"kitten", "@", "launch", "--type=os-window", "--os-window-title=work", "--cwd=/home/u/dev/my app", "--tab-title=acme · PAY-2193", "--", "/usr/bin/claude", "--resume", "id-1", "-n", "acme · PAY-2193"},
		{"kitten", "@", "launch", "--type=tab", "--match=window_id:12", "--cwd=/home/u/dev/cvx", "--tab-title=it's cvx", "--", "/usr/bin/claude", "--resume", "id-2", "-n", "it's cvx"},
	}
	if !reflect.DeepEqual(f.calls, want) {
		t.Fatalf("calls:\n%q\nwant\n%q", f.calls, want)
	}
}

func TestKittyTitleAndFocus(t *testing.T) {
	f := &fakeExec{}
	a := ByName(All(f.exec()), "kitty")
	a.SetTitle(context.Background(), "3", "new")
	a.Focus(context.Background(), "3")
	want := [][]string{
		{"kitten", "@", "set-tab-title", "--match=window_id:3", "--", "new"},
		{"kitten", "@", "focus-window", "--match=id:3"},
	}
	if !reflect.DeepEqual(f.calls, want) {
		t.Fatalf("calls = %q", f.calls)
	}
}
