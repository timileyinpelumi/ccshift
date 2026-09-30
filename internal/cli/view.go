package cli

import (
	"context"
	"fmt"

	"github.com/timileyinpelumi/ccshift/internal/claude"
	"github.com/timileyinpelumi/ccshift/internal/layout"
	"github.com/timileyinpelumi/ccshift/internal/names"
	"github.com/timileyinpelumi/ccshift/internal/term"
)

type view struct {
	adapter term.Adapter
	groups  map[string][]layout.Item
	order   []layout.Item
	live    []claude.Session
	names   map[string]names.Entry
	patches map[string]namePatch
}

func (a *App) adapter(name string) (term.Adapter, error) {
	if name != "" {
		if ad := term.ByName(a.Terms, name); ad != nil {
			return ad, nil
		}
		return nil, usageError{fmt.Sprintf("unknown terminal %q", name)}
	}
	return term.Detect(a.Terms, a.Env, a.Comms(a.AncestorPIDs(a.SelfPID))), nil
}

func (a *App) view(ctx context.Context, termName string) (*view, error) {
	ad, err := a.adapter(termName)
	if err != nil {
		return nil, err
	}
	live, err := a.Claude.Live(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading Claude sessions: %w", err)
	}
	var tabs []term.Tab
	if ad.Tier() == term.Exact {
		if tabs, err = ad.List(ctx); err != nil {
			fmt.Fprintf(a.Err, "ccshift: couldn't read %s tabs (%v), using start order\n", ad.Name(), err)
			tabs = nil
		}
	}
	prev, err := a.Store.Order()
	if err != nil {
		fmt.Fprintf(a.Err, "ccshift: ignoring the saved tab order (%v)\n", err)
		prev = nil
	}
	groups := layout.Build(layout.Input{
		Sessions: live, EnvOf: a.EnvOf, Adapter: ad, Tabs: tabs, Config: a.Config, PrevOrder: prev,
	})
	entries, patches := a.applyNames(ctx, groups)
	return &view{adapter: ad, groups: groups, order: layout.Flatten(groups), live: live, names: entries, patches: patches}, nil
}

func displayName(s claude.Session) string {
	if s.Name != "" {
		return s.Name
	}
	if len(s.ID) > 8 {
		return s.ID[:8]
	}
	return s.ID
}
