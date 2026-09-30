package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/timileyinpelumi/ccshift/internal/claude"
	"github.com/timileyinpelumi/ccshift/internal/handoff"
	"github.com/timileyinpelumi/ccshift/internal/layout"
	"github.com/timileyinpelumi/ccshift/internal/store"
	"github.com/timileyinpelumi/ccshift/internal/target"
	"github.com/timileyinpelumi/ccshift/internal/term"
)

const (
	extractLimit = 150000
	briefBudget  = 5 * time.Minute
	lineageFile  = "lineage.json"
	handoffKeep  = 30 * 24 * time.Hour
)

// handoffSource is the session being handed off: a running one, or one that is only in a save.
type handoffSource struct {
	id, name, cwd, workspace string
	live                     bool
	tab                      term.TabID
	matched                  bool
}

func (a *App) handoffSource(spec string, v *view) (handoffSource, error) {
	it, err := target.Resolve(spec, v.order, a.AncestorPIDs(a.SelfPID))
	if err == nil {
		return handoffSource{
			id: it.Session.ID, name: it.Session.Name, cwd: it.Session.CWD, workspace: it.Workspace,
			live: true, tab: it.Tab, matched: it.Matched,
		}, nil
	}
	// Only when nothing running fits: an ambiguous name must not quietly pick a saved session.
	if !errors.Is(err, target.ErrNoMatch) {
		return handoffSource{}, err
	}
	// A session that crashed or was closed can still be handed off from its transcript.
	live := map[string]bool{}
	for _, s := range v.live {
		live[s.ID] = true
	}
	wss, _ := a.Store.Workspaces()
	found := map[string]handoffSource{}
	for _, ws := range wss {
		snap, ok, serr := a.Store.Latest(ws)
		if serr != nil || !ok {
			continue
		}
		for _, e := range snap.Sessions {
			if live[e.SessionID] {
				continue
			}
			if strings.EqualFold(e.Name, spec) || (len(spec) >= 4 && strings.HasPrefix(e.SessionID, spec)) {
				if _, seen := found[e.SessionID]; !seen {
					found[e.SessionID] = handoffSource{id: e.SessionID, name: layout.Title(e), cwd: e.CWD, workspace: ws}
				}
			}
		}
	}
	switch len(found) {
	case 0:
		return handoffSource{}, err
	case 1:
		for _, src := range found {
			return src, nil
		}
	}
	return handoffSource{}, fmt.Errorf("%q matches more than one saved session; use a session id", spec)
}

// superseded lists sessions that have been handed off. They stay out of every save.
func (a *App) superseded() map[string]string {
	lineage := map[string]string{}
	a.Store.Load(lineageFile, &lineage)
	return lineage
}

func (a *App) cmdHandoff(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("handoff", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	edit := fs.Bool("edit", false, "open the brief in $EDITOR before starting the new session")
	closeOld := fs.Bool("close-old", false, "close the old session's tab once the new one is open")
	noLaunch := fs.Bool("no-launch", false, "write the brief and stop")
	reuse := fs.Bool("reuse", false, "use the brief already written for this session")
	model := fs.String("model", "", "model that writes the brief")
	termName := fs.String("terminal", "", "terminal adapter to use instead of detecting it")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return usageError{err.Error()}
	}
	if len(pos) > 1 {
		return usageError{"usage: ccshift handoff [name|number|.] [--edit] [--close-old] [--no-launch] [--reuse] [--model m]"}
	}
	spec := "."
	if len(pos) == 1 {
		spec = pos[0]
	}
	v, err := a.view(ctx, *termName)
	if err != nil {
		return err
	}
	src, err := a.handoffSource(spec, v)
	if err != nil {
		return err
	}
	a.Store.PruneOlder("handoffs", handoffKeep, a.Now())
	briefPath := filepath.Join(a.Store.Dir, "handoffs", src.id+".md")
	if *reuse {
		if _, err := os.Stat(briefPath); err != nil {
			return fmt.Errorf("there is no brief for %s yet; run ccshift handoff without --reuse", src.name)
		}
	} else if briefPath, err = a.writeBrief(ctx, src, *model); err != nil {
		return err
	}
	if *edit {
		if err := a.Editor(briefPath); err != nil {
			return fmt.Errorf("editor: %w", err)
		}
	}
	if *noLaunch {
		fmt.Fprintf(a.Out, "The brief is at %s.\nWhen you are ready: ccshift handoff %s --reuse\n", briefPath, term.ShellJoin([]string{src.name}))
		return nil
	}

	newID := a.NewID()
	taken := map[string]bool{}
	for _, it := range v.order {
		taken[it.Session.Name] = true
	}
	if wss, err := a.Store.Workspaces(); err == nil {
		for _, ws := range wss {
			if snap, ok, err := a.Store.Latest(ws); err == nil && ok {
				for _, e := range snap.Sessions {
					taken[e.Name] = true
				}
			}
		}
	}
	newName := handoff.NextName(src.name, func(n string) bool { return taken[n] })
	argv := []string{a.ClaudeBin, "--session-id", newID, "--name=" + newName}
	// The new session runs on the model the old one was using, not the user's default.
	if transcript, ok := a.Claude.TranscriptPath(src.id); ok {
		if m := claude.LastModel(transcript); m != "" {
			argv = append(argv, "--model", m)
		}
	}
	launch := term.Launch{CWD: src.cwd, Title: newName, Argv: append(argv, "Read "+briefPath+" and continue from it.")}
	opener, canPlace := v.adapter.(term.TabOpener)
	if canPlace && src.matched {
		err = opener.OpenTab(ctx, src.tab, isolated([]term.Launch{launch})[0])
	} else {
		err = v.adapter.OpenWindow(ctx, src.workspace, isolated([]term.Launch{launch}))
	}
	if errors.Is(err, term.ErrPrinted) {
		// Nothing is running yet, so the save is left alone; autosave picks the new session up once it starts.
		fmt.Fprintf(a.Out, "The brief is at %s. Once the new session is running, close the old one.\n", briefPath)
		return nil
	}
	if err != nil {
		fmt.Fprintf(a.Out, "The brief is at %s. Start the new session yourself with:\n", briefPath)
		term.PrintCommands(a.Out, []term.Launch{launch})
		return fmt.Errorf("couldn't open a tab in %s: %w", v.adapter.Name(), err)
	}

	now := a.Now()
	if err := a.locked(func() error {
		lineage := map[string]string{}
		a.Store.Load(lineageFile, &lineage)
		lineage[src.id] = newID
		if err := a.Store.Put(lineageFile, lineage); err != nil {
			return err
		}
		if order, err := a.Store.Order(); err == nil {
			for ws, ids := range order {
				for i, id := range ids {
					if id == src.id {
						ids[i] = newID
						a.Store.SetOrder(ws, ids)
					}
				}
			}
		}
		wss, err := a.Store.Workspaces()
		if err != nil {
			return err
		}
		for _, ws := range wss {
			snap, ok, err := a.Store.Latest(ws)
			if err != nil || !ok {
				continue
			}
			for i, e := range snap.Sessions {
				if e.SessionID != src.id {
					continue
				}
				// The new session takes the old one's place.
				snap.Sessions[i] = store.Entry{SessionID: newID, CWD: src.cwd, Name: newName, Position: e.Position, LastSeen: now.Unix()}
				if err := a.Store.SaveLatest(snap); err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		return err
	}

	fmt.Fprintf(a.Out, "Handed %s off to %s. The brief is at %s.\n", src.name, newName, briefPath)
	if !src.live {
		return nil
	}
	left := "The old session is still open and is left out of saves from now on."
	closer, canClose := v.adapter.(term.TabCloser)
	switch {
	case !*closeOld:
		fmt.Fprintln(a.Out, left, "Close it when you are done with it.")
	case !canClose:
		fmt.Fprintf(a.Out, "%s can't close a tab from outside. %s\n", v.adapter.Name(), left)
	case !src.matched:
		fmt.Fprintf(a.Out, "no tab was found for the old session in %s. %s\n", v.adapter.Name(), left)
	default:
		if err := closer.CloseTab(ctx, src.tab); err != nil {
			return fmt.Errorf("couldn't close the old tab: %w", err)
		}
	}
	return nil
}

// writeBrief builds the extract, has a fresh Claude write the brief from it, and returns the brief's path.
func (a *App) writeBrief(ctx context.Context, src handoffSource, model string) (string, error) {
	transcript, ok := a.Claude.TranscriptPath(src.id)
	if !ok {
		return "", fmt.Errorf("no transcript found for %s, so there is nothing to hand off", src.name)
	}
	conv, err := claude.ReadConversation(transcript)
	if err != nil {
		return "", fmt.Errorf("reading the transcript of %s: %w", src.name, err)
	}
	if len(conv.Turns) == 0 {
		return "", fmt.Errorf("%s has no conversation yet, so there is nothing to hand off", src.name)
	}
	extract := handoff.Extract(conv, handoff.Info{Name: src.name, CWD: src.cwd, Git: a.GitSummary(ctx, src.cwd)}, extractLimit)
	extractPath, err := a.Store.WriteText(filepath.Join("handoffs", src.id+".extract.md"), extract)
	if err != nil {
		return "", err
	}
	if model == "" {
		model = a.Config.BriefModel
	}
	fmt.Fprintf(a.Out, "Writing the brief for %s with %s. This can take a minute or two.\n", src.name, model)
	bctx, cancel := context.WithTimeout(ctx, briefBudget)
	defer cancel()
	brief, err := a.RunClaude(bctx, src.cwd, []string{"-p", "--model", model, "--tools", "Read,Grep,Glob", "--no-session-persistence"}, handoff.Prompt(extract))
	if err == nil && strings.TrimSpace(brief) == "" {
		err = errors.New("the brief came back empty")
	}
	if err != nil {
		return "", fmt.Errorf("the brief could not be written: %w\nThe extract it would have been written from is at %s. To continue from that by hand:\n  cd %s && claude %s",
			err, extractPath, term.ShellJoin([]string{src.cwd}), term.ShellJoin([]string{"Read " + extractPath + " and continue from it."}))
	}
	briefPath, err := a.Store.WriteText(filepath.Join("handoffs", src.id+".md"), strings.TrimSpace(brief)+"\n")
	if err != nil {
		return "", err
	}
	os.Remove(extractPath)
	return briefPath, nil
}
