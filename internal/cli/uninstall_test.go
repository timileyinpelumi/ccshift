package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/timileyinpelumi/ccshift/internal/settings"
)

func uninstallApp(t *testing.T) (*harness, string) {
	t.Helper()
	h, p := initApp(t, userSettings)
	bin := filepath.Join(t.TempDir(), "ccshift")
	os.WriteFile(bin, []byte("binary"), 0o755)
	h.app.Executable = bin
	h.app.ConfigPath = filepath.Join(t.TempDir(), "ccshift", "config.toml")
	os.MkdirAll(filepath.Dir(h.app.ConfigPath), 0o755)
	os.WriteFile(h.app.ConfigPath, []byte("stale_days = 3\n"), 0o644)
	h.must(t, "init")
	saveSnap(t, h, "work", "s1")
	return h, p
}

func TestUninstallRemovesEverything(t *testing.T) {
	h, p := uninstallApp(t)
	out := h.must(t, "uninstall", "--yes")
	f, _ := settings.Load(p)
	if len(f.HookCommands("Stop")) != 1 || strings.Contains(string(f.StatusLine()), "ccshift") {
		t.Fatalf("hooks or statusline left behind: %q %s", f.HookCommands("Stop"), f.StatusLine())
	}
	for _, gone := range []string{h.app.Store.Dir, filepath.Dir(h.app.ConfigPath), h.app.Executable} {
		if _, err := os.Stat(gone); err == nil {
			t.Errorf("%s is still there", gone)
		}
	}
	if !strings.Contains(out, "ccshift is uninstalled") {
		t.Fatalf("out = %q", out)
	}
}

func TestUninstallKeepData(t *testing.T) {
	h, _ := uninstallApp(t)
	h.must(t, "uninstall", "--yes", "--keep-data")
	if _, err := os.Stat(filepath.Join(h.app.Store.Dir, "workspaces")); err != nil {
		t.Fatal("--keep-data removed the saved layouts")
	}
	if _, err := os.Stat(h.app.Executable); err == nil {
		t.Fatal("the binary should still go")
	}
}

func TestUninstallAsksFirst(t *testing.T) {
	h, _ := uninstallApp(t)
	h.app.In = strings.NewReader("n\n")
	if code := h.run("uninstall"); code != 1 {
		t.Fatalf("exit = %d", code)
	}
	if _, err := os.Stat(h.app.Executable); err != nil {
		t.Fatal("answering no must leave everything in place")
	}
	h.app.In = strings.NewReader("y\n")
	h.must(t, "uninstall")
	if _, err := os.Stat(h.app.Executable); err == nil {
		t.Fatal("answering yes should uninstall")
	}
}

func TestUninstallLeavesAPackagedBinaryToThePackageManager(t *testing.T) {
	h, _ := uninstallApp(t)
	h.app.Executable = "/usr/bin/ccshift"
	out := h.must(t, "uninstall", "--yes")
	if !strings.Contains(out, "sudo apt remove ccshift") {
		t.Fatalf("out = %q", out)
	}
}
