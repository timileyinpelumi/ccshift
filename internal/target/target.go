package target

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/timileyinpelumi/ccshift/internal/layout"
)

// ErrNoMatch is wrapped by the error Resolve returns when no running session fits a name or id.
var ErrNoMatch = errors.New("no running session matches")

func Resolve(spec string, items []layout.Item, selfAncestors []int) (layout.Item, error) {
	if spec == "." {
		anc := map[int]bool{}
		for _, p := range selfAncestors {
			anc[p] = true
		}
		for _, it := range items {
			if anc[it.Session.PID] {
				return it, nil
			}
		}
		return layout.Item{}, errors.New("not running inside a Claude session; use a name or number from ccshift ls")
	}
	if n, err := strconv.Atoi(spec); err == nil {
		if n < 1 || n > len(items) {
			return layout.Item{}, fmt.Errorf("no session number %d; see ccshift ls", n)
		}
		return items[n-1], nil
	}
	lower := strings.ToLower(spec)
	matchers := []func(layout.Item) bool{
		func(it layout.Item) bool { return strings.ToLower(it.Session.Name) == lower },
		func(it layout.Item) bool { return strings.HasPrefix(strings.ToLower(it.Session.Name), lower) },
		func(it layout.Item) bool { return len(spec) >= 4 && strings.HasPrefix(it.Session.ID, spec) },
	}
	for _, match := range matchers {
		var found []layout.Item
		for _, it := range items {
			if match(it) {
				found = append(found, it)
			}
		}
		switch len(found) {
		case 0:
			continue
		case 1:
			return found[0], nil
		default:
			names := make([]string, len(found))
			for i, it := range found {
				names[i] = it.Session.Name
			}
			return layout.Item{}, fmt.Errorf("%q matches more than one session: %s", spec, strings.Join(names, ", "))
		}
	}
	return layout.Item{}, fmt.Errorf("%w %q", ErrNoMatch, spec)
}
