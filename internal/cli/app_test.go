package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func newTestApp() (*App, *bytes.Buffer, *bytes.Buffer) {
	out, errb := &bytes.Buffer{}, &bytes.Buffer{}
	return &App{Out: out, Err: errb, In: strings.NewReader("")}, out, errb
}

func TestUnknownCommand(t *testing.T) {
	a, _, errb := newTestApp()
	if code := a.Run(context.Background(), []string{"nope"}); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), `unknown command "nope"`) {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestHelpListsCommands(t *testing.T) {
	a, out, _ := newTestApp()
	if code := a.Run(context.Background(), nil); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(out.String(), "version") {
		t.Fatalf("help output missing version command: %q", out.String())
	}
}

func TestVersion(t *testing.T) {
	a, out, _ := newTestApp()
	if code := a.Run(context.Background(), []string{"version"}); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if strings.TrimSpace(out.String()) != version {
		t.Fatalf("version output = %q", out.String())
	}
}
