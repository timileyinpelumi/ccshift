package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/timileyinpelumi/ccshift/internal/term"
)

func TestSupportNoteShowsRarely(t *testing.T) {
	old := supportURL
	supportURL = "https://paystack.com/pay/example"
	defer func() { supportURL = old }()
	h := testApp(t, term.Exact)
	saveSnap(t, h, "work", "s1")
	out := h.must(t, "restore")
	if !strings.Contains(out, "https://paystack.com/pay/example") {
		t.Fatalf("first restore should show the note: %q", out)
	}
	h.sessions = nil
	h.at(24 * time.Hour)
	if out := h.must(t, "restore"); strings.Contains(out, "paystack") {
		t.Fatal("the note showed again within a month")
	}
	h.at(31 * 24 * time.Hour)
	if out := h.must(t, "restore"); !strings.Contains(out, "paystack") {
		t.Fatal("the note should come back after a month")
	}
	h.app.Config.SupportNote = false
	h.at(90 * 24 * time.Hour)
	if out := h.must(t, "restore"); strings.Contains(out, "paystack") {
		t.Fatal("support_note = false should turn it off")
	}
}

func TestSupportNoteNeedsAURL(t *testing.T) {
	old := supportURL
	supportURL = ""
	defer func() { supportURL = old }()
	h := testApp(t, term.Exact)
	saveSnap(t, h, "work", "s1")
	if out := h.must(t, "restore"); strings.Contains(out, "support") {
		t.Fatalf("out = %q", out)
	}
}
