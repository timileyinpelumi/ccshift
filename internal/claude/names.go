package claude

import (
	"path/filepath"
	"regexp"
	"strings"
)

var defaultSuffix = regexp.MustCompile(`^-[0-9a-f]{2}$`)

// IsDefaultName reports whether name is Claude's generated display name for cwd, like "personal-cd".
func IsDefaultName(name, cwd string) bool {
	base := filepath.Base(cwd)
	return strings.HasPrefix(name, base) && defaultSuffix.MatchString(name[len(base):])
}
