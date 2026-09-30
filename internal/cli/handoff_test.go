package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/timileyinpelumi/ccshift/internal/target"
	"github.com/timileyinpelumi/ccshift/internal/term"
)

type tabTerm struct {
	*fakeTerm
	near   []term.TabID
	tabs   *[]term.Launch
	closed *[]term.TabID
}

func (t tabTerm) OpenTab(_ context.Context, near term.TabID, l term.Launch) error {
	*t.tabs = append(*t.tabs, plain([]term.Launch{l})[0])
	return nil
}

func (t tabTerm) CloseTab(_ context.Context, id term.TabID) error {
	*t.closed = append(*t.closed, id)
	return nil
}

func handoffApp(t *testing.T) (*harness, *[]string) {
	t.Helper()
	h := testApp(t, term.Exact)
	h.term.tabs = []term.Tab{{ID: "p1"}, {ID: "p2"}}
	h.sessions = []liveSession{
		{id: "s1", cwd: "/w/api", name: "api work", pane: "p1", pid: 1, minute: 0},
		{id: "s2", cwd: "/w/web", name: "web", pane: "p2", pid: 2, minute: 1},
	}
	dir := filepath.Join(h.projects, "-w-api")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "s1.jsonl"), []byte(
		`{"type":"user","message":{"content":"add rate limits"}}`+"\n"+
			`{"type":"assistant","message":{"content":[{"type":"text","text":"Added a limiter in limit.go."},{"type":"tool_use","name":"Edit","input":{"file_path":"/w/api/limit.go"}}]}}`+"\n"), 0o644)
	var prompts []string
	h.app.RunClaude = func(_ context.Context, cwd string, args []string, stdin string) (string, error) {
		prompts = append(prompts, cwd+" | "+strings.Join(args, " ")+" | "+stdin)
		return "## Goal\nRate limits.\n", nil
	}
	h.app.GitSummary = func(context.Context, string) string { return "branch: limits\n M limit.go" }
	h.hook(t, "session-start", `{}`)
	return h, &prompts
}

func TestHandoff(t *testing.T) {
	h, prompts := handoffApp(t)
	out := h.must(t, "handoff", "api work")

	if len(*prompts) != 1 {
		t.Fatalf("brief writer ran %d times", len(*prompts))
	}
	p := (*prompts)[0]
	for _, want := range []string{"/w/api | -p --model sonnet --tools Read,Grep,Glob --no-session-persistence | ", "add rate limits", "edited /w/api/limit.go", "branch: limits", "## Next steps"} {
		if !strings.Contains(p, want) {
			t.Errorf("brief writer input missing %q", want)
		}
	}
	briefPath := filepath.Join(h.app.Store.Dir, "handoffs", "s1.md")
	if b, _ := os.ReadFile(briefPath); string(b) != "## Goal\nRate limits.\n" {
		t.Fatalf("brief file = %q", b)
	}
	if len(h.term.opened) != 1 {
		t.Fatalf("opened = %+v", h.term.opened)
	}
	want := term.Launch{CWD: "/w/api", Title: "api work (2)", Argv: []string{"/bin/claude", "--session-id", "new-id", "--name=api work (2)", "Read " + briefPath + " and continue from it."}}
	if !reflect.DeepEqual(h.term.opened[0][0], want) {
		t.Fatalf("launch = %+v", h.term.opened[0][0])
	}
	// The new session takes the old one's place in the save; the old one stays out of later saves.
	snap, _, _ := h.app.Store.Latest("default")
	if snap.Sessions[0].SessionID != "new-id" || snap.Sessions[0].Name != "api work (2)" || snap.Sessions[1].SessionID != "s2" {
		t.Fatalf("snapshot = %+v", snap.Sessions)
	}
	h.at(3 * 60 * 1e9)
	h.hook(t, "stop", `{}`)
	if got := strings.Join(latestIDs(t, h, "default"), ","); got != "new-id,s2" {
		t.Fatalf("after autosave: %s", got)
	}
	var lineage map[string]string
	h.app.Store.Load("lineage.json", &lineage)
	if lineage["s1"] != "new-id" {
		t.Fatalf("lineage = %v", lineage)
	}
	if !strings.Contains(out, "api work (2)") || !strings.Contains(out, briefPath) {
		t.Fatalf("out = %q", out)
	}
}

func TestHandoffDefaultsToTheCurrentSession(t *testing.T) {
	h, _ := handoffApp(t)
	h.app.AncestorPIDs = func(int) []int { return []int{50, 2} }
	os.WriteFile(filepath.Join(h.projects, "-w-api", "s2.jsonl"), []byte(`{"type":"user","message":{"content":"style the header"}}`+"\n"), 0o644)
	h.must(t, "handoff")
	if h.term.opened[0][0].Title != "web (2)" {
		t.Fatalf("launch = %+v", h.term.opened[0][0])
	}
}

func TestHandoffOpensNextToTheOldTabAndCanCloseIt(t *testing.T) {
	h, _ := handoffApp(t)
	var tabs []term.Launch
	var closed []term.TabID
	h.app.Terms = []term.Adapter{tabTerm{fakeTerm: h.term, tabs: &tabs, closed: &closed}}
	h.must(t, "handoff", "1", "--close-old")
	if len(tabs) != 1 || tabs[0].Title != "api work (2)" || len(h.term.opened) != 0 {
		t.Fatalf("tabs = %+v opened = %+v", tabs, h.term.opened)
	}
	if len(closed) != 1 || closed[0] != "p1" {
		t.Fatalf("closed = %v", closed)
	}
}

func TestHandoffCloseOldUnsupported(t *testing.T) {
	h, _ := handoffApp(t)
	out := h.must(t, "handoff", "1", "--close-old")
	if !strings.Contains(out, "can't close a tab") {
		t.Fatalf("out = %q", out)
	}
}

func TestHandoffEditAndNoLaunch(t *testing.T) {
	h, _ := handoffApp(t)
	var edited string
	h.app.Editor = func(path string) error {
		edited = path
		return os.WriteFile(path, []byte("edited brief\n"), 0o644)
	}
	out := h.must(t, "handoff", "1", "--edit", "--no-launch")
	if !strings.HasSuffix(edited, filepath.Join("handoffs", "s1.md")) || len(h.term.opened) != 0 || !strings.Contains(out, edited) {
		t.Fatalf("edited=%q opened=%v out=%q", edited, h.term.opened, out)
	}
	if got := latestIDs(t, h, "default"); got[0] != "s1" {
		t.Fatalf("--no-launch changed the save: %v", got)
	}
}

func TestHandoffKeepsTheExtractWhenTheBriefFails(t *testing.T) {
	h, _ := handoffApp(t)
	h.app.RunClaude = func(context.Context, string, []string, string) (string, error) { return "", errors.New("rate limited") }
	if code := h.run("handoff", "1"); code != 1 {
		t.Fatalf("exit = %d", code)
	}
	extract := filepath.Join(h.app.Store.Dir, "handoffs", "s1.extract.md")
	if b, _ := os.ReadFile(extract); !strings.Contains(string(b), "add rate limits") {
		t.Fatalf("extract = %q", b)
	}
	if !strings.Contains(h.err.String(), "rate limited") || !strings.Contains(h.err.String(), extract) || len(h.term.opened) != 0 {
		t.Fatalf("stderr = %q", h.err)
	}
	h.app.RunClaude = func(context.Context, string, []string, string) (string, error) { return "  \n", nil }
	if code := h.run("handoff", "1"); code != 1 {
		t.Fatal("an empty brief should fail")
	}
}

func TestHandoffModel(t *testing.T) {
	h, prompts := handoffApp(t)
	h.app.Config.BriefModel = "haiku"
	h.must(t, "handoff", "1", "--no-launch")
	h.must(t, "handoff", "1", "--no-launch", "--model", "opus")
	if !strings.Contains((*prompts)[0], "--model haiku ") || !strings.Contains((*prompts)[1], "--model opus ") {
		t.Fatalf("prompts = %.120q", *prompts)
	}
}

func TestHandoffErrors(t *testing.T) {
	h, _ := handoffApp(t)
	if code := h.run("handoff", "nope"); code != 1 {
		t.Fatalf("unknown target: exit %d", code)
	}
	if code := h.run("handoff", "web"); code != 1 || !strings.Contains(h.err.String(), "transcript") {
		t.Fatalf("no transcript: exit %d, %q", code, h.err)
	}
	if code := h.run("handoff", "1", "2"); code != 2 {
		t.Fatalf("usage: exit %d", code)
	}
}

func TestHandoffOfASavedSessionThatIsNotRunning(t *testing.T) {
	h, _ := handoffApp(t)
	h.sessions = h.sessions[1:]
	h.must(t, "handoff", "api work")
	if len(h.term.opened) != 1 || h.term.opened[0][0].CWD != "/w/api" {
		t.Fatalf("opened = %+v", h.term.opened)
	}
	if got := latestIDs(t, h, "default"); got[0] != "new-id" {
		t.Fatalf("save = %v", got)
	}
}

func TestHandoffWhenTheTerminalCannotOpenATab(t *testing.T) {
	h, _ := handoffApp(t)
	h.term.openErr = errors.New("no display")
	if code := h.run("handoff", "1"); code != 1 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(h.out.String(), "'--name=api work (2)'") {
		t.Fatalf("the command to run by hand should be printed: %q", h.out)
	}
	if got := latestIDs(t, h, "default"); got[0] != "s1" {
		t.Fatalf("the save changed although nothing opened: %v", got)
	}
}

func TestContextWarningPointsAtHandoff(t *testing.T) {
	h := testApp(t, term.Exact)
	h.app.Executable = "/bin/ccshift"
	var notes []string
	var clicks [][]string
	h.app.Notify = func(_, body string, click []string) { notes = append(notes, body); clicks = append(clicks, click) }
	h.status(t, statusJSON("s1", "api work", 72))
	if len(notes) != 1 || !strings.Contains(notes[0], "ccshift handoff "+target.Code("s1")) {
		t.Fatalf("notes = %q", notes)
	}
	if strings.Join(clicks[0], " ") != "/bin/ccshift handoff s1 --auto" {
		t.Fatalf("click = %q", clicks[0])
	}
}

type rawTerm struct {
	*fakeTerm
	got *[][]string
}

func (r rawTerm) OpenWindow(_ context.Context, _ string, ls []term.Launch) error {
	for _, l := range ls {
		*r.got = append(*r.got, l.Argv)
	}
	return nil
}

func TestLaunchedSessionsDoNotInheritAClaudeSession(t *testing.T) {
	h, _ := handoffApp(t)
	var got [][]string
	h.app.Terms = []term.Adapter{rawTerm{h.term, &got}}
	h.must(t, "handoff", "1")
	h.sessions = nil
	h.addTranscript(t, "s2")
	h.must(t, "restore")
	if len(got) < 2 {
		t.Fatalf("launches = %q", got)
	}
	for _, argv := range got {
		line := strings.Join(argv, " ")
		if argv[0] != "env" || !strings.Contains(line, "-u CLAUDE_CODE_CHILD_SESSION") || !strings.Contains(line, "-u CLAUDECODE") || !strings.Contains(line, " /bin/claude ") {
			t.Fatalf("launch is not isolated from the calling session: %q", argv)
		}
	}
	// The commands printed for running by hand stay plain.
	h.term.openErr = errors.New("no")
	h.app.Terms = []term.Adapter{h.term}
	h.sessions = nil
	h.run("restore")
	if strings.Contains(h.out.String(), "env -u") {
		t.Fatalf("printed commands should be plain: %q", h.out)
	}
}

func TestHandoffWhenTheTerminalOnlyPrintsTheCommand(t *testing.T) {
	h, _ := handoffApp(t)
	h.term.openErr = term.ErrPrinted
	out := h.must(t, "handoff", "1")
	if got := latestIDs(t, h, "default"); got[0] != "s1" {
		t.Fatalf("nothing was opened, but the save was rewritten: %v", got)
	}
	var lineage map[string]string
	h.app.Store.Load("lineage.json", &lineage)
	if len(lineage) != 0 || !strings.Contains(out, "brief is at") {
		t.Fatalf("lineage=%v out=%q", lineage, out)
	}
}

func TestRestoreWhenTheTerminalOnlyPrintsTheCommand(t *testing.T) {
	h := testApp(t, term.Exact)
	saveSnap(t, h, "work", "s1")
	h.term.openErr = term.ErrPrinted
	out := h.must(t, "restore")
	if strings.Contains(out, "Run these yourself") || strings.Contains(h.err.String(), "couldn't open") {
		t.Fatalf("the adapter already printed the commands: out=%q err=%q", out, h.err)
	}
}

func TestHandoffDoesNotFallBackToTheSaveForAnAmbiguousName(t *testing.T) {
	h, _ := handoffApp(t)
	h.sessions[1].name = "api work"
	if code := h.run("handoff", "api work"); code != 1 || !strings.Contains(h.err.String(), "more than one") {
		t.Fatalf("exit %d, %q", code, h.err)
	}
	if len(h.term.opened) != 0 {
		t.Fatal("an ambiguous name must not hand anything off")
	}
}

func TestSecondHandoffGetsANewNameAndPlainSaveKeepsTheOldOneOut(t *testing.T) {
	h, _ := handoffApp(t)
	h.must(t, "handoff", "api work")
	ids := []string{"new-2"}
	h.app.NewID = func() string { id := ids[0]; return id }
	// The first new session is live now; the old one is still open too.
	h.sessions = append(h.sessions, liveSession{id: "new-id", cwd: "/w/api", name: "api work (2)", pid: 9, minute: 5})
	h.must(t, "handoff", "api work")
	if got := h.term.opened[1][0].Title; got != "api work (3)" {
		t.Fatalf("second handoff name = %q", got)
	}
	h.must(t, "save")
	for _, id := range latestIDs(t, h, "default") {
		if id == "s1" {
			t.Fatal("a plain save put the handed-off session back")
		}
	}
	order, _ := h.app.Store.Order()
	for _, id := range order["default"] {
		if id == "s1" {
			t.Fatalf("order still lists the old session: %v", order["default"])
		}
	}
}

func TestHandoffReuseKeepsAnEditedBrief(t *testing.T) {
	h, prompts := handoffApp(t)
	h.app.Editor = func(path string) error { return os.WriteFile(path, []byte("my edited brief\n"), 0o644) }
	out := h.must(t, "handoff", "1", "--edit", "--no-launch")
	if !strings.Contains(out, "--reuse") {
		t.Fatalf("out = %q", out)
	}
	h.must(t, "handoff", "1", "--reuse")
	if len(*prompts) != 1 {
		t.Fatalf("the brief was written again: %d runs", len(*prompts))
	}
	b, _ := os.ReadFile(filepath.Join(h.app.Store.Dir, "handoffs", "s1.md"))
	if string(b) != "my edited brief\n" || len(h.term.opened) != 1 {
		t.Fatalf("brief=%q opened=%d", b, len(h.term.opened))
	}
	if code := h.run("handoff", "2", "--reuse"); code != 1 {
		t.Fatal("--reuse with no brief on disk should fail")
	}
}

func TestHandoffCloseOldWithoutATab(t *testing.T) {
	h, _ := handoffApp(t)
	var tabs []term.Launch
	var closed []term.TabID
	h.app.Terms = []term.Adapter{tabTerm{fakeTerm: h.term, tabs: &tabs, closed: &closed}}
	h.sessions[0].pane = ""
	out := h.must(t, "handoff", "api work", "--close-old")
	if len(closed) != 0 || !strings.Contains(out, "no tab was found") {
		t.Fatalf("closed=%v out=%q", closed, out)
	}
}

func TestHandoffKeepsTheOldSessionsModel(t *testing.T) {
	h, _ := handoffApp(t)
	p := filepath.Join(h.projects, "-w-api", "s1.jsonl")
	f, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString(`{"type":"assistant","message":{"model":"claude-opus-5-5","content":[{"type":"text","text":"ok"}]}}` + "\n")
	f.Close()
	h.must(t, "handoff", "1")
	argv := h.term.opened[0][0].Argv
	want := []string{"/bin/claude", "--session-id", "new-id", "--name=api work (2)", "--model", "claude-opus-5-5"}
	if len(argv) != 7 || !reflect.DeepEqual(argv[:6], want) || !strings.HasPrefix(argv[6], "Read ") {
		t.Fatalf("argv = %q", argv)
	}
}
