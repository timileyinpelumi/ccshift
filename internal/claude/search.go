package claude

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// Match is a past session whose conversation contains every search word in one message.
type Match struct {
	SessionID string
	CWD       string
	Title     string // the session's custom name, or Claude's own title for it
	Modified  time.Time
	Snippet   string // the matching message around the first word, on one line
}

// Search looks through every transcript under projectsDir, newest first. Words match without
// regard to case, and all of them must be in the same message.
func Search(projectsDir string, words []string, limit int) ([]Match, error) {
	files, err := filepath.Glob(filepath.Join(projectsDir, "*", "*.jsonl"))
	if err != nil {
		return nil, err
	}
	lower := make([][]byte, len(words))
	for i, w := range words {
		lower[i] = bytes.ToLower([]byte(w))
	}
	var (
		mu  sync.Mutex
		out []Match
		wg  sync.WaitGroup
	)
	jobs := make(chan string)
	for range runtime.NumCPU() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for f := range jobs {
				if m, ok := searchFile(f, lower); ok {
					mu.Lock()
					out = append(out, m)
					mu.Unlock()
				}
			}
		}()
	}
	for _, f := range files {
		jobs <- f
	}
	close(jobs)
	wg.Wait()
	sort.Slice(out, func(i, j int) bool { return out[i].Modified.After(out[j].Modified) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func containsAll(s []byte, words [][]byte) bool {
	for _, w := range words {
		if !bytes.Contains(s, w) {
			return false
		}
	}
	return true
}

func searchFile(path string, words [][]byte) (Match, bool) {
	f, err := os.Open(path)
	if err != nil {
		return Match{}, false
	}
	defer f.Close()
	m := Match{SessionID: strings.TrimSuffix(filepath.Base(path), ".jsonl")}
	if fi, err := f.Stat(); err == nil {
		m.Modified = fi.ModTime()
	}
	var custom, ai string
	found := false
	r := bufio.NewReaderSize(f, 1<<20)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			switch {
			case bytes.Contains(line, []byte(`"custom-title"`)):
				var rec struct {
					CustomTitle string `json:"customTitle"`
				}
				if json.Unmarshal(line, &rec) == nil && rec.CustomTitle != "" {
					custom = rec.CustomTitle
				}
			case bytes.Contains(line, []byte(`"ai-title"`)):
				var rec struct {
					AITitle string `json:"aiTitle"`
				}
				if json.Unmarshal(line, &rec) == nil && rec.AITitle != "" {
					ai = rec.AITitle
				}
			}
			if m.CWD == "" && bytes.Contains(line, []byte(`"cwd"`)) {
				var rec struct {
					CWD string `json:"cwd"`
				}
				if json.Unmarshal(line, &rec) == nil {
					m.CWD = rec.CWD
				}
			}
			// A cheap test on the raw line first; most lines never need decoding.
			if containsAll(bytes.ToLower(line), words) {
				if text := messageText(line); text != "" && containsAll([]byte(strings.ToLower(text)), words) {
					m.Snippet = snippet(text, string(words[0]))
					found = true
				}
			}
		}
		if errors.Is(err, io.EOF) || err != nil {
			break
		}
	}
	m.Title = custom
	if m.Title == "" {
		m.Title = ai
	}
	return m, found
}

// messageText is the text of a user or main-thread assistant message, or "".
func messageText(line []byte) string {
	var rec record
	if json.Unmarshal(line, &rec) != nil || rec.IsSidechain || rec.IsMeta || (rec.Type != "user" && rec.Type != "assistant") {
		return ""
	}
	var plain string
	if json.Unmarshal(rec.Message.Content, &plain) == nil {
		return cleanText(plain)
	}
	var blocks []block
	if json.Unmarshal(rec.Message.Content, &blocks) != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" {
			parts = append(parts, b.Text)
		}
	}
	return cleanText(strings.Join(parts, " "))
}

func snippet(text, word string) string {
	flat := strings.Join(strings.Fields(text), " ")
	i := strings.Index(strings.ToLower(flat), word)
	if i < 0 {
		i = 0
	}
	start := max(0, i-50)
	for start > 0 && !isBoundary(flat, start) {
		start--
	}
	end := min(len(flat), i+90)
	for end < len(flat) && !isBoundary(flat, end) {
		end++
	}
	s := flat[start:end]
	if start > 0 {
		s = "…" + s
	}
	if end < len(flat) {
		s += "…"
	}
	return s
}

func isBoundary(s string, i int) bool { return i >= len(s) || s[i] == ' ' }
