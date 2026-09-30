package term

import (
	"runtime"
	"testing"
	"time"
)

func TestSpawnReportsEarlyFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh and sleep")
	}
	if err := spawn("sh", "-c", "echo boom >&2; exit 3"); err == nil {
		t.Fatal("a terminal that exits with an error right away should be reported")
	}
	start := time.Now()
	if err := spawn("sleep", "2"); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("spawn waited for a long-running terminal")
	}
	if err := spawn("true"); err != nil {
		t.Fatalf("a launcher that exits 0 is fine: %v", err)
	}
}
