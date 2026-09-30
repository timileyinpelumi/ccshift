package cli

import (
	"context"
	"os"
	"runtime"

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

var goos = runtime.GOOS

// isolated makes each launch start with Claude Code's per-session variables removed, so the new
// session does not inherit a Claude session from the terminal it opens in. ccshift clears its own
// environment, but a tmux server or a terminal that was itself started from inside Claude hands
// its environment to every new tab. Unix has env -u. Windows has no such tool, so ccshift runs
// the command itself: "ccshift exec" starts with the variables already cleared.
func (a *App) isolated(ls []term.Launch) []term.Launch {
	prefix := []string{"env"}
	for _, k := range sessionEnv {
		prefix = append(prefix, "-u", k)
	}
	if goos == "windows" {
		prefix = []string{a.Executable, "exec", "--"}
	}
	out := make([]term.Launch, len(ls))
	for i, l := range ls {
		l.Argv = append(append([]string{}, prefix...), l.Argv...)
		out[i] = l
	}
	return out
}

// cmdExec runs a command with the session variables cleared (NewApp has done that already).
func (a *App) cmdExec(_ context.Context, args []string) error {
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		return usageError{"usage: ccshift exec -- <command> [args]"}
	}
	return a.Exec(args)
}
