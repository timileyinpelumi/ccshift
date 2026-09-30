package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/timileyinpelumi/ccshift/internal/names"
)

const DefaultWorkspace = "default"

type Config struct {
	Workspaces              map[string][]string `toml:"workspaces"`
	HistoryKeep             int                 `toml:"history_keep"`
	WarnThresholds          []int               `toml:"warn_thresholds"`
	AutosaveDebounceSeconds int                 `toml:"autosave_debounce_seconds"`
	StaleDays               int                 `toml:"stale_days"`
	TicketPattern           string              `toml:"ticket_pattern"`
	TicketIgnore            []string            `toml:"ticket_ignore"`
	SyncTabTitles           bool                `toml:"sync_tab_titles"`
	BriefModel              string              `toml:"brief_model"`
}

// NameRules says what counts as a ticket id inside a branch name, in any letter case.
func (c Config) NameRules() names.Rules {
	r := names.Rules{Ignore: map[string]bool{}}
	if re, err := regexp.Compile("(?i)" + c.TicketPattern); err == nil {
		r.Ticket = re
	}
	for _, w := range c.TicketIgnore {
		r.Ignore[strings.ToLower(w)] = true
	}
	return r
}

func Default() Config {
	return Config{
		HistoryKeep: 20, WarnThresholds: []int{70, 85}, AutosaveDebounceSeconds: 30, StaleDays: 14,
		TicketPattern: `\b([A-Z][A-Z0-9]{1,5})[-_](\d{2,})\b`, SyncTabTitles: true, BriefModel: "sonnet",
		// Words that come before a number in branch names without being ticket prefixes.
		TicketIgnore: strings.Fields("fix bug feat hotfix issue pr mr rc ver node react vue sha md utf http tls ssl es py go java php ruby rails net ios step part phase test day week"),
	}
}

func DefaultPath() string {
	if runtime.GOOS == "windows" {
		if dir, err := os.UserConfigDir(); err == nil { // %AppData%
			return filepath.Join(dir, "ccshift", "config.toml")
		}
	}
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "ccshift", "config.toml")
}

var workspaceName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func ValidWorkspaceName(name string) bool { return workspaceName.MatchString(name) }

func Load(path string) (Config, error) {
	c := Default()
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return Config{}, err
	}
	if _, err := toml.Decode(string(b), &c); err != nil {
		return Config{}, fmt.Errorf("config %s: %w", path, err)
	}
	for ws := range c.Workspaces {
		if !ValidWorkspaceName(ws) {
			return Config{}, fmt.Errorf("config %s: workspace name %q may only use letters, digits, '.', '_' and '-'", path, ws)
		}
	}
	if c.HistoryKeep <= 0 {
		c.HistoryKeep = Default().HistoryKeep
	}
	if c.AutosaveDebounceSeconds < 0 {
		c.AutosaveDebounceSeconds = Default().AutosaveDebounceSeconds
	}
	if c.StaleDays <= 0 {
		c.StaleDays = Default().StaleDays
	}
	for _, th := range c.WarnThresholds {
		if th < 1 || th > 100 {
			return Config{}, fmt.Errorf("config %s: warn_thresholds must be between 1 and 100, got %d", path, th)
		}
	}
	sort.Ints(c.WarnThresholds)
	if _, err := regexp.Compile("(?i)" + c.TicketPattern); err != nil {
		return Config{}, fmt.Errorf("config %s: ticket_pattern: %w", path, err)
	}
	return c, nil
}

func (c Config) WorkspaceFor(cwd string) string {
	best, bestLen := DefaultWorkspace, -1
	for ws, prefixes := range c.Workspaces {
		for _, p := range prefixes {
			p = filepath.Clean(expandHome(p))
			if cwd != p && !strings.HasPrefix(cwd, p+"/") {
				continue
			}
			if len(p) > bestLen || (len(p) == bestLen && ws < best) {
				best, bestLen = ws, len(p)
			}
		}
	}
	return best
}

func expandHome(p string) string {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, p[1:])
}
