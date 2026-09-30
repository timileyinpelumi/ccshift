package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/timileyinpelumi/ccshift/internal/term"
)

func TestNotifyRunsTheCommandWhenClicked(t *testing.T) {
	h := testApp(t, term.Exact)
	var shown [][]string
	h.app.NotifySend = func(args []string) (string, error) { shown = append(shown, args); return "click\n", nil }
	var started [][]string
	h.app.Detach = func(argv []string) error { started = append(started, argv); return nil }
	h.must(t, "notify", "ccshift", "api is at 72%", "--", "/bin/ccshift", "handoff", "s1", "--auto")
	if len(shown) != 1 || !strings.Contains(strings.Join(shown[0], " "), "--action=click=Hand off") {
		t.Fatalf("shown = %q", shown)
	}
	if len(started) != 1 || strings.Join(started[0], " ") != "/bin/ccshift handoff s1 --auto" {
		t.Fatalf("started = %q", started)
	}
}

func TestNotifyFallsBackWithoutActions(t *testing.T) {
	h := testApp(t, term.Exact)
	var shown [][]string
	h.app.NotifySend = func(args []string) (string, error) {
		shown = append(shown, args)
		if strings.Contains(strings.Join(args, " "), "--action") {
			return "", errors.New("Unknown option --action")
		}
		return "", nil
	}
	h.app.Detach = func(argv []string) error { t.Fatal("nothing was clicked"); return nil }
	h.must(t, "notify", "ccshift", "api is at 72%", "--", "/bin/ccshift", "handoff", "s1", "--auto")
	if len(shown) != 2 || strings.Contains(strings.Join(shown[1], " "), "--action") {
		t.Fatalf("shown = %q", shown)
	}
}
