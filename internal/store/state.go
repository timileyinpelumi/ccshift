package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type ContextEntry struct {
	Percent    float64 `json:"percent"`
	WindowSize int     `json:"window_size,omitempty"`
	Name       string  `json:"name,omitempty"`
	UpdatedAt  int64   `json:"updated_at"`
	Warned     []int   `json:"warned,omitempty"`
	Source     string  `json:"source,omitempty"`
}

func (s *Store) Context() (map[string]ContextEntry, error) {
	c := map[string]ContextEntry{}
	_, err := s.readJSON("context.json", &c)
	return c, err
}

func (s *Store) SetContext(c map[string]ContextEntry) error {
	return s.writeJSON("context.json", c)
}

type InitState struct {
	PreviousStatusLine json.RawMessage `json:"previous_statusline,omitempty"`
	LegacyImported     bool            `json:"legacy_imported,omitempty"`
	Installed          bool            `json:"installed,omitempty"`
}

func (s *Store) InitState() (InitState, error) {
	var st InitState
	_, err := s.readJSON("init.json", &st)
	return st, err
}

func (s *Store) SetInitState(st InitState) error { return s.writeJSON("init.json", st) }

func (s *Store) LastAutosave() time.Time {
	var v struct {
		Last int64 `json:"last"`
	}
	if ok, err := s.readJSON("autosave.json", &v); !ok || err != nil || v.Last == 0 {
		return time.Time{}
	}
	return time.Unix(v.Last, 0)
}

func (s *Store) StampAutosave(t time.Time) error {
	return s.writeJSON("autosave.json", map[string]int64{"last": t.Unix()})
}

// Log appends a line to ccshift.log. Hooks have nowhere else to report a failure.
func (s *Store) Log(format string, args ...any) {
	f, err := os.OpenFile(filepath.Join(s.Dir, "ccshift.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s\n", time.Now().Format(time.RFC3339), fmt.Sprintf(format, args...))
}

// Ended holds sessions the user just exited, by session id, with the time they ended.
func (s *Store) Ended() map[string]int64 {
	e := map[string]int64{}
	s.readJSON("ended.json", &e)
	return e
}

func (s *Store) SetEnded(e map[string]int64) error { return s.writeJSON("ended.json", e) }

// Excluded holds running sessions the user removed in save --edit, so autosave leaves them out.
func (s *Store) Excluded() map[string]int64 {
	e := map[string]int64{}
	s.readJSON("excluded.json", &e)
	return e
}

func (s *Store) SetExcluded(e map[string]int64) error { return s.writeJSON("excluded.json", e) }

// Load and Put read and write a small JSON state file in the store by name.
func (s *Store) Load(name string, v any) error {
	_, err := s.readJSON(name, v)
	return err
}

func (s *Store) Put(name string, v any) error { return s.writeJSON(name, v) }

// WriteText writes a text file in the store and returns its full path.
func (s *Store) WriteText(rel, content string) (string, error) {
	p := filepath.Join(s.Dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".tmp-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	return p, os.Rename(tmp.Name(), p)
}

// PruneOlder removes files in a store directory that were last written more than age before now.
func (s *Store) PruneOlder(rel string, age time.Duration, now time.Time) {
	entries, err := os.ReadDir(filepath.Join(s.Dir, rel))
	if err != nil {
		return
	}
	for _, e := range entries {
		if info, err := e.Info(); err == nil && !e.IsDir() && now.Sub(info.ModTime()) > age {
			os.Remove(filepath.Join(s.Dir, rel, e.Name()))
		}
	}
}
