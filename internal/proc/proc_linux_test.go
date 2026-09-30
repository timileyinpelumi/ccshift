//go:build linux

package proc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseStat(t *testing.T) {
	// comm may contain spaces and ')'
	line := "4242 (weird) name) S 17 4242 4242 0 -1 4194560 1 0 0 0 0 0 0 0 20 0 1 0 6883643 1000 10 0\n"
	ppid, start, err := parseStat(line)
	if err != nil {
		t.Fatal(err)
	}
	if ppid != 17 || start != "6883643" {
		t.Fatalf("ppid=%d start=%q", ppid, start)
	}
	if _, _, err := parseStat("garbage"); err == nil {
		t.Fatal("expected error for bad stat line")
	}
}

func TestSelf(t *testing.T) {
	env, err := Environ(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if env["PATH"] == "" {
		t.Fatal("PATH missing from own environ")
	}
	ppid, err := ParentPID(os.Getpid())
	if err != nil || ppid != os.Getppid() {
		t.Fatalf("ParentPID = %d, %v; want %d", ppid, err, os.Getppid())
	}
	if !Alive(os.Getpid()) {
		t.Fatal("own process not alive")
	}
	if Alive(0) || Alive(-5) {
		t.Fatal("non-positive pids must not be alive")
	}
	anc := Ancestors(os.Getpid())
	if len(anc) == 0 || anc[0] != os.Getppid() {
		t.Fatalf("Ancestors = %v", anc)
	}
}

func TestHasTTY(t *testing.T) {
	root := t.TempDir()
	old := Root
	Root = root
	t.Cleanup(func() { Root = old })
	mk := func(pid, target string) {
		dir := filepath.Join(root, pid, "fd")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(dir, "0")); err != nil {
			t.Fatal(err)
		}
	}
	mk("10", "/dev/pts/3")
	mk("11", "/dev/null")
	mk("13", "/dev/tty2")
	mk("14", "/dev/ttyS0")
	if !HasTTY(10) {
		t.Fatal("pid 10 should have a tty")
	}
	if !HasTTY(13) {
		t.Fatal("a Linux console counts as a tty")
	}
	if HasTTY(11) || HasTTY(12) || HasTTY(14) {
		t.Fatal("pids 11 and 12 should not have a tty")
	}
}
