package cli

import (
	"context"
	"flag"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/timileyinpelumi/ccshift/internal/config"
)

func (a *App) cmdWs(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("ws", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	pos, err := parseArgs(fs, args)
	if err != nil {
		return usageError{err.Error()}
	}
	if len(pos) > 0 {
		return usageError{"ws takes no arguments"}
	}
	saved, err := a.Store.Workspaces()
	if err != nil {
		return err
	}
	v, err := a.view(ctx, "")
	if err != nil {
		return err
	}
	names := map[string]bool{}
	for _, ws := range saved {
		names[ws] = true
	}
	for ws := range a.Config.Workspaces {
		names[ws] = true
	}
	for ws := range v.groups {
		names[ws] = true
	}
	list := make([]string, 0, len(names))
	for ws := range names {
		list = append(list, ws)
	}
	sort.Slice(list, func(i, j int) bool {
		if (list[i] == config.DefaultWorkspace) != (list[j] == config.DefaultWorkspace) {
			return list[j] == config.DefaultWorkspace
		}
		return list[i] < list[j]
	})
	tw := tabwriter.NewWriter(a.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "WORKSPACE\tSAVED AT\tSAVED\tRUNNING\tPATHS")
	for _, ws := range list {
		at, count := "-", 0
		if snap, ok, err := a.Store.Latest(ws); err != nil {
			at = "unreadable"
		} else if ok {
			at, count = snap.SavedAt.Local().Format("2006-01-02 15:04"), len(snap.Sessions)
		}
		paths := strings.Join(a.Config.Workspaces[ws], ",")
		if paths == "" {
			paths = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%s\n", ws, at, count, len(v.groups[ws]), paths)
	}
	return tw.Flush()
}
