package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLastUsage(t *testing.T) {
	p := filepath.Join(t.TempDir(), "t.jsonl")
	lines := []string{
		`{"type":"assistant","message":{"usage":{"input_tokens":5,"cache_creation_input_tokens":10,"cache_read_input_tokens":100,"output_tokens":9}}}`,
		`{"type":"user","message":{"content":"hi"}}`,
		`{"type":"assistant","isSidechain":true,"message":{"usage":{"input_tokens":999999}}}`,
		`{"type":"assistant","message":{"usage":{"input_tokens":2,"cache_creation_input_tokens":3000,"cache_read_input_tokens":140000,"output_tokens":50}}}`,
		`not json`,
		`{"type":"ai-title","aiTitle":"x"}`,
	}
	os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
	if got, ok := LastUsage(p); !ok || got != 143002 {
		t.Fatalf("LastUsage = %d, %v", got, ok)
	}
	if _, ok := LastUsage(filepath.Join(t.TempDir(), "missing")); ok {
		t.Fatal("missing file should report no usage")
	}
	os.WriteFile(p, []byte(`{"type":"user"}`+"\n"), 0o644)
	if _, ok := LastUsage(p); ok {
		t.Fatal("no assistant record should report no usage")
	}
}

func TestLastAITitle(t *testing.T) {
	p := filepath.Join(t.TempDir(), "t.jsonl")
	os.WriteFile(p, []byte(`{"type":"ai-title","aiTitle":"First"}
{"type":"user"}
{"type":"ai-title","aiTitle":"Second title"}
{"type":"assistant","message":{}}
`), 0o644)
	if got := LastAITitle(p); got != "Second title" {
		t.Fatalf("got %q", got)
	}
	if got := LastAITitle(filepath.Join(t.TempDir(), "none")); got != "" {
		t.Fatalf("missing file: %q", got)
	}
}
