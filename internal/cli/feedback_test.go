package cli

import (
	"strings"
	"testing"

	"github.com/timileyinpelumi/ccshift/internal/term"
)

func TestFeedbackOpensTheFormWithTheSetupFilledIn(t *testing.T) {
	h := testApp(t, term.Exact)
	var opened []string
	h.app.OpenURL = func(u string) error { opened = append(opened, u); return nil }
	out := h.must(t, "feedback")
	if len(opened) != 1 || !strings.HasPrefix(opened[0], "https://www.timileyin.dev/ccshift/feedback?") ||
		!strings.Contains(opened[0], "os=linux") || !strings.Contains(opened[0], "terminal=fake") || !strings.Contains(out, opened[0]) {
		t.Fatalf("opened %q, out %q", opened, out)
	}
}
