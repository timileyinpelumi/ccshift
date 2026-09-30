package layout

import (
	"time"

	"github.com/timileyinpelumi/ccshift/internal/store"
)

const ExitUnclean = "unclean"

// Merge builds the snapshot autosave writes. Running sessions come in tab order. Saved sessions that
// are no longer running stay at their old position, so a shutdown or crash never shrinks the save.
// Only sessions that have not run for staleAfter are dropped.
func Merge(prev store.Snapshot, hasPrev bool, ws string, items []Item, terminal string, now time.Time, staleAfter time.Duration) store.Snapshot {
	snap := Snapshot(ws, items, terminal, now)
	if !hasPrev {
		return snap
	}
	live := map[string]bool{}
	for _, e := range snap.Sessions {
		live[e.SessionID] = true
	}
	out := snap.Sessions
	for i, e := range prev.Sessions {
		if live[e.SessionID] {
			continue
		}
		seen := prev.SavedAt
		if e.LastSeen != 0 {
			seen = time.Unix(e.LastSeen, 0)
		}
		if now.Sub(seen) > staleAfter {
			continue
		}
		if e.LastSeen == 0 {
			e.LastSeen = seen.Unix()
		}
		e.Exit = ExitUnclean
		at := min(i, len(out))
		out = append(out[:at], append([]store.Entry{e}, out[at:]...)...)
	}
	for i := range out {
		out[i].Position = i + 1
	}
	snap.Sessions = out
	return snap
}

// SameSessions reports whether two snapshots describe the same layout, ignoring when they were taken.
func SameSessions(a, b store.Snapshot) bool {
	if len(a.Sessions) != len(b.Sessions) {
		return false
	}
	for i := range a.Sessions {
		x, y := a.Sessions[i], b.Sessions[i]
		if x.SessionID != y.SessionID || x.CWD != y.CWD || x.Name != y.Name || x.Generated != y.Generated || x.Exit != y.Exit {
			return false
		}
	}
	return true
}
