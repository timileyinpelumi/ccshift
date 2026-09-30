package layout

import (
	"testing"
	"time"

	"github.com/timileyinpelumi/ccshift/internal/claude"
	"github.com/timileyinpelumi/ccshift/internal/store"
)

func liveItems(ids ...string) []Item {
	var out []Item
	for _, id := range ids {
		out = append(out, Item{Session: claude.Session{ID: id, CWD: "/w/" + id, Name: "n-" + id}})
	}
	return out
}

func entryIDs(s store.Snapshot) []string {
	var out []string
	for _, e := range s.Sessions {
		out = append(out, e.SessionID)
	}
	return out
}

func TestMergeKeepsSessionsThatStoppedRunning(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	prev := Snapshot("work", liveItems("a", "b", "c", "d"), "tmux", now.Add(-time.Minute))
	// A shutdown took b and d; a and c are still up and swapped places, e is new.
	got := Merge(prev, true, "work", liveItems("c", "a", "e"), "tmux", now, 14*24*time.Hour)
	if ids := entryIDs(got); !eq(ids, []string{"c", "b", "a", "d", "e"}) {
		t.Fatalf("order = %v", ids)
	}
	for _, e := range got.Sessions {
		wantExit := ""
		if e.SessionID == "b" || e.SessionID == "d" {
			wantExit = ExitUnclean
		}
		if e.Exit != wantExit {
			t.Errorf("%s exit = %q", e.SessionID, e.Exit)
		}
	}
	if got.Sessions[4].Position != 5 || got.Sessions[0].LastSeen != now.Unix() {
		t.Fatalf("entries = %+v", got.Sessions)
	}
}

func TestMergeWithNothingRunningKeepsEverything(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	prev := Snapshot("work", liveItems("a", "b"), "tmux", now.Add(-time.Minute))
	got := Merge(prev, true, "work", nil, "tmux", now, time.Hour)
	if ids := entryIDs(got); !eq(ids, []string{"a", "b"}) {
		t.Fatalf("order = %v", ids)
	}
}

func TestMergeDropsStaleSessions(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	prev := Snapshot("work", liveItems("a", "b"), "tmux", now.Add(-20*24*time.Hour))
	got := Merge(prev, true, "work", liveItems("a"), "tmux", now, 14*24*time.Hour)
	if ids := entryIDs(got); !eq(ids, []string{"a"}) {
		t.Fatalf("stale session kept: %v", ids)
	}
}

func TestMergeRevivedSessionLosesUncleanMark(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	prev := Snapshot("work", liveItems("a"), "tmux", now.Add(-time.Minute))
	prev.Sessions[0].Exit = ExitUnclean
	got := Merge(prev, true, "work", liveItems("a"), "tmux", now, time.Hour)
	if got.Sessions[0].Exit != "" {
		t.Fatalf("exit = %q", got.Sessions[0].Exit)
	}
}

func TestSameSessions(t *testing.T) {
	now := time.Now()
	a := Snapshot("w", liveItems("a", "b"), "tmux", now)
	b := Snapshot("w", liveItems("a", "b"), "tmux", now.Add(time.Hour))
	if !SameSessions(a, b) {
		t.Fatal("same layout at a later time should compare equal")
	}
	b.Sessions[1].Name = "renamed"
	if SameSessions(a, b) {
		t.Fatal("a rename is a change")
	}
	c := Snapshot("w", liveItems("b", "a"), "tmux", now)
	if SameSessions(a, c) {
		t.Fatal("a reorder is a change")
	}
}
