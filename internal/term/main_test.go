package term

import (
	"os"
	"testing"
)

// Tests describe Linux behaviour unless they pick a system with withOS, whatever they run on.
func TestMain(m *testing.M) {
	goos = "linux"
	os.Exit(m.Run())
}
