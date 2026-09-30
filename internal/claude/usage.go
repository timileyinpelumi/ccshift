package claude

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
)

const usageTail = 512 << 10

// LastUsage returns the context size, in tokens, of the latest main-thread API call in a transcript.
// The transcript format is not documented, so anything unexpected reports ok=false.
func LastUsage(transcriptPath string) (tokens int, ok bool) {
	f, err := os.Open(transcriptPath)
	if err != nil {
		return 0, false
	}
	defer f.Close()
	if fi, err := f.Stat(); err == nil && fi.Size() > usageTail {
		f.Seek(-usageTail, io.SeekEnd)
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return 0, false
	}
	lines := bytes.Split(b, []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		var rec struct {
			Type        string `json:"type"`
			IsSidechain bool   `json:"isSidechain"`
			Message     struct {
				Usage *struct {
					Input         int `json:"input_tokens"`
					CacheCreation int `json:"cache_creation_input_tokens"`
					CacheRead     int `json:"cache_read_input_tokens"`
				} `json:"usage"`
			} `json:"message"`
		}
		if json.Unmarshal(lines[i], &rec) != nil || rec.Type != "assistant" || rec.IsSidechain || rec.Message.Usage == nil {
			continue
		}
		u := rec.Message.Usage
		return u.Input + u.CacheCreation + u.CacheRead, true
	}
	return 0, false
}

// LastAITitle returns the latest title Claude Code generated for a session, or "" if there is none.
func LastAITitle(transcriptPath string) string {
	f, err := os.Open(transcriptPath)
	if err != nil {
		return ""
	}
	defer f.Close()
	if fi, err := f.Stat(); err == nil && fi.Size() > usageTail {
		f.Seek(-usageTail, io.SeekEnd)
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return ""
	}
	lines := bytes.Split(b, []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		if !bytes.Contains(lines[i], []byte(`"ai-title"`)) {
			continue
		}
		var rec struct {
			Type  string `json:"type"`
			Title string `json:"aiTitle"`
		}
		if json.Unmarshal(lines[i], &rec) == nil && rec.Type == "ai-title" && rec.Title != "" {
			return rec.Title
		}
	}
	return ""
}
