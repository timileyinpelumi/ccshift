package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAddHistoryLeavesLatestAlone(t *testing.T) {
	st, _ := Open(t.TempDir())
	now := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)
	st.SaveSnapshot(snap("work", now, "a"), 20)
	if err := st.AddHistory(snap("work", now.Add(-time.Hour), "old1", "old2")); err != nil {
		t.Fatal(err)
	}
	latest, _, _ := st.Latest("work")
	if len(latest.Sessions) != 1 {
		t.Fatalf("latest changed: %+v", latest)
	}
	if h, _ := st.History("work"); len(h) != 2 || len(h[1].Sessions) != 2 {
		t.Fatalf("history = %+v", h)
	}
}

func TestContextRoundTrip(t *testing.T) {
	st, _ := Open(t.TempDir())
	c, err := st.Context()
	if err != nil || len(c) != 0 {
		t.Fatalf("empty context = %v, %v", c, err)
	}
	c["s1"] = ContextEntry{Percent: 72.5, WindowSize: 200000, Name: "api", Warned: []int{70}}
	if err := st.SetContext(c); err != nil {
		t.Fatal(err)
	}
	c, _ = st.Context()
	if c["s1"].Percent != 72.5 || c["s1"].Warned[0] != 70 {
		t.Fatalf("context = %+v", c)
	}
}

func TestInitStateAndStamp(t *testing.T) {
	st, _ := Open(t.TempDir())
	s, _ := st.InitState()
	if s.LegacyImported || s.PreviousStatusLine != nil {
		t.Fatalf("zero state = %+v", s)
	}
	s.PreviousStatusLine = json.RawMessage(`{"type":"command","command":"x"}`)
	s.LegacyImported = true
	st.SetInitState(s)
	s, _ = st.InitState()
	if !s.LegacyImported || !strings.Contains(string(s.PreviousStatusLine), `"x"`) {
		t.Fatalf("state = %+v", s)
	}
	if !st.LastAutosave().IsZero() {
		t.Fatal("no autosave yet")
	}
	now := time.Unix(1790000000, 0)
	st.StampAutosave(now)
	if !st.LastAutosave().Equal(now) {
		t.Fatalf("stamp = %v", st.LastAutosave())
	}
}

func TestLogAppends(t *testing.T) {
	st, _ := Open(t.TempDir())
	st.Log("first %d", 1)
	st.Log("second")
	b, _ := os.ReadFile(filepath.Join(st.Dir, "ccshift.log"))
	if lines := strings.Split(strings.TrimSpace(string(b)), "\n"); len(lines) != 2 || !strings.HasSuffix(lines[0], "first 1") {
		t.Fatalf("log = %q", b)
	}
}
