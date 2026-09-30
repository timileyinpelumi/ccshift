// Package settings edits Claude Code's settings.json without disturbing what it does not own:
// key order is kept and values it does not touch are written back byte for byte.
package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type object struct {
	keys []string
	vals map[string]json.RawMessage
}

func parseObject(raw []byte) (*object, error) {
	o := &object{vals: map[string]json.RawMessage{}}
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, errors.New("not a JSON object")
	}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, _ := tok.(string)
		var val json.RawMessage
		if err := dec.Decode(&val); err != nil {
			return nil, err
		}
		if _, seen := o.vals[key]; !seen {
			o.keys = append(o.keys, key)
		}
		o.vals[key] = val
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	return o, nil
}

func (o *object) set(key string, val json.RawMessage) {
	if _, ok := o.vals[key]; !ok {
		o.keys = append(o.keys, key)
	}
	o.vals[key] = val
}

func (o *object) del(key string) {
	if _, ok := o.vals[key]; !ok {
		return
	}
	delete(o.vals, key)
	for i, k := range o.keys {
		if k == key {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			return
		}
	}
}

func (o *object) raw() json.RawMessage {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.Write(encode(k))
		b.WriteByte(':')
		b.Write(o.vals[k])
	}
	b.WriteByte('}')
	return b.Bytes()
}

// encode marshals without HTML escaping, so a command containing < or & stays readable.
func encode(v any) json.RawMessage {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.Encode(v)
	return bytes.TrimSpace(b.Bytes())
}

type File struct {
	Path     string
	root     *object
	original []byte
}

func Load(path string) (*File, error) {
	// Write through a symlink rather than replacing it, for settings kept in a dotfiles repo.
	if target, err := filepath.EvalSymlinks(path); err == nil {
		path = target
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &File{Path: path, root: &object{vals: map[string]json.RawMessage{}}}, nil
	}
	if err != nil {
		return nil, err
	}
	root, err := parseObject(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &File{Path: path, root: root, original: b}, nil
}

func (f *File) Bytes() ([]byte, error) {
	var out bytes.Buffer
	if err := json.Indent(&out, f.root.raw(), "", "  "); err != nil {
		return nil, err
	}
	out.WriteByte('\n')
	return out.Bytes(), nil
}

// Backup copies the file as it was when loaded. It does nothing if the file did not exist.
func (f *File) Backup(suffix string) error {
	if f.original == nil {
		return nil
	}
	return os.WriteFile(f.Path+suffix, f.original, 0o600)
}

func (f *File) Save() error {
	b, err := f.Bytes()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(f.Path), 0o700); err != nil {
		return err
	}
	// Claude Code rewrites this file too. Refuse rather than undo a change made since Load.
	current, err := os.ReadFile(f.Path)
	if errors.Is(err, fs.ErrNotExist) {
		current = nil
	} else if err != nil {
		return err
	}
	if !bytes.Equal(current, f.original) {
		return fmt.Errorf("%s changed while ccshift was editing it; run the command again", f.Path)
	}
	mode := fs.FileMode(0o600)
	if fi, err := os.Stat(f.Path); err == nil {
		mode = fi.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(f.Path), ".settings-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), f.Path); err != nil {
		return err
	}
	f.original = b
	return nil
}

type hook struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"`
}

func (f *File) hooks() (*object, error) {
	raw, ok := f.root.vals["hooks"]
	if !ok {
		return &object{vals: map[string]json.RawMessage{}}, nil
	}
	return parseObject(raw)
}

func (f *File) putHooks(h *object) {
	if len(h.keys) == 0 {
		f.root.del("hooks")
		return
	}
	f.root.set("hooks", h.raw())
}

func groupCommands(group json.RawMessage) []string {
	var g struct {
		Hooks []hook `json:"hooks"`
	}
	json.Unmarshal(group, &g)
	var out []string
	for _, h := range g.Hooks {
		if h.Type == "command" {
			out = append(out, h.Command)
		}
	}
	return out
}

func (f *File) HookCommands(event string) []string {
	h, err := f.hooks()
	if err != nil {
		return nil
	}
	var groups []json.RawMessage
	json.Unmarshal(h.vals[event], &groups)
	var out []string
	for _, g := range groups {
		out = append(out, groupCommands(g)...)
	}
	return out
}

func (f *File) AddHook(event, command string, timeoutSec int) (bool, error) {
	h, err := f.hooks()
	if err != nil {
		return false, fmt.Errorf("%s: hooks: %w", f.Path, err)
	}
	var groups []json.RawMessage
	if raw, ok := h.vals[event]; ok {
		if err := json.Unmarshal(raw, &groups); err != nil {
			return false, fmt.Errorf("%s: hooks.%s: %w", f.Path, event, err)
		}
	}
	for _, g := range groups {
		for _, c := range groupCommands(g) {
			if c == command {
				return false, nil
			}
		}
	}
	groups = append(groups, encode(map[string][]hook{"hooks": {{Type: "command", Command: command, Timeout: timeoutSec}}}))
	h.set(event, encode(groups))
	f.putHooks(h)
	return true, nil
}

// RemoveHooks deletes command hooks whose command matches, along with any group or event left empty.
func (f *File) RemoveHooks(match func(command string) bool) (int, error) {
	h, err := f.hooks()
	if err != nil {
		return 0, fmt.Errorf("%s: hooks: %w", f.Path, err)
	}
	removed := 0
	for _, event := range append([]string(nil), h.keys...) {
		var groups []json.RawMessage
		if err := json.Unmarshal(h.vals[event], &groups); err != nil {
			continue
		}
		var kept []json.RawMessage
		changed := false
		for _, g := range groups {
			obj, err := parseObject(g)
			if err != nil {
				kept = append(kept, g)
				continue
			}
			var hooks []json.RawMessage
			json.Unmarshal(obj.vals["hooks"], &hooks)
			var keepHooks []json.RawMessage
			for _, raw := range hooks {
				var hk hook
				json.Unmarshal(raw, &hk)
				if hk.Type == "command" && match(hk.Command) {
					removed++
					changed = true
					continue
				}
				keepHooks = append(keepHooks, raw)
			}
			switch {
			case len(keepHooks) == len(hooks):
				kept = append(kept, g)
			case len(keepHooks) > 0:
				obj.set("hooks", encode(keepHooks))
				kept = append(kept, obj.raw())
			}
		}
		if !changed {
			continue
		}
		if len(kept) == 0 {
			h.del(event)
		} else {
			h.set(event, encode(kept))
		}
	}
	if removed > 0 {
		f.putHooks(h)
	}
	return removed, nil
}

func (f *File) StatusLine() json.RawMessage {
	raw, ok := f.root.vals["statusLine"]
	if !ok {
		return nil
	}
	return raw
}

func (f *File) SetStatusLine(raw json.RawMessage) {
	if raw == nil {
		f.root.del("statusLine")
		return
	}
	f.root.set("statusLine", raw)
}
