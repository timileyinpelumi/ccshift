package claude

import "encoding/json"

// HookInput is what Claude Code writes to a hook command's stdin. Only the fields ccshift uses.
type HookInput struct {
	SessionID      string `json:"session_id"`
	Reason         string `json:"reason"`
	Source         string `json:"source"`
	TranscriptPath string `json:"transcript_path"`
	CWD            string `json:"cwd"`
}

func ParseHookInput(b []byte) HookInput {
	var in HookInput
	json.Unmarshal(b, &in)
	return in
}

// UserExit reports whether a SessionEnd reason means the user left the session on purpose.
// Checked on Claude Code 2.1.285: /exit, Ctrl+D and Ctrl+C give prompt_input_exit;
// SIGTERM and SIGHUP give other.
func (in HookInput) UserExit() bool {
	// Gemini CLI also reports "exit". Codex CLI always reports "other", so its exits look like crashes.
	return in.Reason == "prompt_input_exit" || in.Reason == "clear" || in.Reason == "resume" || in.Reason == "exit"
}

// StatusInput is what Claude Code writes to the statusline command's stdin.
type StatusInput struct {
	SessionID     string `json:"session_id"`
	SessionName   string `json:"session_name"`
	ContextWindow struct {
		UsedPercentage *float64 `json:"used_percentage"`
		Size           int      `json:"context_window_size"`
	} `json:"context_window"`
}

func ParseStatusInput(b []byte) StatusInput {
	var in StatusInput
	json.Unmarshal(b, &in)
	return in
}
