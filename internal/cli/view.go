package cli

import (
	"context"
	"fmt"
	"maps"

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
	live, err := a.liveSessions(ctx)
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
		Sessions: live, EnvOf: a.sessionEnv(live), Adapter: ad, Tabs: tabs, Config: a.Config, PrevOrder: prev,
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

const sessionEnvFile = "session-env.json"

// sessionEnv says which terminal variables each session's process has. What the session's own
// hook recorded comes first; reading the process is the fallback for sessions that started before
// the hooks were installed, and it is not possible on every operating system.
func (a *App) sessionEnv(live []claude.Session) func(pid int) (map[string]string, error) {
	recorded := map[string]map[string]string{}
	a.Store.Load(sessionEnvFile, &recorded)
	byPID := map[int]map[string]string{}
	for _, s := range live {
		if env, ok := recorded[s.ID]; ok && s.PID != 0 {
			byPID[s.PID] = env
		}
	}
	return func(pid int) (map[string]string, error) {
		if env, ok := byPID[pid]; ok {
			return env, nil
		}
		env, err := a.EnvOf(pid)
		if err != nil {
			return nil, err
		}
		if a.TTYOf != nil {
			if tty := a.TTYOf(pid); tty != "" {
				env[term.TTYKey] = tty
			}
		}
		return env, nil
	}
}

// recordSessionEnv stores the terminal variables of the session whose hook is running.
// A hook is started by Claude, so it has the session's environment.
func (a *App) recordSessionEnv(id string) error {
	if id == "" {
		return nil
	}
	env := map[string]string{}
	for _, k := range term.EnvKeys {
		if v := a.Env[k]; v != "" {
			env[k] = v
		}
	}
	if a.TTYOf != nil {
		for _, pid := range a.AncestorPIDs(a.SelfPID) {
			if tty := a.TTYOf(pid); tty != "" {
				env[term.TTYKey] = tty
				break
			}
		}
	}
	if len(env) == 0 {
		return nil
	}
	unlock, err := a.Store.TryLock(hookBudget / 4)
	if err != nil {
		return err
	}
	defer unlock()
	recorded := map[string]map[string]string{}
	a.Store.Load(sessionEnvFile, &recorded)
	if maps.Equal(recorded[id], env) {
		return nil
	}
	recorded[id] = env
	return a.Store.Put(sessionEnvFile, recorded)
}
