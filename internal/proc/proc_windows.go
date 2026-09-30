package proc

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

type entry struct {
	parent int
	name   string
}

func snapshot() (map[int]entry, error) {
	h, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(h)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	out := map[int]entry{}
	for err = windows.Process32First(h, &pe); err == nil; err = windows.Process32Next(h, &pe) {
		out[int(pe.ProcessID)] = entry{int(pe.ParentProcessID), windows.UTF16ToString(pe.ExeFile[:])}
	}
	return out, nil
}

// Windows does not let one process read another's environment without reading its memory.
// ccshift relies on what its hooks record instead.
func Environ(int) (map[string]string, error) { return nil, ErrUnsupported }

func ParentPID(pid int) (int, error) {
	procs, err := snapshot()
	if err != nil {
		return 0, err
	}
	e, ok := procs[pid]
	if !ok {
		return 0, ErrUnsupported
	}
	return e.parent, nil
}

func StartTime(int) (string, error) { return "", ErrUnsupported }

func Comm(pid int) (string, error) {
	procs, err := snapshot()
	if err != nil {
		return "", err
	}
	return procs[pid].name, nil
}

func Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	if windows.GetExitCodeProcess(h, &code) != nil {
		return false
	}
	return code == 259 // STILL_ACTIVE
}

func TTY(int) string { return "" }

// There is no tty to check on Windows; the session kind Claude reports is trusted.
func HasTTY(int) bool { return true }
