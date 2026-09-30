package cli

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/timileyinpelumi/ccshift/internal/term"
)

type fakeReleases struct {
	latest    string
	installed []string
	fail      error
	checks    int
}

func (f *fakeReleases) Latest(context.Context) (string, error) { f.checks++; return f.latest, f.fail }
func (f *fakeReleases) Install(_ context.Context, tag, exe string) error {
	f.installed = append(f.installed, tag+" "+exe)
	return nil
}

func updateApp(t *testing.T, latest string) (*harness, *fakeReleases) {
	t.Helper()
	old := version
	version = "0.3.0"
	t.Cleanup(func() { version = old })
	h := testApp(t, term.Exact)
	h.app.Executable = "/home/u/.local/bin/ccshift"
	r := &fakeReleases{latest: latest}
	h.app.Releases = r
	return h, r
}

func TestUpdate(t *testing.T) {
	h, r := updateApp(t, "v0.4.0")
	if out := h.must(t, "update", "--check"); !strings.Contains(out, "v0.4.0 is available") || len(r.installed) != 0 {
		t.Fatalf("--check: %q %v", out, r.installed)
	}
	out := h.must(t, "update")
	if !strings.Contains(out, "Updated ccshift from 0.3.0 to v0.4.0") || r.installed[0] != "v0.4.0 /home/u/.local/bin/ccshift" {
		t.Fatalf("out = %q installed = %v", out, r.installed)
	}
}

func TestUpdateWhenCurrent(t *testing.T) {
	h, r := updateApp(t, "v0.3.0")
	if out := h.must(t, "update"); !strings.Contains(out, "0.3.0 is the latest version") || len(r.installed) != 0 {
		t.Fatalf("out = %q", out)
	}
}

func TestUpdateLeavesPackagesAndSourceBuildsAlone(t *testing.T) {
	h, r := updateApp(t, "v0.4.0")
	h.app.Executable = "/usr/bin/ccshift"
	if out := h.must(t, "update"); !strings.Contains(out, "package manager") || len(r.installed) != 0 {
		t.Fatalf("packaged: %q", out)
	}
	version = "dev"
	h.app.Executable = "/home/u/go/bin/ccshift"
	if out := h.must(t, "update"); !strings.Contains(out, "go install") || len(r.installed) != 0 {
		t.Fatalf("source build: %q", out)
	}
}

func TestUpdateReportsNetworkErrors(t *testing.T) {
	h, r := updateApp(t, "")
	r.fail = errors.New("no network")
	if code := h.run("update"); code != 1 || !strings.Contains(h.err.String(), "no network") {
		t.Fatalf("exit %d, %q", code, h.err)
	}
}

func TestAutoUpdateOncePerDayAfterInteractiveCommands(t *testing.T) {
	h, r := updateApp(t, "v0.4.0")
	h.app.Interactive = func() bool { return true }
	h.must(t, "ls")
	if len(r.installed) != 1 || !strings.Contains(h.out.String(), "Updated ccshift to v0.4.0") {
		t.Fatalf("installed = %v out = %q", r.installed, h.out)
	}
	h.must(t, "ls")
	h.at(2 * time.Hour)
	h.must(t, "ls")
	if r.checks != 1 {
		t.Fatalf("checked %d times in one day", r.checks)
	}
	h.at(25 * time.Hour)
	h.must(t, "ws")
	if r.checks != 2 {
		t.Fatalf("checks after a day = %d", r.checks)
	}
}

func TestAutoUpdateStaysOutOfHooksPipesAndWhenOff(t *testing.T) {
	h, r := updateApp(t, "v0.4.0")
	h.app.Interactive = func() bool { return true }
	h.hook(t, "stop", `{}`)
	h.status(t, statusJSON("s1", "a", 10))
	h.app.Interactive = func() bool { return false }
	h.must(t, "ls")
	h.app.Interactive = func() bool { return true }
	h.app.Config.AutoUpdate = false
	h.must(t, "ls")
	if r.checks != 0 {
		t.Fatalf("checked %d times", r.checks)
	}
}
