package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/timileyinpelumi/ccshift/internal/telemetry"
	"github.com/timileyinpelumi/ccshift/internal/term"
)

func withTelemetry(t *testing.T, h *harness) (queue func() string, started *[][]string) {
	h.app.Telemetry = &telemetry.Client{Dir: h.app.Store.Dir, Endpoint: "http://127.0.0.1:1", Version: "test", OS: "linux", Arch: "amd64", Now: time.Now}
	h.app.Executable = "/bin/ccshift"
	var s [][]string
	h.app.Detach = func(argv []string) error { s = append(s, argv); return nil }
	return func() string {
		b, _ := os.ReadFile(filepath.Join(h.app.Store.Dir, "telemetry.jsonl"))
		return string(b)
	}, &s
}

func TestCommandsAreRecordedAndFlushedInTheBackground(t *testing.T) {
	h := testApp(t, term.Exact)
	queue, started := withTelemetry(t, h)
	h.must(t, "ls")
	q := queue()
	if !strings.Contains(q, `"command":"ls"`) || !strings.Contains(q, `"ok":true`) || !strings.Contains(q, `"terminal":"fake"`) {
		t.Fatalf("queue = %s", q)
	}
	if len(*started) != 1 || strings.Join((*started)[0], " ") != "/bin/ccshift telemetry flush" {
		t.Fatalf("started = %q", *started)
	}
}

func TestHooksRecordSessionsAndNeverSend(t *testing.T) {
	h := testApp(t, term.Exact)
	queue, started := withTelemetry(t, h)
	h.hook(t, "session-start", `{"session_id":"s1","source":"startup"}`)
	if q := queue(); !strings.Contains(q, `"command":"session"`) || !strings.Contains(q, `"agent":"claude"`) {
		t.Fatalf("queue = %s", q)
	}
	_ = started
}

func TestErrorsAreScrubbed(t *testing.T) {
	h := testApp(t, term.Exact)
	saveSnap(t, h, "work", "abcd1234")
	snap, _, _ := h.app.Store.Latest("work")
	snap.Sessions[0].Name = "secret project"
	h.app.Store.SaveLatest(snap)
	got := h.app.scrub(errors.New(`couldn't open "x" for secret project in /home/me/acme at abcd1234: exit status 1`).Error())
	for _, leak := range []string{"secret project", "/home/me", "acme", "abcd1234", `"x"`} {
		if strings.Contains(got, leak) {
			t.Fatalf("scrubbed = %q still has %q", got, leak)
		}
	}
	if !strings.HasPrefix(got, "couldn't open") {
		t.Fatalf("scrubbed = %q", got)
	}
}
