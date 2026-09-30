package claude

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"regexp"
	"strings"
)

// Conversation is the part of a transcript a person would recognise: what was asked, what Claude
// said, and what it touched. The transcript format is not documented and changes between versions,
// so anything unexpected is skipped.
type Conversation struct {
	Turns     []Turn
	Summaries []string // text left by earlier /compact runs
	Todos     []Todo   // the latest todo list
}

type Turn struct {
	User  string
	Texts []string // Claude's text replies, in order
	Tools []Tool
}

type Tool struct {
	Verb   string // "read", "edited", "ran", "searched", or "used <Tool>"
	Detail string
}

type Todo struct {
	Content string
	Status  string
}

type block struct {
	Type  string          `json:"type"`
	Text  string          `json:"text"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

type record struct {
	Type             string `json:"type"`
	IsSidechain      bool   `json:"isSidechain"`
	IsMeta           bool   `json:"isMeta"`
	IsCompactSummary bool   `json:"isCompactSummary"`
	Message          struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

var (
	leadingReminder  = regexp.MustCompile(`(?s)^\s*<system-reminder>.*?</system-reminder>\s*`)
	trailingReminder = regexp.MustCompile(`(?s)\s*<system-reminder>(?:[^<]|<[^/]|</[^s])*?</system-reminder>\s*$`)
	slashCommand     = regexp.MustCompile(`(?s)^\s*(?:<command-message>.*?</command-message>\s*)?<command-name>(.*?)</command-name>`)
	commandArgs      = regexp.MustCompile(`(?s)<command-args>(.*?)</command-args>`)
	localOutput      = regexp.MustCompile(`(?s)^\s*<local-command-\w+>.*</local-command-\w+>\s*$`)
	blankLines       = regexp.MustCompile(`\n{3,}`)
)

// cleanText drops what the harness added around the user's words and keeps the words as typed,
// line breaks included. Tags are only treated as harness text where the harness puts them: a
// reminder at the start or end, or a message that is nothing but a command or a notification.
// A slash command becomes "/name args".
func cleanText(s string) string {
	for {
		t := trailingReminder.ReplaceAllString(leadingReminder.ReplaceAllString(s, ""), "")
		if t == s {
			break
		}
		s = t
	}
	trimmed := strings.TrimSpace(s)
	if strings.HasPrefix(trimmed, "<task-notification>") || strings.HasPrefix(trimmed, "[SYSTEM NOTIFICATION") || localOutput.MatchString(s) {
		return ""
	}
	if m := slashCommand.FindStringSubmatch(s); m != nil {
		cmd := strings.TrimSpace(m[1])
		if a := commandArgs.FindStringSubmatch(s); a != nil && strings.TrimSpace(a[1]) != "" {
			cmd += " " + strings.Join(strings.Fields(a[1]), " ")
		}
		return cmd
	}
	return blankLines.ReplaceAllString(trimmed, "\n\n")
}

func firstLine(s string, limit int) string {
	s, _, _ = strings.Cut(strings.TrimSpace(s), "\n")
	if r := []rune(s); len(r) > limit {
		return string(r[:limit]) + "…"
	}
	return s
}

func toolOf(b block) (Tool, []Todo) {
	var in struct {
		FilePath     string `json:"file_path"`
		NotebookPath string `json:"notebook_path"`
		Command      string `json:"command"`
		Pattern      string `json:"pattern"`
		Description  string `json:"description"`
		Todos        []struct {
			Content string `json:"content"`
			Status  string `json:"status"`
		} `json:"todos"`
	}
	json.Unmarshal(b.Input, &in)
	switch b.Name {
	case "Read":
		return Tool{"read", in.FilePath}, nil
	case "Edit", "Write", "MultiEdit":
		return Tool{"edited", in.FilePath}, nil
	case "NotebookEdit":
		return Tool{"edited", in.NotebookPath}, nil
	case "Bash":
		return Tool{"ran", firstLine(in.Command, 160)}, nil
	case "Grep", "Glob":
		return Tool{"searched", in.Pattern}, nil
	case "TodoWrite":
		todos := make([]Todo, len(in.Todos))
		for i, t := range in.Todos {
			todos[i] = Todo{t.Content, t.Status}
		}
		return Tool{}, todos
	}
	return Tool{"used " + b.Name, firstLine(in.Description, 80)}, nil
}

func ReadConversation(transcriptPath string) (Conversation, error) {
	f, err := os.Open(transcriptPath)
	if err != nil {
		return Conversation{}, err
	}
	defer f.Close()
	var c Conversation
	// Lines can be several megabytes (pasted images, tool output), so no fixed-size scanner.
	r := bufio.NewReaderSize(f, 1<<20)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			c.add(line)
		}
		if errors.Is(err, io.EOF) {
			return c, nil
		}
		if err != nil {
			return c, err
		}
	}
}

func (c *Conversation) add(line []byte) {
	var rec record
	if json.Unmarshal(line, &rec) != nil || rec.IsSidechain || rec.IsMeta {
		return
	}
	if rec.Type != "user" && rec.Type != "assistant" {
		return
	}
	var blocks []block
	var plain string
	if json.Unmarshal(rec.Message.Content, &plain) == nil {
		blocks = []block{{Type: "text", Text: plain}}
	} else if json.Unmarshal(rec.Message.Content, &blocks) != nil {
		return
	}
	if rec.Type == "user" {
		var parts []string
		for _, b := range blocks {
			if b.Type != "text" {
				continue
			}
			// Cleaned one block at a time: the harness adds its reminders as blocks of their own.
			if t := cleanText(b.Text); t != "" {
				parts = append(parts, t)
			}
		}
		text := strings.Join(parts, "\n")
		switch {
		case text == "":
			// Tool results and images only.
		case rec.IsCompactSummary:
			c.Summaries = append(c.Summaries, text)
		default:
			c.Turns = append(c.Turns, Turn{User: text})
		}
		return
	}
	if len(c.Turns) == 0 {
		c.Turns = append(c.Turns, Turn{})
	}
	turn := &c.Turns[len(c.Turns)-1]
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if t := strings.TrimSpace(b.Text); t != "" {
				turn.Texts = append(turn.Texts, t)
			}
		case "tool_use":
			tool, todos := toolOf(b)
			if todos != nil {
				c.Todos = todos
			} else if tool.Verb != "" {
				turn.Tools = append(turn.Tools, tool)
			}
		}
	}
}
