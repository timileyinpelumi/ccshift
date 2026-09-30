package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func snap(ws string, at time.Time, ids ...string) Snapshot {
	s := Snapshot{Workspace: ws, SavedAt: at, Terminal: "tmux"}
	for i, id := range ids {
		s.Sessions = append(s.Sessions, Entry{SessionID: id, CWD: "/w/" + id, Name: "n-" + id, Position: i + 1})
	}
	return s
}

func TestSnapshotsRoundTripAndPrune(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)
	for i := range 4 {
		if err := st.SaveSnapshot(snap("work", base.Add(time.Duration(i)*time.Minute), "a", "b"), 3); err != nil {
			t.Fatal(err)
		}
	}
	latest, ok, err := st.Latest("work")
	if err != nil || !ok {
		t.Fatalf("Latest: %v %v", ok, err)
	}
	if !latest.SavedAt.Equal(base.Add(3*time.Minute)) || len(latest.Sessions) != 2 || latest.Sessions[1].Name != "n-b" {
		t.Fatalf("latest = %+v", latest)
	}
	hist, err := st.History("work")
	if err != nil || len(hist) != 3 {
		t.Fatalf("history len = %d, %v", len(hist), err)
	}
	if !hist[0].SavedAt.After(hist[2].SavedAt) {
		t.Fatal("history not newest first")
	}
	if _, ok, _ := st.Latest("nope"); ok {
		t.Fatal("Latest of unknown workspace should be missing")
	}
	wss, _ := st.Workspaces()
	if len(wss) != 1 || wss[0] != "work" {
		t.Fatalf("Workspaces = %v", wss)
	}
}

func TestNoTempFilesLeft(t *testing.T) {
	dir := t.TempDir()
	st, _ := Open(dir)
	st.SaveSnapshot(snap("work", time.Now(), "a"), 5)
	st.SetOrder("work", []string{"a"})
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if strings.HasPrefix(d.Name(), ".tmp-") {
			t.Errorf("temp file left behind: %s", p)
		}
		return nil
	})
}

func TestInvalidWorkspaceRejected(t *testing.T) {
	st, _ := Open(t.TempDir())
	if err := st.SaveSnapshot(snap("../escape", time.Now(), "a"), 5); err == nil {
		t.Fatal("expected error for invalid workspace name")
	}
}

func TestOrder(t *testing.T) {
	st, _ := Open(t.TempDir())
	if o, err := st.Order(); err != nil || len(o) != 0 {
		t.Fatalf("empty order = %v, %v", o, err)
	}
	st.SetOrder("work", []string{"b", "a"})
	st.SetOrder("personal", []string{"c"})
	o, _ := st.Order()
	if len(o["work"]) != 2 || o["work"][0] != "b" || o["personal"][0] != "c" {
		t.Fatalf("order = %v", o)
	}
}

func TestLockSerializes(t *testing.T) {
	st, _ := Open(t.TempDir())
	unlock, err := st.Lock()
	if err != nil {
		t.Fatal(err)
	}
	got := make(chan struct{})
	go func() {
		u, err := st.Lock()
		if err == nil {
			u()
		}
		close(got)
	}()
	select {
	case <-got:
		t.Fatal("second lock acquired while first held")
	case <-time.After(100 * time.Millisecond):
	}
	unlock()
	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("second lock never acquired")
	}
}
