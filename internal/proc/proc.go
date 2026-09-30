// Package proc answers questions about other processes. Linux reads /proc; macOS asks ps;
// Windows uses the process snapshot API and cannot read another process's environment.
package proc

import "errors"

// ErrUnsupported is returned where this operating system cannot answer.
var ErrUnsupported = errors.New("not supported on this operating system")

// Ancestors returns the parents of pid, nearest first.
func Ancestors(pid int) []int {
	var out []int
	for range 64 {
		ppid, err := ParentPID(pid)
		if err != nil || ppid <= 1 {
			break
		}
		out = append(out, ppid)
		pid = ppid
	}
	return out
}
