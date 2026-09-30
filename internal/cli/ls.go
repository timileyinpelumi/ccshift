package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/timileyinpelumi/ccshift/internal/target"
	"github.com/timileyinpelumi/ccshift/internal/ui"
)

type lsRow struct {
	Pos       int    `json:"position"`
	Code      string `json:"code"`
	Name      string `json:"name"`
	Workspace string `json:"workspace,omitempty"`
	Branch    string `json:"branch,omitempty"`
	Status    string `json:"status"`
	Context   string `json:"context"`
	Age       string `json:"age"`
	Tab       string `json:"tab"`
	SessionID string `json:"session_id"`
	CWD       string `json:"cwd"`
}

func age(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "<1m"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func (a *App) cmdLs(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("ls", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	all := fs.Bool("all", false, "include background sessions")
	asJSON := fs.Bool("json", false, "print JSON")
	termName := fs.String("terminal", "", "terminal adapter to use instead of detecting it")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return usageError{err.Error()}
	}
	if len(pos) > 0 {
		return usageError{"ls takes no arguments"}
	}
	v, err := a.view(ctx, *termName)
	if err != nil {
		return err
	}
	now := a.Now()
	usage, err := a.Store.Context()
	if err != nil {
		fmt.Fprintf(a.Err, "ccshift: ignoring saved context usage (%v)\n", err)
	}
	rows := []lsRow{}
	for i, it := range v.order {
		r := lsRow{
			Pos: i + 1, Name: displayName(it.Session), Workspace: it.Workspace, Branch: it.Branch,
			Status: it.Session.Status, Age: age(now.Sub(it.Session.StartedAt)), Tab: "-",
			SessionID: it.Session.ID, CWD: it.Session.CWD, Code: target.Code(it.Session.ID),
		}
		e, known := usage[it.Session.ID]
		r.Context = contextCell(e, known, a.Config.WarnThresholds)
		if it.Matched {
			r.Tab = fmt.Sprintf("%s:%d", v.adapter.Name(), it.TabIndex+1)
			if it.TabLabel != "" {
				r.Tab = v.adapter.Name() + ":" + it.TabLabel
			}
		}
		rows = append(rows, r)
	}
	bg := []lsRow{}
	if *all {
		for _, s := range v.live {
			if s.Kind == "background" {
				bg = append(bg, lsRow{Name: displayName(s), Status: s.Status, Age: age(now.Sub(s.StartedAt)), SessionID: s.ID, CWD: s.CWD, Tab: "-"})
			}
		}
	}
	if *asJSON {
		enc := json.NewEncoder(a.Out)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{"terminal": v.adapter.Name(), "sessions": rows, "background": bg})
	}
	if len(rows) == 0 && len(bg) == 0 {
		fmt.Fprintln(a.Out, "No Claude sessions running.")
		return nil
	}
	u := a.ui()
	t := u.Table("#", "CODE", "NAME", "WORKSPACE", "BRANCH", "STATUS", "CTX", "AGE", "TAB")
	for _, r := range rows {
		t.Row(ui.Cell{Text: strconv.Itoa(r.Pos), Style: ui.Dim}, ui.Cell{Text: r.Code, Style: ui.Code}, ui.Cell{Text: r.Name, Style: ui.Bold},
			ui.Cell{Text: r.Workspace}, ui.Cell{Text: dash(r.Branch), Style: ui.Dim}, ui.Cell{Text: dash(r.Status), Style: statusStyle(r.Status)},
			ui.Cell{Text: r.Context, Style: contextStyle(r.Context)}, ui.Cell{Text: r.Age, Style: ui.Dim}, ui.Cell{Text: r.Tab, Style: ui.Dim})
	}
	t.Flush()
	if len(bg) > 0 {
		fmt.Fprintln(a.Out, "\n"+u.Paint(ui.Dim, "Background:"))
		t = u.Table()
		for _, r := range bg {
			t.Row(ui.Cell{Text: "  " + r.Name}, ui.Cell{Text: dash(r.Status), Style: statusStyle(r.Status)}, ui.Cell{Text: r.Age, Style: ui.Dim})
		}
		t.Flush()
	}
	return nil
}

func statusStyle(status string) ui.Style {
	switch status {
	case "busy":
		return ui.Amber
	case "waiting":
		return ui.Cyan
	case "", "idle":
		return ui.Dim
	}
	return ui.None
}

func contextStyle(cell string) ui.Style {
	switch {
	case strings.HasSuffix(cell, "!"):
		return ui.Amber
	case cell == "-":
		return ui.Dim
	}
	return ui.Green
}
