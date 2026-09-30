package ui

import (
	"os"

	"golang.org/x/sys/windows"
)

// enableVT turns on escape code handling in the Windows console. Consoles too old for it get plain output.
func enableVT(f *os.File) bool {
	h := windows.Handle(f.Fd())
	var mode uint32
	if windows.GetConsoleMode(h, &mode) != nil {
		return false
	}
	return windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil
}
