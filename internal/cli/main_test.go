package cli

import (
	"os"
	"testing"
)

// Tests describe Linux behaviour unless they set goos themselves, whatever they run on.
func TestMain(m *testing.M) {
	goos = "linux"
	os.Exit(m.Run())
}
