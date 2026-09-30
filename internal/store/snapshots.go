package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/timileyinpelumi/ccshift/internal/config"
)

type Entry struct {
	SessionID string `json:"session_id"`
	CWD       string `json:"cwd"`
	Name      string `json:"name,omitempty"`
	Position  int    `json:"position"`
	Generated bool   `json:"generated_name,omitempty"`
	Exit      string `json:"exit,omitempty"`
	LastSeen  int64  `json:"last_seen,omitempty"`
}

type Snapshot struct {
	Workspace string    `json:"workspace"`
	SavedAt   time.Time `json:"saved_at"`
	Terminal  string    `json:"terminal"`
	Auto      bool      `json:"auto,omitempty"`
	Sessions  []Entry   `json:"sessions"`
}

const (
	stampLayout = "20060102T150405.000Z"
	autoSuffix  = "-auto.json"
	// Autosaves get their own, smaller history so they never push out saves the user made.
	autoKeep  = 10
	autoEvery = time.Hour
)

func wsDir(ws string) string { return filepath.Join("workspaces", ws) }

func (s *Store) SaveSnapshot(snap Snapshot, keep int) error {
	if !config.ValidWorkspaceName(snap.Workspace) {
		return fmt.Errorf("invalid workspace name %q", snap.Workspace)
	}
	snap.Auto = false
	name := snap.SavedAt.UTC().Format(stampLayout) + ".json"
	if err := s.writeJSON(filepath.Join(wsDir(snap.Workspace), "history", name), snap); err != nil {
		return err
	}
	if err := s.writeJSON(filepath.Join(wsDir(snap.Workspace), "latest.json"), snap); err != nil {
		return err
	}
	return s.prune(snap.Workspace, keep)
}

// SaveLatest replaces the latest snapshot without adding to history.
func (s *Store) SaveLatest(snap Snapshot) error {
	if !config.ValidWorkspaceName(snap.Workspace) {
		return fmt.Errorf("invalid workspace name %q", snap.Workspace)
	}
	return s.writeJSON(filepath.Join(wsDir(snap.Workspace), "latest.json"), snap)
}

// SaveAuto is SaveLatest plus at most one history entry per hour.
func (s *Store) SaveAuto(snap Snapshot, keep int) error {
	snap.Auto = true
	if err := s.SaveLatest(snap); err != nil {
		return err
	}
	files, err := s.historyFiles(snap.Workspace)
	if err != nil {
		return err
	}
	for i := len(files) - 1; i >= 0; i-- {
		base := filepath.Base(files[i])
		if !strings.HasSuffix(base, autoSuffix) {
			continue
		}
		if at, err := time.Parse(stampLayout, strings.TrimSuffix(base, autoSuffix)); err == nil && snap.SavedAt.Sub(at) < autoEvery {
			return nil
		}
		break
	}
	name := snap.SavedAt.UTC().Format(stampLayout) + autoSuffix
	if err := s.writeJSON(filepath.Join(wsDir(snap.Workspace), "history", name), snap); err != nil {
		return err
	}
	return s.prune(snap.Workspace, keep)
}

// AddHistory records a snapshot without making it the latest one.
func (s *Store) AddHistory(snap Snapshot) error {
	if !config.ValidWorkspaceName(snap.Workspace) {
		return fmt.Errorf("invalid workspace name %q", snap.Workspace)
	}
	name := snap.SavedAt.UTC().Format(stampLayout) + ".json"
	return s.writeJSON(filepath.Join(wsDir(snap.Workspace), "history", name), snap)
}

func (s *Store) Latest(ws string) (Snapshot, bool, error) {
	var snap Snapshot
	if !config.ValidWorkspaceName(ws) {
		return snap, false, nil
	}
	ok, err := s.readJSON(filepath.Join(wsDir(ws), "latest.json"), &snap)
	return snap, ok, err
}

func (s *Store) historyFiles(ws string) ([]string, error) {
	files, err := filepath.Glob(filepath.Join(s.Dir, wsDir(ws), "history", "*.json"))
	sort.Strings(files)
	return files, err
}

func (s *Store) History(ws string) ([]Snapshot, error) {
	if !config.ValidWorkspaceName(ws) {
		return nil, nil
	}
	files, err := s.historyFiles(ws)
	if err != nil {
		return nil, err
	}
	var out []Snapshot
	for i := len(files) - 1; i >= 0; i-- {
		rel, _ := filepath.Rel(s.Dir, files[i])
		var snap Snapshot
		if ok, err := s.readJSON(rel, &snap); ok && err == nil {
			out = append(out, snap)
		}
	}
	return out, nil
}

func (s *Store) prune(ws string, keep int) error {
	files, err := s.historyFiles(ws)
	if err != nil {
		return err
	}
	var manual, auto []string
	for _, f := range files {
		if strings.HasSuffix(f, autoSuffix) {
			auto = append(auto, f)
		} else {
			manual = append(manual, f)
		}
	}
	for _, g := range []struct {
		files []string
		keep  int
	}{{manual, keep}, {auto, autoKeep}} {
		for len(g.files) > g.keep {
			if err := os.Remove(g.files[0]); err != nil {
				return err
			}
			g.files = g.files[1:]
		}
	}
	return nil
}

func (s *Store) Workspaces() ([]string, error) {
	m, err := filepath.Glob(filepath.Join(s.Dir, "workspaces", "*", "latest.json"))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, p := range m {
		out = append(out, filepath.Base(filepath.Dir(p)))
	}
	sort.Strings(out)
	return out, nil
}

func (s *Store) Order() (map[string][]string, error) {
	o := map[string][]string{}
	_, err := s.readJSON("order.json", &o)
	return o, err
}

func (s *Store) SetOrder(ws string, ids []string) error {
	o, err := s.Order()
	if err != nil {
		// An unreadable order file is replaced rather than blocking every later save.
		o = map[string][]string{}
	}
	o[ws] = ids
	return s.writeJSON("order.json", o)
}
