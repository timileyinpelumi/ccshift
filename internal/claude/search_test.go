package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSearch(t *testing.T) {
	root := t.TempDir()
	write := func(dir, id string, age time.Duration, lines ...string) {
		p := filepath.Join(root, dir, id+".jsonl")
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
		when := time.Now().Add(-age)
		os.Chtimes(p, when, when)
	}
	write("-w-api", "aaaa1111", time.Hour,
		`{"type":"user","cwd":"/w/api","message":{"content":"the login page throws on submit"}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"Fixed the Auth bug in session.go by checking the token first."}]}}`,
		`{"type":"ai-title","aiTitle":"Fix login"}`,
		`{"type":"custom-title","customTitle":"api · PAY-12"}`)
	write("-w-web", "bbbb2222", 10*time.Minute,
		`{"type":"user","cwd":"/w/web","message":{"content":"style the header, then look at the auth bug again"}}`,
		`{"type":"assistant","isSidechain":true,"message":{"content":[{"type":"text","text":"auth bug in a subagent"}]}}`)
	write("-w-docs", "cccc3333", time.Minute,
		`{"type":"user","cwd":"/w/docs","message":{"content":"write the install guide"}}`)

	got, err := Search(root, []string{"auth", "BUG"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].SessionID != "bbbb2222" || got[1].SessionID != "aaaa1111" {
		t.Fatalf("matches = %+v", got)
	}
	if got[1].Title != "api · PAY-12" || got[1].CWD != "/w/api" || !strings.Contains(got[1].Snippet, "Auth bug in session.go") {
		t.Fatalf("match = %+v", got[1])
	}
	if got[0].Title != "" || !strings.Contains(got[0].Snippet, "auth bug again") {
		t.Fatalf("match = %+v", got[0])
	}
	if got, _ := Search(root, []string{"nothing", "here"}, 10); len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
	if got, _ := Search(root, []string{"the"}, 1); len(got) != 1 {
		t.Fatalf("limit ignored: %d", len(got))
	}
}
