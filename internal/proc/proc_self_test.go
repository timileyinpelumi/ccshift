package proc

import (
	"os"
	"runtime"
	"testing"
)

// Runs on every system: what each implementation says about the test process itself.
func TestOwnProcess(t *testing.T) {
	pid := os.Getpid()
	if !Alive(pid) || Alive(0) || Alive(-3) {
		t.Fatal("Alive is wrong about this process or about non-positive pids")
	}
	ppid, err := ParentPID(pid)
	if err != nil || ppid != os.Getppid() {
		t.Fatalf("ParentPID = %d, %v; want %d", ppid, err, os.Getppid())
	}
	if name, err := Comm(pid); err != nil || name == "" {
		t.Fatalf("Comm = %q, %v", name, err)
	}
	if anc := Ancestors(pid); len(anc) == 0 || anc[0] != os.Getppid() {
		t.Fatalf("Ancestors = %v", anc)
	}
	_ = TTY(pid)
	env, err := Environ(pid)
	if runtime.GOOS == "windows" {
		if err == nil {
			t.Fatal("Windows cannot read a process environment; Environ should say so")
		}
		return
	}
	if err != nil || env["PATH"] == "" {
		t.Fatalf("Environ: PATH=%q, %v", env["PATH"], err)
	}
}
