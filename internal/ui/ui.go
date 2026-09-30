// Package ui styles command output when it goes to a terminal. Without a terminal, or with
// NO_COLOR set, every function writes the same words plain, so scripts and tests see stable text.
package ui

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type Style string

const (
	None  Style = ""
	Bold  Style = "1"
	Dim   Style = "2"
	Red   Style = "31"
	Green Style = "32"
	Amber Style = "38;5;215"
	Cyan  Style = "36"
	Code  Style = "1;38;5;215"
)

type UI struct {
	W     io.Writer
	Color bool
}

// For styles w when it is a terminal that takes colour.
func For(w io.Writer, getenv func(string) string) *UI {
	u := &UI{W: w}
	f, ok := w.(*os.File)
	if !ok || getenv("NO_COLOR") != "" || getenv("TERM") == "dumb" {
		return u
	}
	if fi, err := f.Stat(); err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return u
	}
	u.Color = enableVT(f)
	return u
}

func (u *UI) Paint(s Style, text string) string {
	if !u.Color || s == None || text == "" {
		return text
	}
	return "\033[" + string(s) + "m" + text + "\033[0m"
}

var escape = regexp.MustCompile("\033\\[[0-9;?]*[A-Za-z]")

// Strip removes styling, for measuring and for tests.
func Strip(s string) string { return escape.ReplaceAllString(s, "") }

func (u *UI) line(mark string, s Style, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if u.Color {
		msg = u.Paint(s, mark) + " " + msg
	}
	fmt.Fprintln(u.W, msg)
}

// OK reports something that worked.
func (u *UI) OK(format string, args ...any) { u.line("✓", Green, format, args...) }

// Info introduces what happens next.
func (u *UI) Info(format string, args ...any) { u.line("›", Cyan, format, args...) }

// Warn reports something the user should look at.
func (u *UI) Warn(format string, args ...any) { u.line("!", Amber, format, args...) }

// Fail reports something that did not work.
func (u *UI) Fail(format string, args ...any) { u.line("✗", Red, format, args...) }

const wordmark = `                  __    _ ______
  _______________/ /_  (_) __/ /_
 / ___/ ___/ ___/ __ \/ / /_/ __/
/ /__/ /__(__  ) / / / / __/ /_
\___/\___/____/_/ /_/_/_/  \__/`

// Banner prints the wordmark, on terminals only.
func (u *UI) Banner() {
	if !u.Color {
		return
	}
	fmt.Fprintf(u.W, "\n%s\n%s\n\n", u.Paint(Amber, wordmark), u.Paint(Dim, "Keep your Claude Code sessions across restarts."))
}

var frames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Spin shows a spinner with the time taken while fn runs, then a ✓ or ✗ in its place. On a
// plain writer it prints nothing, so callers keep their own plain lines. fn must not write to W.
func (u *UI) Spin(label string, fn func() error) error {
	if !u.Color {
		return fn()
	}
	start := time.Now()
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		t := time.NewTicker(80 * time.Millisecond)
		defer t.Stop()
		for i := 0; ; i++ {
			fmt.Fprintf(u.W, "\r\033[K%s %s%s", u.Paint(Cyan, frames[i%len(frames)]), label, u.took(start))
			select {
			case <-stop:
				return
			case <-t.C:
			}
		}
	}()
	err := fn()
	close(stop)
	wg.Wait()
	mark := u.Paint(Green, "✓")
	if err != nil {
		mark = u.Paint(Red, "✗")
	}
	fmt.Fprintf(u.W, "\r\033[K%s %s%s\n", mark, label, u.took(start))
	return err
}

func (u *UI) took(start time.Time) string {
	d := time.Since(start)
	if d < time.Second {
		return ""
	}
	return u.Paint(Dim, fmt.Sprintf("  %ds", int(d.Seconds())))
}

type Cell struct {
	Text  string
	Style Style
}

// Table lines up columns by their visible width, which text/tabwriter cannot do once cells
// carry escape codes.
type Table struct {
	u    *UI
	rows [][]Cell
}

func (u *UI) Table(header ...string) *Table {
	t := &Table{u: u}
	if len(header) == 0 {
		return t
	}
	row := make([]Cell, len(header))
	for i, h := range header {
		row[i] = Cell{Text: h, Style: Dim}
	}
	t.rows = append(t.rows, row)
	return t
}

func (t *Table) Row(cells ...Cell) { t.rows = append(t.rows, cells) }

func (t *Table) Flush() {
	var widths []int
	for _, r := range t.rows {
		for i, c := range r {
			if i >= len(widths) {
				widths = append(widths, 0)
			}
			widths[i] = max(widths[i], utf8.RuneCountInString(c.Text))
		}
	}
	for _, r := range t.rows {
		var b strings.Builder
		for i, c := range r {
			text := c.Text
			if i < len(r)-1 {
				text += strings.Repeat(" ", widths[i]-utf8.RuneCountInString(c.Text)+2)
				b.WriteString(t.u.Paint(c.Style, c.Text) + text[len(c.Text):])
				continue
			}
			b.WriteString(t.u.Paint(c.Style, c.Text))
		}
		fmt.Fprintln(t.u.W, b.String())
	}
	t.rows = nil
}
