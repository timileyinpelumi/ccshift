package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"

	"github.com/timileyinpelumi/ccshift/internal/target"
	"github.com/timileyinpelumi/ccshift/internal/term"
)

func (a *App) cmdFocus(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("focus", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	termName := fs.String("terminal", "", "terminal adapter to use instead of detecting it")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return usageError{err.Error()}
	}
	if len(pos) != 1 {
		return usageError{"usage: ccshift focus <name|number|.>"}
	}
	v, err := a.view(ctx, *termName)
	if err != nil {
		return err
	}
	if v.adapter.Tier() != term.Exact {
		return fmt.Errorf("focus isn't supported on %s", v.adapter.Name())
	}
	it, err := target.Resolve(pos[0], v.order, a.AncestorPIDs(a.SelfPID))
	if err != nil {
		return err
	}
	if !it.Matched {
		return fmt.Errorf("no %s tab found for %s", v.adapter.Name(), displayName(it.Session))
	}
	err = v.adapter.Focus(ctx, it.Tab)
	if errors.Is(err, term.ErrUnsupported) {
		return fmt.Errorf("focus isn't supported on %s", v.adapter.Name())
	}
	return err
}
