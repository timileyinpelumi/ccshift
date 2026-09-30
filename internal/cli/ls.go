package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"text/tabwriter"
	"time"
)

type lsRow struct {
	Pos       int    `json:"position"`
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
			SessionID: it.Session.ID, CWD: it.Session.CWD,
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
	tw := tabwriter.NewWriter(a.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "#\tNAME\tWORKSPACE\tBRANCH\tSTATUS\tCTX\tAGE\tTAB")
	for _, r := range rows {
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", r.Pos, r.Name, r.Workspace, dash(r.Branch), dash(r.Status), r.Context, r.Age, r.Tab)
	}
	tw.Flush()
	if len(bg) > 0 {
		fmt.Fprintln(a.Out, "\nBackground:")
		tw = tabwriter.NewWriter(a.Out, 0, 0, 2, ' ', 0)
		for _, r := range bg {
			fmt.Fprintf(tw, "  %s\t%s\t%s\n", r.Name, dash(r.Status), r.Age)
		}
		tw.Flush()
	}
	return nil
}
