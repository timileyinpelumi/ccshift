package cli

import (
	"os"
	"testing"
)

func TestScrubSessionEnv(t *testing.T) {
	t.Setenv("CLAUDE_CODE_CHILD_SESSION", "1")
	t.Setenv("CLAUDE_CODE_SESSION_ID", "abc")
	t.Setenv("CLAUDECODE", "1")
	t.Setenv("CLAUDE_CONFIG_DIR", "/keep")
	t.Setenv("CLAUDE_CODE_USE_BEDROCK", "1")
	scrubSessionEnv()
	for _, k := range []string{"CLAUDE_CODE_CHILD_SESSION", "CLAUDE_CODE_SESSION_ID", "CLAUDECODE"} {
		if _, ok := os.LookupEnv(k); ok {
			t.Errorf("%s should be unset", k)
		}
	}
	for _, k := range []string{"CLAUDE_CONFIG_DIR", "CLAUDE_CODE_USE_BEDROCK"} {
		if os.Getenv(k) == "" {
			t.Errorf("%s should be kept", k)
		}
	}
}
