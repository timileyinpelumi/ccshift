package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceFor(t *testing.T) {
	t.Setenv("HOME", "/home/u")
	t.Setenv("USERPROFILE", "/home/u") // what Windows uses for the home directory
	c := Config{Workspaces: map[string][]string{
		"work":     {"~/dev/acme", "/srv/shopfront"},
		"personal": {"~/dev/personal"},
		"cvx":      {"~/dev/personal/cvx"},
	}}
	cases := map[string]string{
		"/home/u/dev/acme":             "work",
		"/home/u/dev/acme/api":         "work",
		"/srv/shopfront":               "work",
		"/home/u/dev/personal/trueset": "personal",
		"/home/u/dev/personal/cvx/web": "cvx",
		"/home/u/dev/personal-old":     DefaultWorkspace,
		"/tmp":                         DefaultWorkspace,
	}
	for cwd, want := range cases {
		if got := c.WorkspaceFor(cwd); got != want {
			t.Errorf("WorkspaceFor(%q) = %q, want %q", cwd, got, want)
		}
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	c, err := Load(filepath.Join(dir, "missing.toml"))
	if err != nil || c.HistoryKeep != 20 {
		t.Fatalf("missing file: %+v, %v", c, err)
	}

	p := filepath.Join(dir, "config.toml")
	os.WriteFile(p, []byte("history_keep = 5\n[workspaces]\nwork = [\"/w\"]\n"), 0o644)
	c, err = Load(p)
	if err != nil || c.HistoryKeep != 5 || c.Workspaces["work"][0] != "/w" {
		t.Fatalf("got %+v, %v", c, err)
	}

	os.WriteFile(p, []byte("history_keep = 0\n"), 0o644)
	if c, _ = Load(p); c.HistoryKeep != 20 {
		t.Fatalf("history_keep 0 should fall back to 20, got %d", c.HistoryKeep)
	}

	os.WriteFile(p, []byte("[workspaces\n"), 0o644)
	if _, err = Load(p); err == nil {
		t.Fatal("expected parse error")
	}

	os.WriteFile(p, []byte("[workspaces]\n\"../x\" = [\"/w\"]\n"), 0o644)
	if _, err = Load(p); err == nil {
		t.Fatal("expected error for invalid workspace name")
	}
}

func TestValidWorkspaceName(t *testing.T) {
	for _, ok := range []string{"work", "default", "cvx-2", "a.b_c"} {
		if !ValidWorkspaceName(ok) {
			t.Errorf("%q should be valid", ok)
		}
	}
	for _, bad := range []string{"", "../x", "a/b", ".hidden", "with space"} {
		if ValidWorkspaceName(bad) {
			t.Errorf("%q should be invalid", bad)
		}
	}
}

func TestHookSettingsDefaultsAndOverrides(t *testing.T) {
	c := Default()
	if len(c.WarnThresholds) != 2 || c.WarnThresholds[0] != 70 || c.AutosaveDebounceSeconds != 30 || c.StaleDays != 14 {
		t.Fatalf("defaults = %+v", c)
	}
	p := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(p, []byte("warn_thresholds = [90, 50]\nautosave_debounce_seconds = 5\nstale_days = 3\n"), 0o644)
	c, err := Load(p)
	if err != nil || c.WarnThresholds[0] != 50 || c.WarnThresholds[1] != 90 || c.AutosaveDebounceSeconds != 5 || c.StaleDays != 3 {
		t.Fatalf("got %+v, %v", c, err)
	}
	os.WriteFile(p, []byte("warn_thresholds = [0, 140]\n"), 0o644)
	if _, err := Load(p); err == nil {
		t.Fatal("thresholds outside 1..100 should be rejected")
	}
}

func TestNamingSettings(t *testing.T) {
	c := Default()
	if !c.SyncTabTitles || c.TicketPattern == "" || !c.NameRules().Ignore["node"] {
		t.Fatalf("defaults = %+v", c)
	}
	p := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(p, []byte("sync_tab_titles = false\nticket_pattern = \"ABC-\\\\d+\"\n"), 0o644)
	c, err := Load(p)
	if err != nil || c.SyncTabTitles || c.TicketPattern != `ABC-\d+` {
		t.Fatalf("got %+v, %v", c, err)
	}
	os.WriteFile(p, []byte("ticket_pattern = \"(\"\n"), 0o644)
	if _, err := Load(p); err == nil {
		t.Fatal("a pattern that doesn't compile should be rejected")
	}
}
