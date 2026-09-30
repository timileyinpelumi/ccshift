package term

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type zellij struct{ x Exec }

func (z *zellij) Name() string { return "zellij" }
func (z *zellij) Tier() Tier   { return Exact }

func (z *zellij) Detect(env map[string]string, _ []string) bool { return env["ZELLIJ"] != "" }

func (z *zellij) TabOf(env map[string]string) (TabID, bool) {
	id := env["ZELLIJ_PANE_ID"]
	return TabID(id), id != "" && sameInstance(z.x.Env, env, "ZELLIJ_SESSION_NAME")
}

func (z *zellij) action(ctx context.Context, args ...string) ([]byte, error) {
	return z.x.Run(ctx, "zellij", append([]string{"action"}, args...)...)
}

type zellijPane struct {
	ID          int  `json:"id"`
	IsPlugin    bool `json:"is_plugin"`
	TabID       int  `json:"tab_id"`
	TabPosition int  `json:"tab_position"`
}

func (z *zellij) List(ctx context.Context) ([]Tab, error) {
	out, err := z.action(ctx, "list-panes", "--json")
	if err != nil {
		return nil, err
	}
	var ps []zellijPane
	if err := json.Unmarshal(out, &ps); err != nil {
		return nil, fmt.Errorf("zellij: reading list-panes output: %w", err)
	}
	sort.SliceStable(ps, func(i, j int) bool { return ps[i].TabPosition < ps[j].TabPosition })
	session := z.x.Env["ZELLIJ_SESSION_NAME"]
	var tabs []Tab
	for _, p := range ps {
		if !p.IsPlugin {
			tabs = append(tabs, Tab{ID: TabID(strconv.Itoa(p.ID)), Window: session, Label: strconv.Itoa(p.TabPosition + 1)})
		}
	}
	return tabs, nil
}

func kdlQuote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

func paneNode(l Launch) string {
	args := make([]string, len(l.Argv)-1)
	for i, a := range l.Argv[1:] {
		args[i] = kdlQuote(a)
	}
	return fmt.Sprintf("pane command=%s cwd=%s {\n    args %s\n  }", kdlQuote(l.Argv[0]), kdlQuote(l.CWD), strings.Join(args, " "))
}

func tabLayout(l Launch) string {
	return "layout {\n  " + paneNode(l) + "\n}\n"
}

func sessionLayout(ls []Launch) string {
	var b strings.Builder
	b.WriteString("layout {\n  default_tab_template {\n    pane size=1 borderless=true { plugin location=\"zellij:tab-bar\"; }\n    children\n    pane size=2 borderless=true { plugin location=\"zellij:status-bar\"; }\n  }\n")
	for _, l := range ls {
		fmt.Fprintf(&b, "  tab name=%s {\n  %s\n  }\n", kdlQuote(l.Title), paneNode(l))
	}
	b.WriteString("}\n")
	return b.String()
}

func writeTemp(pattern, content string) (string, error) {
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", err
	}
	defer f.Close()
	_, err = f.WriteString(content)
	return f.Name(), err
}

func (z *zellij) OpenWindow(ctx context.Context, title string, ls []Launch) error {
	if len(ls) == 0 {
		return nil
	}
	if z.x.Env["ZELLIJ"] == "" {
		// zellij reads this file when the user starts the session, so it has to outlive ccshift.
		// One file per workspace, overwritten each time.
		cache, err := os.UserCacheDir()
		if err != nil {
			return err
		}
		path := filepath.Join(cache, "ccshift", "zellij-"+title+".kdl")
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(sessionLayout(ls)), 0o600); err != nil {
			return err
		}
		fmt.Fprintf(z.x.Out, "Start the zellij session with: zellij --session %s --new-session-with-layout %s\n", title, path)
		return ErrPrinted
	}
	for _, l := range ls {
		path, err := writeTemp("ccshift-zellij-tab-*.kdl", tabLayout(l))
		if err != nil {
			return err
		}
		_, err = z.action(ctx, "new-tab", "--name="+l.Title, "--cwd", l.CWD, "--layout", path)
		os.Remove(path)
		if err != nil {
			return err
		}
	}
	return nil
}

func (z *zellij) tabIDOf(ctx context.Context, pane TabID) (string, error) {
	out, err := z.action(ctx, "list-panes", "--json")
	if err != nil {
		return "", err
	}
	var ps []zellijPane
	if err := json.Unmarshal(out, &ps); err != nil {
		return "", err
	}
	for _, p := range ps {
		if !p.IsPlugin && strconv.Itoa(p.ID) == string(pane) {
			return strconv.Itoa(p.TabID), nil
		}
	}
	return "", fmt.Errorf("zellij: pane %s not found", pane)
}

func (z *zellij) SetTitle(ctx context.Context, id TabID, title string) error {
	tab, err := z.tabIDOf(ctx, id)
	if err != nil {
		return err
	}
	_, err = z.action(ctx, "rename-tab-by-id", tab, "--", title)
	return err
}

func (z *zellij) Focus(ctx context.Context, id TabID) error {
	tab, err := z.tabIDOf(ctx, id)
	if err != nil {
		return err
	}
	_, err = z.action(ctx, "go-to-tab-by-id", tab)
	return err
}

func (z *zellij) Check(ctx context.Context) error {
	out, err := z.x.Run(ctx, "zellij", "--version")
	if err != nil {
		return err
	}
	var major, minor int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "zellij %d.%d", &major, &minor); err != nil {
		return fmt.Errorf("can't read the zellij version from %q", strings.TrimSpace(string(out)))
	}
	if major == 0 && minor < 44 {
		return fmt.Errorf("zellij %d.%d is too old: ccshift needs 0.44 or later to read tab order", major, minor)
	}
	return nil
}

// OpenTab adds a tab to the current zellij session. zellij has no option to place it next to another.
func (z *zellij) OpenTab(ctx context.Context, _ TabID, l Launch) error {
	if z.x.Env["ZELLIJ"] == "" {
		return fmt.Errorf("not inside a zellij session")
	}
	return z.OpenWindow(ctx, "", []Launch{l})
}
