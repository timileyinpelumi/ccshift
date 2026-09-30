package term

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type kitty struct{ x Exec }

func (k *kitty) Name() string { return "kitty" }
func (k *kitty) Tier() Tier   { return Exact }

func (k *kitty) Detect(env map[string]string, _ []string) bool { return env["KITTY_WINDOW_ID"] != "" }

func (k *kitty) TabOf(env map[string]string) (TabID, bool) {
	id := env["KITTY_WINDOW_ID"]
	return TabID(id), id != "" && sameInstance(k.x.Env, env, "KITTY_PID")
}

func (k *kitty) run(ctx context.Context, args ...string) ([]byte, error) {
	return k.x.Run(ctx, "kitten", append([]string{"@"}, args...)...)
}

type kittyOSWindow struct {
	ID   int `json:"id"`
	Tabs []struct {
		Windows []struct {
			ID int `json:"id"`
		} `json:"windows"`
	} `json:"tabs"`
}

func (k *kitty) List(ctx context.Context) ([]Tab, error) {
	out, err := k.run(ctx, "ls")
	if err != nil {
		return nil, err
	}
	var wins []kittyOSWindow
	if err := json.Unmarshal(out, &wins); err != nil {
		return nil, fmt.Errorf("kitty: reading ls output: %w", err)
	}
	var tabs []Tab
	for wi, w := range wins {
		for ti, t := range w.Tabs {
			for _, p := range t.Windows {
				tabs = append(tabs, Tab{ID: TabID(strconv.Itoa(p.ID)), Window: strconv.Itoa(w.ID), Label: fmt.Sprintf("%d.%d", wi+1, ti+1)})
			}
		}
	}
	return tabs, nil
}

func (k *kitty) OpenWindow(ctx context.Context, title string, ls []Launch) error {
	first := ""
	for i, l := range ls {
		args := []string{"launch"}
		if i == 0 {
			args = append(args, "--type=os-window", "--os-window-title="+title)
		} else {
			args = append(args, "--type=tab", "--match=window_id:"+first)
		}
		args = append(append(args, "--cwd="+l.CWD, "--tab-title="+l.Title, "--"), l.Argv...)
		out, err := k.run(ctx, args...)
		if err != nil {
			return err
		}
		if i == 0 {
			first = strings.TrimSpace(string(out))
			if first == "" {
				return fmt.Errorf("kitty: launch did not report a window id")
			}
		}
	}
	return nil
}

func (k *kitty) SetTitle(ctx context.Context, id TabID, title string) error {
	_, err := k.run(ctx, "set-tab-title", "--match=window_id:"+string(id), "--", title)
	return err
}

func (k *kitty) Focus(ctx context.Context, id TabID) error {
	_, err := k.run(ctx, "focus-window", "--match=id:"+string(id))
	return err
}

func (k *kitty) Check(ctx context.Context) error {
	if _, err := k.run(ctx, "ls"); err != nil {
		return fmt.Errorf("kitty remote control isn't reachable (%v). Add these to kitty.conf and restart kitty:\n    allow_remote_control socket-only\n    listen_on unix:/tmp/kitty-{kitty_pid}", err)
	}
	return nil
}

func (k *kitty) OpenTab(ctx context.Context, near TabID, l Launch) error {
	args := []string{"launch", "--type=tab", "--match=window_id:" + string(near), "--cwd=" + l.CWD, "--tab-title=" + l.Title, "--"}
	_, err := k.run(ctx, append(args, l.Argv...)...)
	return err
}

func (k *kitty) CloseTab(ctx context.Context, id TabID) error {
	// Only the session's window (pane): the tab may hold others.
	_, err := k.run(ctx, "close-window", "--match=id:"+string(id))
	return err
}
