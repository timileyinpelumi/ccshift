package cli

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/timileyinpelumi/ccshift/internal/claude"
	"github.com/timileyinpelumi/ccshift/internal/layout"
	"github.com/timileyinpelumi/ccshift/internal/term"
)

const (
	statuslineBudget = 3 * time.Second
	// A statusline reading newer than this wins over an estimate from the transcript.
	contextFresh = 10 * time.Minute
	contextKeep  = 30 * 24 * time.Hour
	// A threshold counts as crossed again only after usage has dropped this far below it.
	warnHysteresis = 10
	// Set for the previous statusline command, so ccshift does not run itself in a loop.
	statuslineGuard = "CCSHIFT_STATUSLINE"
)

// cmdStatusline records context usage, then shows whatever the user's own statusline command prints.
// Like hooks, it never fails.
func (a *App) cmdStatusline(ctx context.Context, _ []string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			a.Store.Log("statusline: panic: %v", r)
		}
		if err != nil {
			a.Store.Log("statusline: %v", err)
		}
		err = nil
	}()
	ctx, cancel := context.WithTimeout(ctx, statuslineBudget)
	defer cancel()
	b := readInput(a.In)
	in := claude.ParseStatusInput(b)
	if in.SessionID != "" && in.ContextWindow.UsedPercentage != nil {
		if err := a.recordContext(in.SessionID, in.SessionName, *in.ContextWindow.UsedPercentage, in.ContextWindow.Size, "statusline", true); err != nil {
			a.Store.Log("statusline: %v", err)
		}
	}
	out := ""
	if a.Env[statuslineGuard] == "" {
		out = a.previousStatusline(ctx, b)
	}
	if strings.TrimSpace(out) == "" {
		out = defaultLine(in)
	}
	fmt.Fprint(a.Out, out)
	return nil
}

func defaultLine(in claude.StatusInput) string {
	var parts []string
	if in.SessionName != "" {
		parts = append(parts, in.SessionName)
	}
	if in.ContextWindow.UsedPercentage != nil {
		parts = append(parts, fmt.Sprintf("%d%%", int(*in.ContextWindow.UsedPercentage)))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " · ") + "\n"
}

func (a *App) previousStatusline(ctx context.Context, stdin []byte) string {
	st, err := a.Store.InitState()
	if err != nil || st.PreviousStatusLine == nil {
		return ""
	}
	command := statusLineCommand(st.PreviousStatusLine)
	if command == "" || a.ours(command) {
		return ""
	}
	// Whatever it printed is shown, even when it exits non-zero.
	out, _ := a.Shell(ctx, command, stdin)
	return string(out)
}

func (a *App) recordContext(id, name string, pct float64, size int, source string, mayNotify bool) error {
	unlock, err := a.Store.TryLock(200 * time.Millisecond)
	if err != nil {
		return err
	}
	defer unlock()
	all, err := a.Store.Context()
	if err != nil || all == nil {
		all = map[string]storeContext{}
	}
	now := a.Now()
	e := all[id]
	age := now.Sub(time.Unix(e.UpdatedAt, 0))
	if source == "transcript" && e.Source == "statusline" && age < contextFresh {
		return nil
	}
	if name == "" {
		name = e.Name
	}
	if size == 0 {
		size = e.WindowSize
	}

	var warned, fresh []int
	for _, th := range a.Config.WarnThresholds {
		already := slices.Contains(e.Warned, th)
		switch {
		case pct >= float64(th):
			warned = append(warned, th)
			if !already {
				fresh = append(fresh, th)
			}
		case already && pct >= float64(th-warnHysteresis):
			warned = append(warned, th)
		}
	}
	if !mayNotify {
		// Without a notification the threshold stays unwarned, so a later trusted reading can notify.
		warned = slices.DeleteFunc(warned, func(th int) bool { return slices.Contains(fresh, th) })
		fresh = nil
	}
	unchanged := int(e.Percent) == int(pct) && e.Name == name && e.Source == source && slices.Equal(e.Warned, warned)
	if unchanged && age < time.Minute {
		return nil
	}
	all[id] = storeContext{Percent: pct, WindowSize: size, Name: name, UpdatedAt: now.Unix(), Warned: warned, Source: source}
	for k, v := range all {
		if now.Sub(time.Unix(v.UpdatedAt, 0)) > contextKeep {
			delete(all, k)
		}
	}
	if err := a.Store.SetContext(all); err != nil {
		return err
	}
	if len(fresh) > 0 {
		label := name
		if label == "" {
			label, _ = a.savedName(id)
		}
		a.Notify("ccshift", fmt.Sprintf("%s is at %d%% of its context window. Hand it off with: ccshift handoff %s", label, int(pct), term.ShellJoin([]string{label})))
	}
	return nil
}

// savedName looks a session up in the saved layouts. Headless runs (claude -p) are never saved.
func (a *App) savedName(id string) (string, bool) {
	wss, _ := a.Store.Workspaces()
	for _, ws := range wss {
		if snap, ok, err := a.Store.Latest(ws); err == nil && ok {
			for _, e := range snap.Sessions {
				if e.SessionID == id {
					return layout.Title(e), true
				}
			}
		}
	}
	if len(id) > 8 {
		return id[:8], false
	}
	return id, false
}

// contextFromTranscript covers sessions whose statusline is not wired through ccshift.
func (a *App) contextFromTranscript(in claude.HookInput) {
	if in.SessionID == "" || in.TranscriptPath == "" {
		return
	}
	if _, saved := a.savedName(in.SessionID); !saved {
		return
	}
	tokens, ok := claude.LastUsage(in.TranscriptPath)
	if !ok {
		return
	}
	// The transcript does not say how large the window is. Use the size the statusline last
	// reported; without one, guess, and do not notify on a guess.
	size, known := 0, false
	if all, err := a.Store.Context(); err == nil && all[in.SessionID].WindowSize > 0 {
		size, known = all[in.SessionID].WindowSize, true
	}
	if !known {
		size = 200000
		if tokens > size {
			size = 1000000
		}
	}
	if err := a.recordContext(in.SessionID, "", float64(tokens)*100/float64(size), size, "transcript", known); err != nil {
		a.Store.Log("context estimate: %v", err)
	}
}

func contextCell(e storeContext, known bool, thresholds []int) string {
	if !known {
		return "-"
	}
	s := fmt.Sprintf("%d%%", int(e.Percent))
	if e.Source == "transcript" {
		s = "~" + s
	}
	if len(thresholds) > 0 && e.Percent >= float64(thresholds[0]) {
		s += "!"
	}
	return s
}
