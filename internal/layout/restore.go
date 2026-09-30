package layout

import (
	"sort"

	"github.com/timileyinpelumi/ccshift/internal/claude"
	"github.com/timileyinpelumi/ccshift/internal/store"
	"github.com/timileyinpelumi/ccshift/internal/term"
)

type Skip struct {
	Entry  store.Entry
	Reason string
}

type RestorePlan struct {
	Launches []term.Launch
	Launched []store.Entry // the entry behind each launch, in the same order
	Skipped  []Skip
}

func PlanRestore(snap store.Snapshot, running map[string]bool, transcriptExists func(string) bool, dirExists func(string) bool, claudeBin string) RestorePlan {
	entries := append([]store.Entry(nil), snap.Sessions...)
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Position < entries[j].Position })
	var p RestorePlan
	for _, e := range entries {
		switch {
		case running[e.SessionID]:
			p.Skipped = append(p.Skipped, Skip{e, "already running"})
		case !transcriptExists(e.SessionID):
			p.Skipped = append(p.Skipped, Skip{e, "transcript not found"})
		case !dirExists(e.CWD):
			p.Skipped = append(p.Skipped, Skip{e, "directory no longer exists: " + e.CWD})
		default:
			argv := []string{claudeBin, "--resume", e.SessionID}
			if e.Name != "" && !claude.IsDefaultName(e.Name, e.CWD) {
				// The = form keeps a name that starts with "-" from being read as a flag.
				argv = append(argv, "--name="+e.Name)
			}
			p.Launches = append(p.Launches, term.Launch{CWD: e.CWD, Title: Title(e), Argv: argv})
			p.Launched = append(p.Launched, e)
		}
	}
	return p
}
