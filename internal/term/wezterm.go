package term

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type wezterm struct{ x Exec }

func (w *wezterm) Name() string { return "wezterm" }
func (w *wezterm) Tier() Tier   { return Exact }

func (w *wezterm) Detect(env map[string]string, _ []string) bool { return env["WEZTERM_PANE"] != "" }

func (w *wezterm) TabOf(env map[string]string) (TabID, bool) {
	id := env["WEZTERM_PANE"]
	return TabID(id), id != "" && sameInstance(w.x.Env, env, "WEZTERM_UNIX_SOCKET")
}

func (w *wezterm) cli(ctx context.Context, args ...string) ([]byte, error) {
	return w.x.Run(ctx, "wezterm", append([]string{"cli"}, args...)...)
}

type weztermPane struct {
	WindowID int `json:"window_id"`
	TabID    int `json:"tab_id"`
	PaneID   int `json:"pane_id"`
}

func (w *wezterm) panes(ctx context.Context) ([]weztermPane, error) {
	out, err := w.cli(ctx, "list", "--format", "json")
	if err != nil {
		return nil, err
	}
	var ps []weztermPane
	if err := json.Unmarshal(out, &ps); err != nil {
		return nil, fmt.Errorf("wezterm: reading list output: %w", err)
	}
	return ps, nil
}

func (w *wezterm) List(ctx context.Context) ([]Tab, error) {
	ps, err := w.panes(ctx)
	if err != nil {
		return nil, err
	}
	tabs := make([]Tab, len(ps))
	winNo := map[int]int{}
	tabNo := map[int]map[int]int{}
	for i, p := range ps {
		if _, ok := winNo[p.WindowID]; !ok {
			winNo[p.WindowID] = len(winNo) + 1
			tabNo[p.WindowID] = map[int]int{}
		}
		if _, ok := tabNo[p.WindowID][p.TabID]; !ok {
			tabNo[p.WindowID][p.TabID] = len(tabNo[p.WindowID]) + 1
		}
		tabs[i] = Tab{
			ID: TabID(strconv.Itoa(p.PaneID)), Window: strconv.Itoa(p.WindowID),
			Label: fmt.Sprintf("%d.%d", winNo[p.WindowID], tabNo[p.WindowID][p.TabID]),
		}
	}
	return tabs, nil
}

func (w *wezterm) OpenWindow(ctx context.Context, _ string, ls []Launch) error {
	window := ""
	for i, l := range ls {
		args := []string{"spawn"}
		if i == 0 {
			args = append(args, "--new-window")
		} else {
			args = append(args, "--window-id", window)
		}
		out, err := w.cli(ctx, append(append(args, "--cwd", l.CWD, "--"), l.Argv...)...)
		if err != nil {
			return err
		}
		pane := strings.TrimSpace(string(out))
		w.SetTitle(ctx, TabID(pane), l.Title)
		if i == 0 {
			if window, err = w.windowOf(ctx, pane); err != nil {
				return err
			}
		}
	}
	return nil
}

func (w *wezterm) windowOf(ctx context.Context, pane string) (string, error) {
	ps, err := w.panes(ctx)
	if err != nil {
		return "", err
	}
	for _, p := range ps {
		if strconv.Itoa(p.PaneID) == pane {
			return strconv.Itoa(p.WindowID), nil
		}
	}
	return "", fmt.Errorf("wezterm: new pane %s not found", pane)
}

func (w *wezterm) SetTitle(ctx context.Context, id TabID, title string) error {
	_, err := w.cli(ctx, "set-tab-title", "--pane-id", string(id), "--", title)
	return err
}

func (w *wezterm) Focus(ctx context.Context, id TabID) error {
	ps, err := w.panes(ctx)
	if err != nil {
		return err
	}
	for _, p := range ps {
		if strconv.Itoa(p.PaneID) != string(id) {
			continue
		}
		// activate-pane only picks the pane inside its tab, so switch the tab first.
		if _, err := w.cli(ctx, "activate-tab", "--tab-id", strconv.Itoa(p.TabID)); err != nil {
			return err
		}
		_, err := w.cli(ctx, "activate-pane", "--pane-id", string(id))
		return err
	}
	return fmt.Errorf("wezterm: pane %s not found", id)
}

func (w *wezterm) OpenTab(ctx context.Context, near TabID, l Launch) error {
	out, err := w.cli(ctx, append([]string{"spawn", "--pane-id", string(near), "--cwd", l.CWD, "--"}, l.Argv...)...)
	if err != nil {
		return err
	}
	// The session is running by now; a title that could not be set is not a failed open.
	w.SetTitle(ctx, TabID(strings.TrimSpace(string(out))), l.Title)
	return nil
}

func (w *wezterm) CloseTab(ctx context.Context, id TabID) error {
	_, err := w.cli(ctx, "kill-pane", "--pane-id", string(id))
	return err
}
