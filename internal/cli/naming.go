package cli

import (
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/timileyinpelumi/ccshift/internal/layout"
	"github.com/timileyinpelumi/ccshift/internal/names"
	"github.com/timileyinpelumi/ccshift/internal/target"
	"github.com/timileyinpelumi/ccshift/internal/term"
	"github.com/timileyinpelumi/ccshift/internal/ui"
)

const (
	namesFile  = "names.json"
	titlesFile = "titles.json"
	// An entry ccshift wrote for a session that is not listed yet (just started, or being restored)
	// is kept this long before it can be pruned.
	nameGrace = 10 * time.Minute
)

// namePatch is what one look at the sessions wants changed in a name entry. Patches are applied to
// the file as it is when the lock is held, so a slower reader cannot undo a newer rename.
type namePatch struct {
	title        *string
	base         *string
	num          *int
	clearUser    string // clear User only if it still has this value
	clearApplied string
}

func emptyEntry(e names.Entry) bool {
	return e.User == "" && e.Applied == "" && e.Title == "" && e.Base == "" && e.Num == 0
}

func (a *App) loadNames() map[string]names.Entry {
	m := map[string]names.Entry{}
	if err := a.Store.Load(namesFile, &m); err != nil {
		a.errorf("ignoring saved names (%v)", err)
		return map[string]names.Entry{}
	}
	return m
}

// updateNames reads names.json, lets fn change it, and writes it back. The caller holds the store lock.
func (a *App) updateNames(fn func(m map[string]names.Entry)) error {
	m := a.loadNames()
	fn(m)
	for id, e := range m {
		if emptyEntry(e) {
			delete(m, id)
		}
	}
	return a.Store.Put(namesFile, m)
}

// saveNames writes what a view learned about names. The caller holds the store lock.
func (a *App) saveNames(v *view) error {
	if len(v.patches) == 0 {
		return nil
	}
	return a.updateNames(func(m map[string]names.Entry) {
		for id, p := range v.patches {
			e := m[id]
			if p.title != nil {
				e.Title = *p.title
			}
			if p.base != nil {
				e.Base = *p.base
			}
			if p.num != nil {
				e.Num = *p.num
			}
			if p.clearUser != "" && e.User == p.clearUser {
				e.User, e.ClaudeAtSet = "", ""
			}
			if p.clearApplied != "" && e.Applied == p.clearApplied {
				e.Applied = ""
			}
			m[id] = e
		}
	})
}

type gitResult struct {
	git names.Git
	err error
}

func (a *App) gitAll(ctx context.Context, cwds map[string]bool) map[string]gitResult {
	out := map[string]gitResult{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for cwd := range cwds {
		wg.Add(1)
		go func() {
			defer wg.Done()
			g, err := a.Git(ctx, cwd)
			mu.Lock()
			out[cwd] = gitResult{g, err}
			mu.Unlock()
		}()
	}
	wg.Wait()
	return out
}

// applyNames replaces each session's name with the one ccshift shows for it.
func (a *App) applyNames(ctx context.Context, groups map[string][]layout.Item) (map[string]names.Entry, map[string]namePatch) {
	entries := a.loadNames()
	patches := map[string]namePatch{}
	var all []*layout.Item
	cwds := map[string]bool{}
	for ws := range groups {
		for i := range groups[ws] {
			all = append(all, &groups[ws][i])
			cwds[groups[ws][i].Session.CWD] = true
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		if !all[i].Session.StartedAt.Equal(all[j].Session.StartedAt) {
			return all[i].Session.StartedAt.Before(all[j].Session.StartedAt)
		}
		return all[i].Session.ID < all[j].Session.ID
	})
	gits := a.gitAll(ctx, cwds)
	rules := a.Config.NameRules()

	taken := map[string]bool{}
	var genIdx []int
	var bases []string
	var prev []names.Entry
	for i, it := range all {
		s := it.Session
		entry := entries[s.ID]
		it.ClaudeName = s.Name
		g := gits[s.CWD]
		if g.err == nil {
			it.Branch = g.git.Branch
		}
		name, gen, keep := names.Resolve(s.Name, s.CWD, entry)
		p := patches[s.ID]
		if entry.User != "" && keep.User == "" {
			p.clearUser = entry.User
		}
		if entry.Applied != "" && keep.Applied == "" {
			p.clearApplied = entry.Applied
		}
		it.Generated = gen
		if !gen {
			it.Session.Name = name
			taken[name] = true
			if p != (namePatch{}) {
				patches[s.ID] = p
			}
			entries[s.ID] = keep
			continue
		}
		var base string
		switch {
		// A git timeout, a missing directory or a detached HEAD says nothing new about the name.
		case (g.err != nil || g.git.Detached) && entry.Base != "":
			base = entry.Base
		case g.err != nil:
			base = filepath.Base(s.CWD)
		default:
			title := entry.Title
			if names.NeedsTitle(g.git, rules) {
				// The title is cached: a long transcript's tail often has no title record in it.
				if t := a.AITitle(s.ID); t != "" && t != title {
					title = t
					p.title = &t
					keep.Title = t
				}
			}
			base = names.Generate(g.git, title, rules)
		}
		if base != entry.Base {
			b := base
			p.base = &b
		}
		patches[s.ID] = p
		entries[s.ID] = keep
		genIdx = append(genIdx, i)
		bases = append(bases, base)
		prev = append(prev, entry)
	}
	nums := names.Number(bases, prev, taken)
	for k, i := range genIdx {
		id := all[i].Session.ID
		all[i].Session.Name = names.Numbered(bases[k], nums[k])
		e := entries[id]
		if nums[k] != prev[k].Num {
			n := nums[k]
			p := patches[id]
			p.num = &n
			patches[id] = p
		}
		e.Base, e.Num = bases[k], nums[k]
		entries[id] = e
	}
	for id, p := range patches {
		if p == (namePatch{}) {
			delete(patches, id)
		}
	}
	return entries, patches
}

type titled struct {
	Tab  string `json:"tab"`
	Name string `json:"name"`
}

// syncTitles sets the tab title of each session whose name or tab changed since it was last set.
// It runs terminal commands, so callers must not hold the store lock; titles.json is only a cache.
func (a *App) syncTitles(ctx context.Context, v *view) {
	if !a.Config.SyncTabTitles || v.adapter.Tier() != term.Exact || ctx.Err() != nil {
		return
	}
	titles := map[string]titled{}
	a.Store.Load(titlesFile, &titles)
	// A tab with two sessions in it (split panes) has no single right title.
	shared := map[string]int{}
	for _, it := range v.order {
		if it.Matched && it.TabLabel != "" {
			shared[it.TabLabel]++
		}
	}
	changed := false
	live := map[string]bool{}
	for _, it := range v.order {
		live[it.Session.ID] = true
		want := titled{Tab: string(it.Tab), Name: it.Session.Name}
		if !it.Matched || shared[it.TabLabel] > 1 || titles[it.Session.ID] == want {
			continue
		}
		if err := v.adapter.SetTitle(ctx, it.Tab, it.Session.Name); err != nil {
			a.Store.Log("tab title for %s: %v", it.Session.Name, err)
			continue
		}
		titles[it.Session.ID] = want
		changed = true
	}
	for id := range titles {
		if !live[id] {
			delete(titles, id)
			changed = true
		}
	}
	if changed {
		a.Store.Put(titlesFile, titles)
	}
}

func cleanName(words []string) string {
	return strings.Join(strings.Fields(strings.Join(words, " ")), " ")
}

func (a *App) cmdRename(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("rename", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	edit := fs.Bool("edit", false, "edit every session's name in $EDITOR")
	reset := fs.Bool("reset", false, "drop the name set with ccshift rename")
	termName := fs.String("terminal", "", "terminal adapter to use instead of detecting it")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return usageError{err.Error()}
	}
	usage := usageError{"usage: ccshift rename <name|number|.> <new name>, ccshift rename <target> --reset, or ccshift rename --edit"}
	switch {
	case *edit && (len(pos) > 0 || *reset):
		return usage
	case !*edit && *reset && len(pos) != 1:
		return usage
	case !*edit && !*reset && len(pos) < 2:
		return usage
	}
	v, err := a.view(ctx, *termName)
	if err != nil {
		return err
	}
	// An empty new name means "back to the generated name".
	renames := map[string]string{}
	if *edit {
		if renames, err = a.editNames(v.order); err != nil {
			return err
		}
	} else {
		it, err := target.Resolve(pos[0], v.order, a.AncestorPIDs(a.SelfPID))
		if err != nil {
			return err
		}
		name := cleanName(pos[1:])
		if name == "" && !*reset {
			return usageError{"the new name is empty"}
		}
		renames[it.Session.ID] = name
	}
	if len(renames) == 0 {
		fmt.Fprintln(a.Out, "No names changed.")
		return nil
	}

	before := map[string]layout.Item{}
	for _, it := range v.order {
		before[it.Session.ID] = it
	}
	now := a.Now().Unix()
	if err := a.locked(func() error {
		if err := a.saveNames(v); err != nil {
			return err
		}
		return a.updateNames(func(m map[string]names.Entry) {
			for id, name := range renames {
				e := m[id]
				if name == "" {
					e.User, e.ClaudeAtSet = "", ""
				} else {
					e.User, e.ClaudeAtSet, e.Touched = name, before[id].ClaudeName, now
				}
				m[id] = e
			}
		})
	}); err != nil {
		return err
	}

	// Look again, so snapshots and tab titles get the names as they now resolve.
	after, err := a.view(ctx, *termName)
	if err != nil {
		return err
	}
	current := map[string]layout.Item{}
	for _, it := range after.order {
		current[it.Session.ID] = it
	}
	if err := a.locked(func() error {
		wss, err := a.Store.Workspaces()
		if err != nil {
			return err
		}
		for _, ws := range wss {
			snap, ok, err := a.Store.Latest(ws)
			if err != nil || !ok {
				continue
			}
			touched := false
			for i, e := range snap.Sessions {
				if _, ok := renames[e.SessionID]; ok {
					it := current[e.SessionID]
					snap.Sessions[i].Name, snap.Sessions[i].Generated = it.Session.Name, it.Generated
					touched = true
				}
			}
			if touched {
				if err := a.Store.SaveLatest(snap); err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		return err
	}
	a.syncTitles(ctx, after)

	renamed := 0
	for _, it := range v.order {
		name, ok := renames[it.Session.ID]
		if !ok {
			continue
		}
		now := current[it.Session.ID]
		switch {
		case name != "":
			renamed++
			a.ui().OK("Renamed %s to %s.", it.Session.Name, a.ui().Paint(ui.Bold, name))
		case now.Generated:
			a.ui().OK("Set %s back to its generated name, %s.", it.Session.Name, a.ui().Paint(ui.Bold, now.Session.Name))
		default:
			a.ui().OK("%s now shows the name Claude has for it, %s.", it.Session.Name, a.ui().Paint(ui.Bold, now.Session.Name))
		}
	}
	if renamed > 0 {
		fmt.Fprintln(a.Out, a.ui().Paint(ui.Dim, "Claude picks a new name up when the session is next resumed. To change it there now, run /rename <name> in that session."))
	}
	return nil
}

func (a *App) locked(fn func() error) error {
	unlock, err := a.Store.Lock()
	if err != nil {
		return err
	}
	defer unlock()
	return fn()
}

func (a *App) editNames(items []layout.Item) (map[string]string, error) {
	f, err := os.CreateTemp("", "ccshift-names-*.txt")
	if err != nil {
		return nil, err
	}
	defer os.Remove(f.Name())
	fmt.Fprintln(f, "# One session per line: id, then its name. Change the names you want, then save and quit.")
	fmt.Fprintln(f, "# Leave only the id on a line to put that session back on its generated name.")
	current := map[string]string{}
	userNamed := map[string]bool{}
	for _, it := range items {
		current[it.Session.ID] = cleanName([]string{it.Session.Name})
		userNamed[it.Session.ID] = !it.Generated
		fmt.Fprintf(f, "%s  %s\n", it.Session.ID, it.Session.Name)
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	if err := a.Editor(f.Name()); err != nil {
		return nil, fmt.Errorf("editor: %w", err)
	}
	b, err := os.ReadFile(f.Name())
	if err != nil {
		return nil, err
	}
	renames := map[string]string{}
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		old, ok := current[fields[0]]
		if !ok {
			continue
		}
		name := cleanName(fields[1:])
		if name == "" && !userNamed[fields[0]] {
			continue
		}
		if name != old {
			renames[fields[0]] = name
		}
	}
	return renames, nil
}

func newSessionID() string {
	var b [16]byte
	rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func (a *App) cmdNew(ctx context.Context, args []string) error {
	var extra []string
	for i, arg := range args {
		if arg == "--" {
			args, extra = args[:i], args[i+1:]
			break
		}
	}
	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	flagName := fs.String("name", "", "the session's name, for a name that starts with a dash")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return usageError{err.Error()}
	}
	if !a.HasCommand(a.ClaudeBin) {
		return fmt.Errorf("can't find %s", a.ClaudeBin)
	}
	v, err := a.view(ctx, "")
	if err != nil {
		return err
	}
	id := a.NewID()
	name := cleanName(append([]string{*flagName}, pos...))
	entry := names.Entry{}
	if name == "" {
		taken := map[string]bool{}
		for _, it := range v.order {
			taken[it.Session.Name] = true
		}
		g, err := a.Git(ctx, a.Cwd)
		base := filepath.Base(a.Cwd)
		if err == nil {
			base = names.Generate(g, "", a.Config.NameRules())
		}
		n := 1
		for taken[names.Numbered(base, n)] {
			n++
		}
		name = names.Numbered(base, n)
		// Applied marks the name as one ccshift made, so it keeps following the branch.
		entry = names.Entry{Applied: name, Base: base, Num: n, Touched: a.Now().Unix()}
	}
	if err := a.locked(func() error {
		if err := a.saveNames(v); err != nil {
			return err
		}
		if emptyEntry(entry) {
			return nil
		}
		return a.updateNames(func(m map[string]names.Entry) { m[id] = entry })
	}); err != nil {
		return err
	}
	if a.Config.SyncTabTitles && v.adapter.Tier() == term.Exact {
		if tab, ok := v.adapter.TabOf(a.Env); ok {
			if err := v.adapter.SetTitle(ctx, tab, name); err == nil {
				titles := map[string]titled{}
				a.Store.Load(titlesFile, &titles)
				titles[id] = titled{Tab: string(tab), Name: name}
				a.Store.Put(titlesFile, titles)
			}
		}
	}
	return a.Exec(append([]string{a.ClaudeBin, "--session-id", id, "--name=" + name}, extra...))
}
