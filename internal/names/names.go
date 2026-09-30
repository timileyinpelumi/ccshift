// Package names decides what a session is called: a name the user set, the name Claude has,
// or one generated from the repository, branch and Claude's own title for the session.
package names

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/timileyinpelumi/ccshift/internal/claude"
)

const shortLimit = 36

type Git struct {
	Repo     string // main repository's directory name, or the working directory's name outside git
	Branch   string
	Default  string // the default branch, when it can be told
	Detached bool   // inside a repository with no branch checked out (rebase, bisect)
}

var versionPrefix = regexp.MustCompile(`(?i)^v\d*$`)

// Rules say what counts as a ticket id in a branch name.
type Rules struct {
	Ticket *regexp.Regexp  // with two groups (prefix, number), or none to use the whole match
	Ignore map[string]bool // lower-case prefixes that are not ticket prefixes, such as "node" or "fix"
}

func (r Rules) ticket(branch string) string {
	if r.Ticket == nil {
		return ""
	}
	for _, m := range r.Ticket.FindAllStringSubmatch(branch, -1) {
		if len(m) < 3 {
			return strings.ToUpper(m[0])
		}
		prefix := strings.ToLower(m[1])
		if r.Ignore[prefix] || versionPrefix.MatchString(prefix) {
			continue
		}
		return strings.ToUpper(m[1]) + "-" + m[2]
	}
	return ""
}

func (g Git) onDefault() bool {
	if g.Branch == "" {
		return true
	}
	if g.Default != "" {
		return g.Branch == g.Default
	}
	switch g.Branch {
	case "main", "master", "develop", "trunk":
		return true
	}
	return false
}

// NeedsTitle reports whether Generate would use the session's title, so callers can skip reading it.
func NeedsTitle(g Git, r Rules) bool {
	return r.ticket(g.Branch) == "" && g.onDefault()
}

// Generate builds a name. First match wins: repo and ticket, repo and branch,
// repo and the session's title, repo alone.
func Generate(g Git, aiTitle string, r Rules) string {
	if t := r.ticket(g.Branch); t != "" {
		return g.Repo + " · " + t
	}
	if !g.onDefault() {
		return g.Repo + " · " + g.Branch
	}
	if t := Short(aiTitle); t != "" {
		return g.Repo + " · " + t
	}
	return g.Repo
}

// Short trims a title to a length that fits a tab, cutting at a word where it can.
func Short(title string) string {
	words := strings.Fields(title)
	out := ""
	for _, w := range words {
		next := w
		if out != "" {
			next = out + " " + w
		}
		if len([]rune(next)) > shortLimit {
			break
		}
		out = next
	}
	if out == "" && len(words) > 0 {
		return string([]rune(words[0])[:shortLimit])
	}
	return out
}

// Entry is what ccshift remembers about one session's name.
type Entry struct {
	User        string `json:"user,omitempty"`          // set with ccshift rename
	ClaudeAtSet string `json:"claude_at_set,omitempty"` // Claude's name when User was set
	Applied     string `json:"applied,omitempty"`       // a generated name ccshift passed to Claude
	Title       string `json:"title,omitempty"`         // last title Claude gave the session
	Base        string `json:"base,omitempty"`          // last generated name, before numbering
	Num         int    `json:"num,omitempty"`           // this session's number among those sharing Base
	Touched     int64  `json:"touched,omitempty"`       // when ccshift last set User or Applied
}

// Resolve picks between a name the user set and a generated one. It returns the name (empty when
// it should be generated), whether it is generated, and the entry with whatever no longer applies cleared.
func Resolve(claudeName, cwd string, e Entry) (string, bool, Entry) {
	// A generated name that ccshift handed to Claude on restore is still a generated name.
	custom := claudeName != "" && !claude.IsDefaultName(claudeName, cwd) && claudeName != e.Applied
	if e.User != "" {
		// Claude now has the name, or the user renamed the session inside Claude afterwards.
		if claudeName == e.User || (custom && claudeName != e.ClaudeAtSet) {
			e.User, e.ClaudeAtSet, e.Applied = "", "", ""
			return claudeName, false, e
		}
		return e.User, false, e
	}
	if custom {
		e.Applied = ""
		return claudeName, false, e
	}
	return "", true, e
}

// Number gives sessions that share a generated name a number each. A session keeps the number it
// had, so closing one does not renumber the rest; a new one takes the lowest free number.
// taken lists names already used by sessions with a name the user chose.
func Number(bases []string, prev []Entry, taken map[string]bool) []int {
	nums := make([]int, len(bases))
	used := map[string]map[int]bool{}
	claim := func(base string, n int) {
		if used[base] == nil {
			used[base] = map[int]bool{}
		}
		used[base][n] = true
	}
	for base := range taken {
		claim(base, 1)
	}
	for i, base := range bases {
		if p := prev[i]; p.Base == base && p.Num > 0 && !used[base][p.Num] {
			nums[i] = p.Num
			claim(base, p.Num)
		}
	}
	for i, base := range bases {
		if nums[i] != 0 {
			continue
		}
		n := 1
		for used[base][n] {
			n++
		}
		nums[i] = n
		claim(base, n)
	}
	return nums
}

func Numbered(base string, n int) string {
	if n <= 1 {
		return base
	}
	return fmt.Sprintf("%s #%d", base, n)
}
