package claude

import "testing"

func TestIsDefaultName(t *testing.T) {
	cases := []struct {
		name, cwd string
		want      bool
	}{
		{"personal-cd", "/home/x/dev/personal", true},
		{"acme-d7", "/home/x/dev/acme", true},
		{"acme-dz", "/home/x/dev/acme", false},
		{"PAY 2193", "/home/x/dev/acme", false},
		{"acme · PAY-2193", "/home/x/dev/acme", false},
		{"", "/home/x/dev/acme", false},
	}
	for _, c := range cases {
		if got := IsDefaultName(c.name, c.cwd); got != c.want {
			t.Errorf("IsDefaultName(%q, %q) = %v", c.name, c.cwd, got)
		}
	}
}
