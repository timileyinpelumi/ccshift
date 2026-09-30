package names

import (
	"regexp"
	"testing"
)

var rules = Rules{
	Ticket: regexp.MustCompile(`(?i)\b([A-Z][A-Z0-9]{1,5})[-_](\d{2,})\b`),
	Ignore: map[string]bool{"fix": true, "node": true, "react": true, "sha": true, "hotfix": true},
}

func TestGenerate(t *testing.T) {
	cases := []struct {
		git   Git
		title string
		want  string
	}{
		{Git{Repo: "acme", Branch: "feature/pay-2193-fix-login", Default: "main"}, "whatever", "acme · PAY-2193"},
		{Git{Repo: "acme", Branch: "pay_2193", Default: "main"}, "", "acme · PAY-2193"},
		{Git{Repo: "cvx", Branch: "compose-v2", Default: "main"}, "whatever", "cvx · compose-v2"},
		{Git{Repo: "trueset", Branch: "main", Default: "main"}, "Add dark mode and a better tagline for the whole site", "trueset · Add dark mode and a better tagline"},
		{Git{Repo: "trueset", Branch: "master", Default: ""}, "Fix rate limits", "trueset · Fix rate limits"},
		{Git{Repo: "trueset", Branch: "develop", Default: ""}, "Fix rate limits", "trueset · Fix rate limits"},
		{Git{Repo: "trueset", Branch: "main", Default: "main"}, "", "trueset"},
		{Git{Repo: "notes"}, "Plan the trip", "notes · Plan the trip"},
		{Git{Repo: "notes"}, "", "notes"},
		{Git{Repo: "api", Branch: "release-2", Default: "main"}, "", "api · release-2"},
		// Branch names that look like tickets but are not.
		{Git{Repo: "web", Branch: "node-18-upgrade", Default: "main"}, "", "web · node-18-upgrade"},
		{Git{Repo: "web", Branch: "renovate/react-18.x", Default: "main"}, "", "web · renovate/react-18.x"},
		{Git{Repo: "web", Branch: "sha-256", Default: "main"}, "", "web · sha-256"},
		{Git{Repo: "web", Branch: "hotfix-2024", Default: "main"}, "", "web · hotfix-2024"},
		{Git{Repo: "web", Branch: "v2-10", Default: "main"}, "", "web · v2-10"},
		{Git{Repo: "web", Branch: "fix-123", Default: "main"}, "", "web · fix-123"},
		{Git{Repo: "web", Branch: "fix-123-for-ops-664", Default: "main"}, "", "web · OPS-664"},
	}
	for _, c := range cases {
		if got := Generate(c.git, c.title, rules); got != c.want {
			t.Errorf("Generate(%+v, %q) = %q, want %q", c.git, c.title, got, c.want)
		}
	}
	if !NeedsTitle(Git{Repo: "x", Branch: "main"}, rules) || NeedsTitle(Git{Repo: "x", Branch: "pay-2193"}, rules) || NeedsTitle(Git{Repo: "x", Branch: "topic"}, rules) {
		t.Fatal("NeedsTitle should be true only when no ticket or branch name applies")
	}
}

func TestShort(t *testing.T) {
	if got := Short("Supercalifragilisticexpialidocious-and-then-some-more-text"); len([]rune(got)) > 40 {
		t.Fatalf("one long word not cut: %q", got)
	}
	if got := Short("  spaced   out  "); got != "spaced out" {
		t.Fatalf("got %q", got)
	}
}

func TestResolve(t *testing.T) {
	cwd := "/home/u/dev/acme"
	cases := []struct {
		name      string
		claude    string
		entry     Entry
		wantName  string
		wantGen   bool
		wantEntry Entry
	}{
		{"default claude name", "acme-d7", Entry{}, "", true, Entry{}},
		{"no claude name", "", Entry{}, "", true, Entry{}},
		{"custom claude name", "PAY 2193", Entry{}, "PAY 2193", false, Entry{}},
		{"ccshift rename pending", "acme-d7", Entry{User: "login fix", ClaudeAtSet: "acme-d7"}, "login fix", false, Entry{User: "login fix", ClaudeAtSet: "acme-d7"}},
		{"ccshift rename applied by resume", "login fix", Entry{User: "login fix", ClaudeAtSet: "acme-d7"}, "login fix", false, Entry{}},
		{"renamed in claude afterwards", "auth work", Entry{User: "login fix", ClaudeAtSet: "acme-d7"}, "auth work", false, Entry{}},
		{"ccshift rename over a custom name", "PAY 2193", Entry{User: "login fix", ClaudeAtSet: "PAY 2193"}, "login fix", false, Entry{User: "login fix", ClaudeAtSet: "PAY 2193"}},
		{"generated name applied by restore", "acme · old-branch", Entry{Applied: "acme · old-branch"}, "", true, Entry{Applied: "acme · old-branch"}},
		{"renamed in claude after a restore", "mine", Entry{Applied: "acme · old-branch"}, "mine", false, Entry{}},
		{"caches survive", "acme-d7", Entry{Title: "T", Base: "b", Num: 2}, "", true, Entry{Title: "T", Base: "b", Num: 2}},
	}
	for _, c := range cases {
		name, gen, entry := Resolve(c.claude, cwd, c.entry)
		if name != c.wantName || gen != c.wantGen || entry != c.wantEntry {
			t.Errorf("%s: got (%q, %v, %+v)", c.name, name, gen, entry)
		}
	}
}

func TestNumberIsStable(t *testing.T) {
	bases := []string{"api", "web", "api", "api"}
	prev := make([]Entry, 4)
	nums := Number(bases, prev, nil)
	if want := []int{1, 1, 2, 3}; !equal(nums, want) {
		t.Fatalf("first numbering = %v", nums)
	}
	// The first session exits. The others keep their numbers.
	nums = Number([]string{"web", "api", "api"}, []Entry{{Base: "web", Num: 1}, {Base: "api", Num: 2}, {Base: "api", Num: 3}}, nil)
	if want := []int{1, 2, 3}; !equal(nums, want) {
		t.Fatalf("after an exit = %v", nums)
	}
	// A new one takes the lowest free number.
	nums = Number([]string{"api", "api", "api"}, []Entry{{Base: "api", Num: 2}, {Base: "api", Num: 3}, {}}, nil)
	if want := []int{2, 3, 1}; !equal(nums, want) {
		t.Fatalf("new session = %v", nums)
	}
	// A number from another base does not carry over.
	nums = Number([]string{"api", "api"}, []Entry{{Base: "old", Num: 5}, {Base: "api", Num: 1}}, nil)
	if want := []int{2, 1}; !equal(nums, want) {
		t.Fatalf("base changed = %v", nums)
	}
	// A name the user chose counts as taken.
	nums = Number([]string{"api"}, []Entry{{}}, map[string]bool{"api": true})
	if want := []int{2}; !equal(nums, want) {
		t.Fatalf("taken by a user name = %v", nums)
	}
	if Numbered("api", 1) != "api" || Numbered("api", 3) != "api #3" {
		t.Fatal("Numbered")
	}
}

func equal(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
