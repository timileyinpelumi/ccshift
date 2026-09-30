package proc

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

var Root = "/proc"

// Terminal emulators give a pts; the Linux console gives /dev/ttyN.
var ttyPath = regexp.MustCompile(`^/dev/(pts/\d+|tty\d+)$`)

func path(pid int, name string) string {
	return filepath.Join(Root, strconv.Itoa(pid), name)
}

func Environ(pid int) (map[string]string, error) {
	b, err := os.ReadFile(path(pid, "environ"))
	if err != nil {
		return nil, err
	}
	env := map[string]string{}
	for _, kv := range bytes.Split(b, []byte{0}) {
		k, v, ok := strings.Cut(string(kv), "=")
		if ok && k != "" {
			env[k] = v
		}
	}
	return env, nil
}

// The comm field is in parens and may itself contain spaces or ')', so split after the last ')'.
func parseStat(line string) (ppid int, start string, err error) {
	i := strings.LastIndexByte(line, ')')
	if i < 0 {
		return 0, "", errors.New("proc: bad stat line")
	}
	f := strings.Fields(line[i+1:])
	if len(f) < 20 {
		return 0, "", errors.New("proc: bad stat line")
	}
	ppid, err = strconv.Atoi(f[1])
	return ppid, f[19], err
}

func readStat(pid int) (int, string, error) {
	b, err := os.ReadFile(path(pid, "stat"))
	if err != nil {
		return 0, "", err
	}
	return parseStat(string(b))
}

func ParentPID(pid int) (int, error) {
	ppid, _, err := readStat(pid)
	return ppid, err
}

func StartTime(pid int) (string, error) {
	_, start, err := readStat(pid)
	return start, err
}

func Comm(pid int) (string, error) {
	b, err := os.ReadFile(path(pid, "comm"))
	return strings.TrimSpace(string(b)), err
}

func Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// TTY returns the terminal device the process reads from, or "" if it has none.
func TTY(pid int) string {
	t, err := os.Readlink(filepath.Join(Root, strconv.Itoa(pid), "fd", "0"))
	if err != nil || !ttyPath.MatchString(t) {
		return ""
	}
	return t
}

func HasTTY(pid int) bool { return TTY(pid) != "" }
