package ui

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestPlainOutputHasNoEscapes(t *testing.T) {
	var b bytes.Buffer
	u := &UI{W: &b}
	u.OK("saved %d sessions", 3)
	u.Info("opening")
	u.Warn("careful")
	u.Banner()
	if got := b.String(); got != "saved 3 sessions\nopening\ncareful\n" {
		t.Fatalf("got %q", got)
	}
}

func TestColorOutputMarksLines(t *testing.T) {
	var b bytes.Buffer
	u := &UI{W: &b, Color: true}
	u.OK("saved")
	if !strings.Contains(b.String(), "✓") || !strings.Contains(b.String(), "\033[") {
		t.Fatalf("got %q", b.String())
	}
}

func TestTableAlignsOnVisibleWidth(t *testing.T) {
	var b bytes.Buffer
	u := &UI{W: &b, Color: true}
	tb := u.Table("#", "NAME", "CTX")
	tb.Row(Cell{Text: "1"}, Cell{Text: "api · PAY-2193", Style: Bold}, Cell{Text: "78%!", Style: Red})
	tb.Row(Cell{Text: "2"}, Cell{Text: "web", Style: Bold}, Cell{Text: "12%"})
	tb.Flush()
	lines := strings.Split(strings.TrimRight(Strip(b.String()), "\n"), "\n")
	want := []string{"#  NAME            CTX", "1  api · PAY-2193  78%!", "2  web             12%"}
	for i, w := range want {
		if strings.TrimRight(lines[i], " ") != w {
			t.Errorf("line %d = %q, want %q", i, lines[i], w)
		}
	}
	var p bytes.Buffer
	pt := (&UI{W: &p}).Table("#", "NAME")
	pt.Row(Cell{Text: "1"}, Cell{Text: "web", Style: Bold})
	pt.Flush()
	if p.String() != "#  NAME\n1  web\n" {
		t.Fatalf("plain table = %q", p.String())
	}
}

func TestSpinReportsTheResult(t *testing.T) {
	var b bytes.Buffer
	u := &UI{W: &b, Color: true}
	if err := u.Spin("Writing the brief", func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(Strip(b.String()), "✓ Writing the brief") {
		t.Fatalf("got %q", b.String())
	}
	b.Reset()
	boom := errors.New("boom")
	if err := u.Spin("Opening tabs", func() error { return boom }); err != boom {
		t.Fatal(err)
	}
	if !strings.Contains(Strip(b.String()), "✗ Opening tabs") {
		t.Fatalf("got %q", b.String())
	}
	b.Reset()
	(&UI{W: &b}).Spin("Quiet", func() error { return nil })
	if b.Len() != 0 {
		t.Fatalf("plain spin wrote %q", b.String())
	}
}
