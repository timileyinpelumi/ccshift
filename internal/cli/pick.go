package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"golang.org/x/term"

	"github.com/timileyinpelumi/ccshift/internal/layout"
	"github.com/timileyinpelumi/ccshift/internal/target"
	cterm "github.com/timileyinpelumi/ccshift/internal/term"
)

const (
	keyUp        = "up"
	keyDown      = "down"
	keyEnter     = "enter"
	keyEsc       = "esc"
	keyBackspace = "backspace"
	keyCtrlX     = "ctrl+x"
	keyCtrlC     = "ctrl+c"
)

type pickAction int

const (
	pickNone pickAction = iota
	pickOpen
	pickHandoff
	pickQuit
)

type pickItem struct {
	id, name, cwd, agent, detail string
}

// pickModel is the picker's state: the list, what has been typed, and which line is selected.
type pickModel struct {
	items []pickItem
	query string
	sel   int
}

func (m *pickModel) visible() []int {
	var out []int
	q := strings.ToLower(m.query)
	for i, it := range m.items {
		if q == "" || fuzzy(strings.ToLower(it.name+" "+it.cwd), q) || strings.EqualFold(target.Code(it.id), q) {
			out = append(out, i)
		}
	}
	return out
}

// fuzzy reports whether the letters of q appear in s in order.
func fuzzy(s, q string) bool {
	for _, r := range q {
		i := strings.IndexRune(s, r)
		if i < 0 {
			return false
		}
		s = s[i+utf8.RuneLen(r):]
	}
	return true
}

func (m *pickModel) key(k string) (pickAction, pickItem) {
	vis := m.visible()
	switch k {
	case keyUp:
		m.sel = max(0, m.sel-1)
	case keyDown:
		m.sel = min(max(0, len(vis)-1), m.sel+1)
	case keyBackspace:
		if r := []rune(m.query); len(r) > 0 {
			m.query = string(r[:len(r)-1])
			m.sel = 0
		}
	case keyEsc, keyCtrlC:
		return pickQuit, pickItem{}
	case keyEnter, keyCtrlX:
		if len(vis) == 0 {
			return pickNone, pickItem{}
		}
		it := m.items[vis[min(m.sel, len(vis)-1)]]
		if k == keyEnter {
			return pickOpen, it
		}
		return pickHandoff, it
	default:
		if utf8.RuneCountInString(k) == 1 {
			m.query += k
			m.sel = 0
		}
	}
	return pickNone, pickItem{}
}

func parseKeys(b []byte) []string {
	var out []string
	for len(b) > 0 {
		switch {
		case len(b) >= 3 && b[0] == 0x1b && b[1] == '[' && (b[2] == 'A' || b[2] == 'B'):
			out = append(out, map[byte]string{'A': keyUp, 'B': keyDown}[b[2]])
			b = b[3:]
			continue
		case b[0] == 0x1b:
			out = append(out, keyEsc)
		case b[0] == '\r' || b[0] == '\n':
			out = append(out, keyEnter)
		case b[0] == 0x7f || b[0] == 0x08:
			out = append(out, keyBackspace)
		case b[0] == 0x18:
			out = append(out, keyCtrlX)
		case b[0] == 0x03:
			out = append(out, keyCtrlC)
		case b[0] >= 0x20:
			r, n := utf8.DecodeRune(b)
			out = append(out, string(r))
			b = b[n:]
			continue
		}
		b = b[1:]
	}
	return out
}

// pickItems lists running sessions in tab order, then saved ones that are not running.
func (a *App) pickItems(v *view) []pickItem {
	usage, _ := a.Store.Context()
	var items []pickItem
	running := map[string]bool{}
	for _, it := range v.order {
		running[it.Session.ID] = true
		detail := "running"
		if it.Matched {
			detail += " · " + v.adapter.Name() + ":" + it.TabLabel
		}
		if e, ok := usage[it.Session.ID]; ok {
			detail += " · " + contextCell(e, true, a.Config.WarnThresholds)
		}
		items = append(items, pickItem{id: it.Session.ID, name: it.Session.Name, cwd: it.Session.CWD, agent: it.Session.Agent, detail: detail})
	}
	wss, _ := a.Store.Workspaces()
	for _, ws := range wss {
		snap, ok, err := a.Store.Latest(ws)
		if err != nil || !ok {
			continue
		}
		for _, e := range snap.Sessions {
			if !running[e.SessionID] {
				running[e.SessionID] = true
				items = append(items, pickItem{id: e.SessionID, name: layout.Title(e), cwd: e.CWD, agent: e.Agent, detail: "saved · " + ws})
			}
		}
	}
	return items
}

func (a *App) cmdPick(ctx context.Context, _ []string) error {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) || a.Interactive == nil || !a.Interactive() {
		return usageError{"the picker needs a terminal; see ccshift help"}
	}
	v, err := a.view(ctx, "")
	if err != nil {
		return err
	}
	m := &pickModel{items: a.pickItems(v)}
	if len(m.items) == 0 {
		fmt.Fprintln(a.Out, "No running or saved sessions yet.")
		return nil
	}
	old, err := term.MakeRaw(fd)
	if err != nil {
		return err
	}
	fmt.Fprint(a.Out, "\033[?1049h\033[?25l")
	restore := func() {
		fmt.Fprint(a.Out, "\033[?25h\033[?1049l")
		term.Restore(fd, old)
	}
	action, chosen := pickNone, pickItem{}
	buf := make([]byte, 64)
	for action == pickNone {
		a.drawPicker(m)
		n, err := os.Stdin.Read(buf)
		if err != nil {
			break
		}
		for _, k := range parseKeys(buf[:n]) {
			if action, chosen = m.key(k); action != pickNone {
				break
			}
		}
	}
	restore()
	switch action {
	case pickOpen:
		return a.resumeSession(ctx, chosen.id, chosen.cwd, chosen.agent)
	case pickHandoff:
		return a.cmdHandoff(ctx, []string{chosen.id})
	}
	return nil
}

func (a *App) drawPicker(m *pickModel) {
	_, rows, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || rows < 6 {
		rows = 24
	}
	var b strings.Builder
	b.WriteString("\033[H\033[2J")
	fmt.Fprintf(&b, "\033[1mccshift\033[0m  \033[2mtype to filter  ↑↓ move  Enter open  Ctrl+X hand off  Esc quit\033[0m\r\n\r\n")
	fmt.Fprintf(&b, "\033[33m›\033[0m %s\033[2m▏\033[0m\r\n\r\n", m.query)
	vis := m.visible()
	if len(vis) == 0 {
		b.WriteString("\033[2mNothing matches.\033[0m\r\n")
	}
	room := rows - 5
	start := 0
	if m.sel >= room {
		start = m.sel - room + 1
	}
	for i := start; i < len(vis) && i < start+room; i++ {
		it := m.items[vis[i]]
		code := target.Code(it.id)
		line := fmt.Sprintf(" \033[2m%s\033[0m  %-34s \033[2m%s\033[0m", code, cterm.Truncate(it.name, 34), it.detail)
		if i == m.sel {
			line = "\033[7m" + fmt.Sprintf(" %s  %-34s %s", code, cterm.Truncate(it.name, 34), it.detail) + "\033[0m"
		}
		b.WriteString(line + "\r\n")
	}
	fmt.Fprint(a.Out, b.String())
}
