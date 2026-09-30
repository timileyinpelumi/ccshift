package cli

import (
	"os"

	"github.com/timileyinpelumi/ccshift/internal/term"
)

// Set by Claude Code for processes it starts. A session launched with these inherited
// starts as a child of the calling session, with transcript saving off.
var sessionEnv = []string{
	"CLAUDECODE",
	"CLAUDE_CODE_CHILD_SESSION",
	"CLAUDE_CODE_ENTRYPOINT",
	"CLAUDE_CODE_EXECPATH",
	"CLAUDE_CODE_MESSAGING_SOCKET",
	"CLAUDE_CODE_MESSAGING_TOKEN",
	"CLAUDE_CODE_SESSION_ATTENDED",
	"CLAUDE_CODE_SESSION_ID",
	"CLAUDE_EFFORT",
	"CLAUDE_PID",
}

func scrubSessionEnv() {
	for _, k := range sessionEnv {
		os.Unsetenv(k)
	}
}

// isolated makes each launch start through env -u, so the new session does not inherit a Claude
// session from the terminal it opens in. ccshift clears its own environment, but a tmux server or
// a terminal that was itself started from inside Claude hands its environment to every new tab.
func isolated(ls []term.Launch) []term.Launch {
	prefix := []string{"env"}
	for _, k := range sessionEnv {
		prefix = append(prefix, "-u", k)
	}
	out := make([]term.Launch, len(ls))
	for i, l := range ls {
		l.Argv = append(append([]string{}, prefix...), l.Argv...)
		out[i] = l
	}
	return out
}
