package term

import "testing"

func TestShellJoin(t *testing.T) {
	got := ShellJoin([]string{"/usr/bin/claude", "--resume", "abc-123", "-n", "acme · PAY-2193", "it's", ""})
	want := `/usr/bin/claude --resume abc-123 -n 'acme · PAY-2193' 'it'\''s' ''`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}
