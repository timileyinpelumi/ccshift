package cli

import (
	"fmt"
	"time"
)

// supportURL is where the occasional note points. Empty turns the note off.
var supportURL = "https://paystack.shop/pay/ne1sknbf3o"

const supportEvery = 30 * 24 * time.Hour

// supportNote prints one line asking for support, at most once a month, after a command that
// did something useful. Never from hooks, the statusline or JSON output.
func (a *App) supportNote() {
	if supportURL == "" || !a.Config.SupportNote {
		return
	}
	var last struct {
		Shown int64 `json:"shown"`
	}
	a.Store.Load("support.json", &last)
	now := a.Now()
	if last.Shown != 0 && now.Sub(time.Unix(last.Shown, 0)) < supportEvery {
		return
	}
	last.Shown = now.Unix()
	if a.Store.Put("support.json", last) != nil {
		return
	}
	fmt.Fprintf(a.Out, "\n%s\n", a.color("2", "ccshift is free and built in spare time. If it saves you time, you can support it at "+supportURL+" (support_note = false hides this)."))
}
