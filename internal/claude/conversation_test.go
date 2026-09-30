package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTranscript(t *testing.T, lines ...string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "t.jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReadConversation(t *testing.T) {
	big := strings.Repeat("A", 3<<20) // a pasted image makes a line several MB long
	p := writeTranscript(t,
		`{"type":"user","isMeta":true,"message":{"content":"<local-command-caveat>ignore me</local-command-caveat>"}}`,
		`{"type":"user","message":{"content":"add dark mode please <system-reminder>hook noise</system-reminder>"}}`,
		`{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"secret"},{"type":"text","text":"Looking at the CSS."},{"type":"tool_use","name":"Read","input":{"file_path":"/w/site.css"}}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"x","content":"body{}"}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Edit","input":{"file_path":"/w/site.css","old_string":"a","new_string":"b"}},{"type":"tool_use","name":"Bash","input":{"command":"go test ./...\nsecond line"}}]}}`,
		`{"type":"assistant","isSidechain":true,"message":{"content":[{"type":"text","text":"subagent chatter"}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"Dark mode is in."}]}}`,
		`not json at all`,
		`{"type":"ai-title","aiTitle":"Dark mode"}`,
		`{"type":"user","message":{"content":[{"type":"image","source":{"data":"`+big+`"}},{"type":"text","text":"now the tagline"}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"TodoWrite","input":{"todos":[{"content":"write tagline","status":"in_progress"},{"content":"ship","status":"pending"}]}},{"type":"tool_use","name":"Agent","input":{"description":"research taglines"}}]}}`,
		`{"type":"user","isCompactSummary":true,"message":{"content":"Summary: user wants dark mode and a tagline."}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"Tagline drafted."}]}}`,
	)
	c, err := ReadConversation(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Turns) != 2 {
		t.Fatalf("turns = %d: %+v", len(c.Turns), c.Turns)
	}
	first := c.Turns[0]
	if first.User != "add dark mode please" {
		t.Fatalf("user text = %q", first.User)
	}
	if len(first.Texts) != 2 || first.Texts[1] != "Dark mode is in." {
		t.Fatalf("texts = %q", first.Texts)
	}
	wantTools := []Tool{{"read", "/w/site.css"}, {"edited", "/w/site.css"}, {"ran", "go test ./..."}}
	if len(first.Tools) != 3 || first.Tools[0] != wantTools[0] || first.Tools[1] != wantTools[1] || first.Tools[2] != wantTools[2] {
		t.Fatalf("tools = %+v", first.Tools)
	}
	second := c.Turns[1]
	if second.User != "now the tagline" || len(second.Texts) != 1 {
		t.Fatalf("second turn = %+v", second)
	}
	if len(second.Tools) != 1 || second.Tools[0] != (Tool{"used Agent", "research taglines"}) {
		t.Fatalf("second tools = %+v", second.Tools)
	}
	if len(c.Summaries) != 1 || !strings.HasPrefix(c.Summaries[0], "Summary:") {
		t.Fatalf("summaries = %q", c.Summaries)
	}
	if len(c.Todos) != 2 || c.Todos[0] != (Todo{"write tagline", "in_progress"}) {
		t.Fatalf("todos = %+v", c.Todos)
	}
	for _, turn := range c.Turns {
		for _, s := range append(turn.Texts, turn.User) {
			if strings.Contains(s, "secret") || strings.Contains(s, "subagent") || strings.Contains(s, "hook noise") || strings.Contains(s, "AAAA") {
				t.Fatalf("leaked: %.80q", s)
			}
		}
	}
}

func TestReadConversationMissingFile(t *testing.T) {
	if _, err := ReadConversation(filepath.Join(t.TempDir(), "none.jsonl")); err == nil {
		t.Fatal("expected an error")
	}
}

func TestReadConversationSlashCommands(t *testing.T) {
	p := writeTranscript(t,
		`{"type":"user","message":{"content":"<command-name>/model</command-name> <command-message>model</command-message> <command-args></command-args>"}}`,
		`{"type":"user","message":{"content":"<local-command-stdout>Set model to Opus</local-command-stdout>"}}`,
		`{"type":"user","message":{"content":"<command-message>code-review</command-message> <command-name>/code-review</command-name> <command-args>high --fix</command-args>"}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"Reviewing."}]}}`,
	)
	c, _ := ReadConversation(p)
	if len(c.Turns) != 2 || c.Turns[0].User != "/model" || c.Turns[1].User != "/code-review high --fix" {
		t.Fatalf("turns = %+v", c.Turns)
	}
}

func TestReadConversationKeepsWhatTheUserTyped(t *testing.T) {
	p := writeTranscript(t,
		`{"type":"user","message":{"content":"<task-notification><task-id>x</task-id><result>agent output</result></task-notification>"}}`,
		`{"type":"user","message":{"content":"[SYSTEM NOTIFICATION - NOT USER INPUT]\nbackground event"}}`,
		`{"type":"user","message":{"content":"why does the parser drop <command-name> tags and <system-reminder>x</system-reminder> blocks?\n\n  func main() {\n      fmt.Println(1)\n  }"}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"Because of the regex."}]}}`,
	)
	c, _ := ReadConversation(p)
	if len(c.Turns) != 1 {
		t.Fatalf("harness records became turns: %+v", c.Turns)
	}
	u := c.Turns[0].User
	if !strings.Contains(u, "<command-name> tags and <system-reminder>x</system-reminder> blocks?") {
		t.Fatalf("text the user typed about tags was removed: %q", u)
	}
	if !strings.Contains(u, "\n  func main() {\n      fmt.Println(1)\n  }") {
		t.Fatalf("pasted code lost its line breaks and indentation: %q", u)
	}
}
