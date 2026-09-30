package cli

import (
	"context"
	"errors"
	"flag"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/timileyinpelumi/ccshift/internal/telemetry"
)

func newTelemetry(enabled bool, getenv func(string) string, stateDir string) *telemetry.Client {
	if !enabled || getenv("DO_NOT_TRACK") != "" || getenv("CCSHIFT_NO_TELEMETRY") != "" || getenv("CI") != "" {
		return nil
	}
	return &telemetry.Client{Dir: stateDir, Endpoint: telemetry.Endpoint, Version: version, OS: runtime.GOOS, Arch: runtime.GOARCH, Now: time.Now, Describe: describePlatform}
}

// track records one finished command and starts the background sender when a send is due.
func (a *App) track(name string, err error, start time.Time) {
	if a.Telemetry == nil {
		return
	}
	ok := err == nil
	e := telemetry.Event{Command: name, OK: &ok, N: a.tally, MS: int(time.Since(start).Milliseconds())}
	if ad, aerr := a.adapter(""); aerr == nil {
		e.Terminal = ad.Name()
	}
	var ue usageError
	switch {
	case errors.As(err, &ue):
		e.Error = "usage"
	case err != nil:
		e.Error = a.scrub(err.Error())
	}
	a.Telemetry.Record(e)
	a.sendLater()
}

func (a *App) record(e telemetry.Event) {
	if a.Telemetry != nil {
		a.Telemetry.Record(e)
	}
}

func (a *App) sendLater() {
	if a.Telemetry != nil && a.Telemetry.Due() && a.Detach != nil {
		a.Detach([]string{a.Executable, "telemetry", "flush"})
	}
}

// cmdTelemetry is run in the background by ccshift itself, and by the install scripts.
//
//	ccshift telemetry flush
//	ccshift telemetry install --method script
func (a *App) cmdTelemetry(ctx context.Context, args []string) error {
	if a.Telemetry == nil || len(args) == 0 {
		return nil
	}
	switch args[0] {
	case "flush":
		return a.Telemetry.Flush(ctx)
	case "install":
		fs := flag.NewFlagSet("telemetry install", flag.ContinueOnError)
		fs.SetOutput(a.Err)
		method := fs.String("method", "", "how ccshift was installed")
		if _, err := parseArgs(fs, args[1:]); err != nil {
			return usageError{err.Error()}
		}
		a.Telemetry.Record(telemetry.Event{Command: "install", Method: *method})
		return a.Telemetry.Flush(ctx)
	}
	return nil
}

var (
	quotedText = regexp.MustCompile(`"[^"]*"|'[^']*'`)
	pathLike   = regexp.MustCompile(`\S*[/\\~]\S*`)
	withDigits = regexp.MustCompile(`\S*\d\S*`)
)

// scrub keeps the shape of an error message and drops what could identify a person or their
// work: quoted text, paths, ids and numbers, and the names of their sessions and directories.
func (a *App) scrub(msg string) string {
	var names []string
	if wss, err := a.Store.Workspaces(); err == nil {
		for _, ws := range wss {
			if snap, ok, err := a.Store.Latest(ws); err == nil && ok {
				for _, e := range snap.Sessions {
					names = append(names, e.Name, filepath.Base(e.CWD))
				}
			}
		}
	}
	sort.Slice(names, func(i, j int) bool { return len(names[i]) > len(names[j]) })
	for _, n := range names {
		if len(n) >= 2 {
			msg = strings.ReplaceAll(msg, n, "<name>")
		}
	}
	msg = quotedText.ReplaceAllString(msg, "<text>")
	msg = pathLike.ReplaceAllString(msg, "<path>")
	msg = withDigits.ReplaceAllString(msg, "<n>")
	if len(msg) > 200 {
		msg = msg[:200]
	}
	return msg
}
