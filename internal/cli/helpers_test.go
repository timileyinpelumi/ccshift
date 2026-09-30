package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/timileyinpelumi/ccshift/internal/claude"
	"github.com/timileyinpelumi/ccshift/internal/config"
	"github.com/timileyinpelumi/ccshift/internal/names"
	"github.com/timileyinpelumi/ccshift/internal/store"
	"github.com/timileyinpelumi/ccshift/internal/term"
)

func init() { term.EnvKeys = append(term.EnvKeys, "FAKE_PANE") }

type fakeTerm struct {
	name    string
	tier    term.Tier
	tabs    []term.Tab
	opened  [][]term.Launch
	focused []term.TabID
	openErr error
	titles  map[term.TabID]string
	titled  int

	onSetTitle func()
}

func (f *fakeTerm) Name() string                            { return f.name }
func (f *fakeTerm) Tier() term.Tier                         { return f.tier }
func (f *fakeTerm) Detect(map[string]string, []string) bool { return true }
func (f *fakeTerm) TabOf(env map[string]string) (term.TabID, bool) {
	p := env["FAKE_PANE"]
	return term.TabID(p), p != ""
}
func (f *fakeTerm) List(context.Context) ([]term.Tab, error) { return f.tabs, nil }
func (f *fakeTerm) OpenWindow(_ context.Context, _ string, ls []term.Launch) error {
	if f.openErr != nil {
		return f.openErr
	}
	f.opened = append(f.opened, plain(ls))
	return nil
}
func (f *fakeTerm) SetTitle(_ context.Context, id term.TabID, title string) error {
	if f.titles == nil {
		f.titles = map[term.TabID]string{}
	}
	if f.onSetTitle != nil {
		f.onSetTitle()
	}
	f.titles[id] = title
	f.titled++
	return nil
}
func (f *fakeTerm) Focus(_ context.Context, id term.TabID) error {
	if f.tier != term.Exact {
		return term.ErrUnsupported
	}
	f.focused = append(f.focused, id)
	return nil
}

type liveSession struct {
	id, cwd, name, pane string
	pid                 int
	minute              int
}

type harness struct {
	app      *App
	out, err *bytes.Buffer
	term     *fakeTerm
	sessions []liveSession
	bgStates []string
	projects string
	gits     map[string]names.Git
	gitErr   map[string]error
	aiTitles map[string]string
	execd    [][]string
}

var testNow = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

func testApp(t *testing.T, tier term.Tier) *harness {
	t.Helper()
	h := &harness{out: &bytes.Buffer{}, err: &bytes.Buffer{}}
	h.term = &fakeTerm{name: "fake", tier: tier}
	h.projects = t.TempDir()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	reader := &claude.Reader{
		Run: func(context.Context, string, ...string) ([]byte, error) {
			var rows []map[string]any
			for _, s := range h.sessions {
				rows = append(rows, map[string]any{
					"pid": s.pid, "sessionId": s.id, "cwd": s.cwd, "name": s.name, "kind": "interactive",
					"status": "idle", "startedAt": testNow.Add(time.Duration(s.minute-60) * time.Minute).UnixMilli(),
				})
			}
			for i, st := range h.bgStates {
				rows = append(rows, map[string]any{"id": "bg", "sessionId": "bg-" + strconv.Itoa(i), "cwd": "/b", "name": "job", "kind": "background", "state": st, "startedAt": testNow.UnixMilli()})
			}
			return json.Marshal(rows)
		},
		ProjectsDir: h.projects,
		Alive:       func(int) bool { return true },
		HasTTY:      func(int) bool { return true },
		StartTime:   func(int) (string, error) { return "", nil },
	}
	h.app = &App{
		Out: h.out, Err: h.err, In: strings.NewReader(""),
		Claude: reader, Store: st, Config: config.Default(),
		Terms: []term.Adapter{h.term}, Env: map[string]string{}, SelfPID: 1,
		EnvOf: func(pid int) (map[string]string, error) {
			for _, s := range h.sessions {
				if s.pid == pid {
					return map[string]string{"FAKE_PANE": s.pane}, nil
				}
			}
			return nil, os.ErrNotExist
		},
		AncestorPIDs: func(int) []int { return nil },
		TTYOf:        func(int) string { return "" },
		Comms:        func([]int) []string { return nil },
		Git: func(_ context.Context, cwd string) (names.Git, error) {
			if err := h.gitErr[cwd]; err != nil {
				return names.Git{}, err
			}
			if g, ok := h.gits[cwd]; ok {
				return g, nil
			}
			return names.Git{Repo: filepath.Base(cwd), Branch: "main", Default: "main"}, nil
		},
		AITitle:    func(id string) string { return h.aiTitles[id] },
		Exec:       func(argv []string) error { h.execd = append(h.execd, argv); return nil },
		NewID:      func() string { return "new-id" },
		RunClaude:  func(context.Context, string, []string, string) (string, error) { return "brief\n", nil },
		GitSummary: func(context.Context, string) string { return "" },
		Cwd:        "/p/new",
		DirExists:  func(string) bool { return true },
		ClaudeBin:  "/bin/claude",
		Now:        func() time.Time { return testNow },
		Editor:     func(string) error { return nil },
		Notify:     func(string, string) {},
		HasCommand: func(string) bool { return true },
		Shell:      func(context.Context, string, []byte) ([]byte, error) { return nil, nil },
	}
	return h
}

func (h *harness) addTranscript(t *testing.T, id string) {
	t.Helper()
	dir := filepath.Join(h.projects, "-p")
	os.MkdirAll(dir, 0o755)
	if err := os.WriteFile(filepath.Join(dir, id+".jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (h *harness) run(args ...string) int {
	h.out.Reset()
	h.err.Reset()
	return h.app.Run(context.Background(), args)
}

func (h *harness) must(t *testing.T, args ...string) string {
	t.Helper()
	if code := h.run(args...); code != 0 {
		t.Fatalf("ccshift %v exited %d\nstdout: %s\nstderr: %s", args, code, h.out, h.err)
	}
	return h.out.String()
}

// plain strips the env -u prefix ccshift puts in front of every launch.
func plain(ls []term.Launch) []term.Launch {
	out := make([]term.Launch, len(ls))
	for i, l := range ls {
		for len(l.Argv) > 0 && (l.Argv[0] == "env" || l.Argv[0] == "-u" || strings.HasPrefix(l.Argv[0], "CLAUDE")) {
			l.Argv = l.Argv[1:]
		}
		out[i] = l
	}
	return out
}
