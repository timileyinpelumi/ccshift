package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"github.com/timileyinpelumi/ccshift/internal/config"
	"github.com/timileyinpelumi/ccshift/internal/settings"
	"github.com/timileyinpelumi/ccshift/internal/store"
	"github.com/timileyinpelumi/ccshift/internal/term"
)

const hookTimeoutSeconds = 5

var hookEvents = []struct{ event, arg string }{
	{"SessionStart", "session-start"},
	{"Stop", "stop"},
	{"SessionEnd", "session-end"},
}

// Matches a command ccshift installed, whatever path the binary had at the time.
var ours = regexp.MustCompile(`ccshift(\.exe)?['"]? (hook (session-start|stop|session-end)|statusline)$`)

// ours reports whether ccshift installed a command: by shape, or exactly what it would install now
// (which also covers a binary that is not named ccshift).
func (a *App) ours(command string) bool {
	if ours.MatchString(command) || command == a.command("statusline") {
		return true
	}
	for _, h := range hookEvents {
		if command == a.command("hook "+h.arg) {
			return true
		}
	}
	return false
}

func (a *App) installed(f *settings.File) bool {
	if a.ours(statusLineCommand(f.StatusLine())) {
		return true
	}
	for _, h := range hookEvents {
		for _, c := range f.HookCommands(h.event) {
			if a.ours(c) {
				return true
			}
		}
	}
	return false
}

func (a *App) command(args string) string {
	return term.ShellJoin([]string{a.Executable}) + " " + args
}

func (a *App) settingsPath() string { return filepath.Join(a.ClaudeDir, "settings.json") }

func isNull(raw json.RawMessage) bool {
	return raw == nil || string(bytes.TrimSpace(raw)) == "null"
}

func statusLineCommand(raw json.RawMessage) string {
	var sl struct {
		Command string `json:"command"`
	}
	json.Unmarshal(raw, &sl)
	return sl.Command
}

func (a *App) cmdInit(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	remove := fs.Bool("remove", false, "take ccshift's hooks and statusline out of settings.json")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return usageError{err.Error()}
	}
	if len(pos) > 0 {
		return usageError{"usage: ccshift init [--remove]"}
	}
	f, err := settings.Load(a.settingsPath())
	if err != nil {
		return fmt.Errorf("can't read %w; fix the file and run this again", err)
	}
	before, _ := f.Bytes()
	st, err := a.Store.InitState()
	if err != nil {
		return err
	}

	hadOurs := a.installed(f)

	if *remove {
		if _, err := f.RemoveHooks(a.ours); err != nil {
			return err
		}
		if a.ours(statusLineCommand(f.StatusLine())) {
			prev := st.PreviousStatusLine
			if !st.Installed {
				// The record of the previous statusline is gone; the backup still has it.
				if bak, err := settings.Load(f.Path + ".ccshift-bak"); err == nil && !a.ours(statusLineCommand(bak.StatusLine())) {
					prev = bak.StatusLine()
				}
			}
			if isNull(prev) {
				prev = nil
			}
			f.SetStatusLine(prev)
			st.PreviousStatusLine = nil
			st.Installed = false
		}
		after, _ := f.Bytes()
		if bytes.Equal(before, after) {
			fmt.Fprintln(a.Out, "Nothing to remove.")
			return nil
		}
		if err := f.Save(); err != nil {
			return err
		}
		if err := a.Store.SetInitState(st); err != nil {
			return err
		}
		fmt.Fprintf(a.Out, "Removed ccshift's hooks and statusline from %s.\n", f.Path)
		return nil
	}

	if _, err := f.RemoveHooks(a.ours); err != nil {
		return err
	}
	for _, h := range hookEvents {
		if _, err := f.AddHook(h.event, a.command("hook "+h.arg), hookTimeoutSeconds); err != nil {
			return err
		}
	}
	cur := f.StatusLine()
	if isNull(cur) {
		cur = nil
	}
	if !a.ours(statusLineCommand(cur)) {
		st.PreviousStatusLine = cur
	}
	st.Installed = true
	sl := map[string]any{}
	if cur != nil {
		json.Unmarshal(cur, &sl)
	}
	if sl == nil {
		sl = map[string]any{}
	}
	sl["type"] = "command"
	sl["command"] = a.command("statusline")
	raw, _ := json.Marshal(sl)
	f.SetStatusLine(raw)

	after, _ := f.Bytes()
	if bytes.Equal(before, after) {
		fmt.Fprintf(a.Out, "Hooks and statusline are already set up in %s.\n", f.Path)
	} else {
		// Only a file without ccshift in it is worth keeping as the backup.
		if !hadOurs {
			if err := f.Backup(".ccshift-bak"); err != nil {
				return err
			}
		}
		if err := a.Store.SetInitState(st); err != nil {
			return err
		}
		if err := f.Save(); err != nil {
			return err
		}
		fmt.Fprintf(a.Out, "Added autosave hooks and the statusline to %s.\n", f.Path)
		fmt.Fprintf(a.Out, "The previous file is at %s.ccshift-bak. New Claude sessions pick this up; running ones need a restart.\n", f.Path)
	}

	ad, err := a.adapter("")
	if err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "Terminal: %s (%s).\n", ad.Name(), tierNote(ad.Tier()))
	if c, ok := ad.(term.Checker); ok {
		if err := c.Check(ctx); err != nil {
			fmt.Fprintf(a.Out, "  %v\n", err)
		}
	}

	if !st.LegacyImported {
		n, err := a.importLegacy()
		if err != nil {
			return err
		}
		st.LegacyImported = true
		if err := a.Store.SetInitState(st); err != nil {
			return err
		}
		if n > 0 {
			fmt.Fprintf(a.Out, "Imported %d saves from the old claude-sessions scripts into workspace default. See them with: ccshift restore default --pick\n", n)
		}
	}
	return nil
}

func tierNote(t term.Tier) string {
	switch t {
	case term.Exact:
		return "tab order is read from the terminal"
	case term.LaunchOnly:
		return "tab order can't be read back, so ccshift keeps the order it last restored"
	default:
		return "one window per session"
	}
}

type legacySave struct {
	SavedAt  int64 `json:"saved_at"`
	Sessions []struct {
		SessionID string `json:"session_id"`
		CWD       string `json:"cwd"`
		Name      string `json:"name"`
	} `json:"sessions"`
}

// importLegacy copies saves made by the claude-sessions-* scripts into the default workspace's history.
func (a *App) importLegacy() (int, error) {
	files, _ := filepath.Glob(filepath.Join(a.ClaudeDir, "saved-sessions", "*.json"))
	var snaps []store.Snapshot
	for _, p := range files {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var ls legacySave
		if json.Unmarshal(b, &ls) != nil || len(ls.Sessions) == 0 {
			continue
		}
		snap := store.Snapshot{Workspace: config.DefaultWorkspace, SavedAt: time.UnixMilli(ls.SavedAt), Terminal: "gnome-terminal"}
		for i, s := range ls.Sessions {
			snap.Sessions = append(snap.Sessions, store.Entry{SessionID: s.SessionID, CWD: s.CWD, Name: s.Name, Position: i + 1})
		}
		snaps = append(snaps, snap)
	}
	if len(snaps) == 0 {
		return 0, nil
	}
	sort.Slice(snaps, func(i, j int) bool { return snaps[i].SavedAt.Before(snaps[j].SavedAt) })
	unlock, err := a.Store.Lock()
	if err != nil {
		return 0, err
	}
	defer unlock()
	for _, snap := range snaps {
		if err := a.Store.AddHistory(snap); err != nil {
			return 0, err
		}
	}
	return len(snaps), nil
}

func (a *App) cmdDoctor(ctx context.Context, args []string) error {
	if len(args) > 0 {
		return usageError{"doctor takes no arguments"}
	}
	problems := 0
	report := func(ok bool, what, fix string) {
		if ok {
			fmt.Fprintf(a.Out, "ok       %s\n", what)
			return
		}
		problems++
		fmt.Fprintf(a.Out, "problem  %s\n         %s\n", what, fix)
	}

	report(a.HasCommand(a.ClaudeBin), "Claude Code is installed", "the claude command was not found on your PATH; ccshift works on top of Claude Code")

	_, err := a.Claude.Live(ctx)
	report(err == nil, "Claude sessions can be read", fmt.Sprintf("claude agents --json failed and no session files were found: %v", err))

	unlock, err := a.Store.TryLock(time.Second)
	if err == nil {
		unlock()
	}
	report(err == nil, "state directory "+a.Store.Dir+" is writable", fmt.Sprint(err))

	_, err = os.Stat(a.Executable)
	report(err == nil, "ccshift binary is at "+a.Executable, "the binary the hooks would call is missing; reinstall and run ccshift init")

	f, err := settings.Load(a.settingsPath())
	if err != nil {
		report(false, "settings.json can be read", err.Error())
	} else {
		for _, h := range hookEvents {
			want := a.command("hook " + h.arg)
			found := false
			for _, c := range f.HookCommands(h.event) {
				found = found || c == want
			}
			report(found, h.event+" hook is installed", "run: ccshift init")
		}
		report(statusLineCommand(f.StatusLine()) == a.command("statusline"), "statusline is wired through ccshift", "run: ccshift init (without it the context warning falls back to an estimate after each turn)")
	}

	ad, err := a.adapter("")
	if err == nil {
		var cerr error
		if c, ok := ad.(term.Checker); ok {
			cerr = c.Check(ctx)
		}
		report(cerr == nil, fmt.Sprintf("terminal is %s (%s)", ad.Name(), tierNote(ad.Tier())), fmt.Sprint(cerr))
	}

	report(a.HasCommand(notifier()), notifier()+" is available for the context warning", "without it the warning only shows in ccshift ls")

	if problems > 0 {
		return fmt.Errorf("%d problems found", problems)
	}
	return nil
}
