package proc

import (
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

func ps(pid int, format string, extra ...string) (string, error) {
	args := append(extra, "-o", format+"=", "-p", strconv.Itoa(pid))
	out, err := exec.Command("ps", args...).Output()
	return strings.TrimSpace(string(out)), err
}

var envToken = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// Environ reads the environment ps shows for a process owned by the same user. Values with
// spaces in them come out cut short, which is fine for the terminal ids ccshift looks for.
func Environ(pid int) (map[string]string, error) {
	out, err := ps(pid, "command", "eww")
	if err != nil {
		return nil, err
	}
	env := map[string]string{}
	for _, tok := range strings.Fields(out) {
		if envToken.MatchString(tok) {
			k, v, _ := strings.Cut(tok, "=")
			env[k] = v
		}
	}
	return env, nil
}

func ParentPID(pid int) (int, error) {
	out, err := ps(pid, "ppid")
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(out)
}

func StartTime(int) (string, error) { return "", ErrUnsupported }

func Comm(pid int) (string, error) {
	out, err := ps(pid, "comm")
	return filepath.Base(out), err
}

func Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

func TTY(pid int) string {
	out, err := ps(pid, "tty")
	if err != nil || out == "" || strings.HasPrefix(out, "?") {
		return ""
	}
	return "/dev/" + out
}

func HasTTY(pid int) bool { return TTY(pid) != "" }
