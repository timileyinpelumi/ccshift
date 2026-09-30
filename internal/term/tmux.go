package term

import (
	"context"
	"fmt"
	"strings"
)

type tmux struct {
	x      Exec
	socket string // tests use a private server without the user's tmux.conf
}

func (t *tmux) Name() string { return "tmux" }
func (t *tmux) Tier() Tier   { return Exact }

func (t *tmux) Detect(env map[string]string, _ []string) bool { return env["TMUX"] != "" }

func (t *tmux) TabOf(env map[string]string) (TabID, bool) {
	p := env["TMUX_PANE"]
	own, _, _ := strings.Cut(t.x.Env["TMUX"], ",")
	theirs, _, _ := strings.Cut(env["TMUX"], ",")
	return TabID(p), p != "" && (own == "" || own == theirs)
}

func (t *tmux) run(ctx context.Context, args ...string) ([]byte, error) {
	if t.socket != "" {
		args = append([]string{"-L", t.socket, "-f", "/dev/null"}, args...)
	}
	return t.x.Run(ctx, "tmux", args...)
}

func (t *tmux) List(ctx context.Context) ([]Tab, error) {
	out, err := t.run(ctx, "list-panes", "-a", "-F", "#{session_name}\t#{window_index}\t#{pane_index}\t#{pane_id}")
	if err != nil {
		return nil, err
	}
	var tabs []Tab
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Split(line, "\t")
		if len(f) == 4 {
			tabs = append(tabs, Tab{ID: TabID(f[3]), Window: f[0], Label: f[0] + ":" + f[1]})
		}
	}
	return tabs, nil
}

func (t *tmux) OpenWindow(ctx context.Context, title string, ls []Launch) error {
	if len(ls) == 0 {
		return nil
	}
	name := strings.NewReplacer(".", "-", ":", "-").Replace(title)
	// A session with the workspace's name gets the new windows, so a second restore fills in what is missing.
	_, err := t.run(ctx, "has-session", "-t", "="+name)
	exists := err == nil
	for i, l := range ls {
		var args []string
		if i == 0 && !exists {
			args = []string{"new-session", "-d", "-s", name, "-n", l.Title, "-c", l.CWD, "--"}
		} else {
			args = []string{"new-window", "-t", name + ":", "-n", l.Title, "-c", l.CWD, "--"}
		}
		if _, err := t.run(ctx, append(args, l.Argv...)...); err != nil {
			return err
		}
	}
	if t.x.Env["TMUX"] != "" {
		_, err := t.run(ctx, "switch-client", "-t", "="+name)
		return err
	}
	fmt.Fprintf(t.x.Out, "Tabs are in tmux session %s. Attach with: tmux attach -t %s\n", name, name)
	return nil
}

func (t *tmux) SetTitle(ctx context.Context, id TabID, title string) error {
	_, err := t.run(ctx, "rename-window", "-t", string(id), "--", title)
	return err
}

func (t *tmux) Focus(ctx context.Context, id TabID) error {
	if _, err := t.run(ctx, "select-window", "-t", string(id)); err != nil {
		return err
	}
	if _, err := t.run(ctx, "select-pane", "-t", string(id)); err != nil {
		return err
	}
	if t.x.Env["TMUX"] != "" {
		_, err := t.run(ctx, "switch-client", "-t", string(id))
		return err
	}
	return nil
}

// windowOf turns a pane id into its window id: window commands do not take a pane as target.
func (t *tmux) windowOf(ctx context.Context, pane TabID) (string, error) {
	out, err := t.run(ctx, "display-message", "-p", "-t", string(pane), "#{window_id}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (t *tmux) OpenTab(ctx context.Context, near TabID, l Launch) error {
	win, err := t.windowOf(ctx, near)
	if err != nil {
		return err
	}
	_, err = t.run(ctx, append([]string{"new-window", "-a", "-t", win, "-n", l.Title, "-c", l.CWD, "--"}, l.Argv...)...)
	return err
}

func (t *tmux) CloseTab(ctx context.Context, id TabID) error {
	// Only the session's pane: the window may hold other panes the user still wants.
	_, err := t.run(ctx, "kill-pane", "-t", string(id))
	return err
}
