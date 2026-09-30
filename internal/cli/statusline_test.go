package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/timileyinpelumi/ccshift/internal/store"
	"github.com/timileyinpelumi/ccshift/internal/term"
)

func statusJSON(id, name string, pct float64) string {
	b, _ := json.Marshal(map[string]any{
		"session_id": id, "session_name": name,
		"context_window": map[string]any{"used_percentage": pct, "context_window_size": 200000},
	})
	return string(b)
}

func (h *harness) status(t *testing.T, stdin string) string {
	t.Helper()
	h.app.In = strings.NewReader(stdin)
	return h.must(t, "statusline")
}

func TestStatuslineRecordsAndRunsThePreviousCommand(t *testing.T) {
	h := testApp(t, term.Exact)
	h.app.Store.SetInitState(store.InitState{PreviousStatusLine: json.RawMessage(`{"type":"command","command":"node x.js"}`)})
	var gotCmd, gotStdin string
	h.app.Shell = func(_ context.Context, cmd string, stdin []byte) ([]byte, error) {
		gotCmd, gotStdin = cmd, string(stdin)
		return []byte("their line\n"), nil
	}
	in := statusJSON("s1", "api", 41.5)
	if out := h.status(t, in); out != "their line\n" {
		t.Fatalf("out = %q", out)
	}
	if gotCmd != "node x.js" || gotStdin != in {
		t.Fatalf("previous command got %q / %q", gotCmd, gotStdin)
	}
	c, _ := h.app.Store.Context()
	if c["s1"].Percent != 41.5 || c["s1"].Name != "api" || c["s1"].WindowSize != 200000 {
		t.Fatalf("context = %+v", c)
	}
}

func TestStatuslineDefaultLine(t *testing.T) {
	h := testApp(t, term.Exact)
	if out := h.status(t, statusJSON("s1", "api", 41.5)); out != "api · 41%\n" {
		t.Fatalf("out = %q", out)
	}
	h.app.Store.SetInitState(store.InitState{PreviousStatusLine: json.RawMessage(`{"type":"command","command":"broken"}`)})
	h.app.Shell = func(context.Context, string, []byte) ([]byte, error) { return nil, errors.New("exit 1") }
	if out := h.status(t, statusJSON("s1", "api", 41.5)); out != "api · 41%\n" {
		t.Fatalf("failing previous command: out = %q", out)
	}
	h.app.Shell = func(context.Context, string, []byte) ([]byte, error) { return []byte("  \n"), nil }
	if out := h.status(t, statusJSON("s1", "api", 41.5)); out != "api · 41%\n" {
		t.Fatalf("empty previous output: out = %q", out)
	}
}

func TestStatuslineNeverFails(t *testing.T) {
	h := testApp(t, term.Exact)
	for _, in := range []string{"", "garbage", `{"session_id":"s1"}`, `{"context_window":{"used_percentage":null}}`} {
		h.status(t, in)
	}
	if c, _ := h.app.Store.Context(); len(c) != 0 {
		t.Fatalf("recorded context from bad input: %+v", c)
	}
}

func TestContextWarningFiresOncePerThreshold(t *testing.T) {
	h := testApp(t, term.Exact)
	var notes []string
	h.app.Notify = func(title, body string) { notes = append(notes, body) }
	for _, pct := range []float64{50, 72, 74, 86, 88} {
		h.status(t, statusJSON("s1", "api", pct))
	}
	if len(notes) != 2 || !strings.Contains(notes[0], "api is at 72%") || !strings.Contains(notes[1], "86%") {
		t.Fatalf("notes = %q", notes)
	}
	// After /compact the percentage drops; crossing again warns again.
	h.status(t, statusJSON("s1", "api", 30))
	h.status(t, statusJSON("s1", "api", 90))
	if len(notes) != 3 || !strings.Contains(notes[2], "90%") {
		t.Fatalf("notes after re-crossing = %q", notes)
	}
}

func TestLsShowsContext(t *testing.T) {
	h := testApp(t, term.Exact)
	h.sessions = twoSessions()
	h.app.Store.SetContext(map[string]store.ContextEntry{"s1": {Percent: 72.4}, "zz": {Percent: 5}})
	lines := strings.Split(strings.TrimSpace(h.must(t, "ls")), "\n")
	if !strings.Contains(lines[0], "CTX") || !strings.Contains(lines[1], " 72%! ") || !strings.Contains(lines[2], " - ") {
		t.Fatalf("ls:\n%s", strings.Join(lines, "\n"))
	}
}

func TestStopHookEstimatesContextFromTranscript(t *testing.T) {
	h := testApp(t, term.Exact)
	var notes []string
	h.app.Notify = func(_, body string) { notes = append(notes, body) }
	p := filepath.Join(t.TempDir(), "s1.jsonl")
	os.WriteFile(p, []byte(`{"type":"assistant","message":{"usage":{"input_tokens":1000,"cache_read_input_tokens":149000}}}`+"\n"), 0o644)
	h.sessions = twoSessions()
	h.hook(t, "stop", stopInput("s1", p))
	c, _ := h.app.Store.Context()
	// The window size is a guess here, so the estimate is recorded but does not notify.
	if c["s1"].Percent != 75 || c["s1"].Source != "transcript" || len(notes) != 0 {
		t.Fatalf("context = %+v notes = %q", c["s1"], notes)
	}
	if out := h.must(t, "ls"); !strings.Contains(out, " ~75%! ") {
		t.Fatalf("ls should mark an estimate:\n%s", out)
	}
	// A headless run (claude -p) is not a saved session and gets no entry.
	h.hook(t, "stop", stopInput("headless", p))
	if c, _ = h.app.Store.Context(); len(c) != 1 {
		t.Fatalf("context = %+v", c)
	}
	// A fresh statusline reading wins over the estimate.
	h.status(t, statusJSON("s1", "one", 20))
	h.hook(t, "stop", stopInput("s1", p))
	if c, _ = h.app.Store.Context(); c["s1"].Percent != 20 {
		t.Fatalf("estimate overwrote the statusline reading: %+v", c["s1"])
	}
}

// stopInput builds the hook's JSON with the path encoded, since Windows paths contain backslashes.
func stopInput(id, transcript string) string {
	b, _ := json.Marshal(map[string]string{"session_id": id, "transcript_path": transcript})
	return string(b)
}
