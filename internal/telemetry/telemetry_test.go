package telemetry

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func client(t *testing.T, url string) *Client {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	return &Client{Dir: t.TempDir(), Endpoint: url, Version: "0.7.0", OS: "linux", Arch: "amd64", Now: func() time.Time { return now }}
}

func TestFlushSendsQueuedEventsOnce(t *testing.T) {
	var got []payload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p payload
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &p)
		got = append(got, p)
		w.WriteHeader(204)
	}))
	defer srv.Close()
	c := client(t, srv.URL)
	c.Record(Event{Command: "restore", N: 3})
	c.Record(Event{Command: "ls"})
	if !c.Due() {
		t.Fatal("a new install should be due")
	}
	if err := c.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || len(got[0].Events) != 2 || got[0].InstallID == "" || got[0].OS != "linux" || got[0].Events[0].N != 3 {
		t.Fatalf("got %+v", got)
	}
	if c.Due() {
		t.Fatal("should not be due right after a flush")
	}
	c.Flush(context.Background())
	if len(got) != 1 {
		t.Fatal("an empty queue should send nothing")
	}
	id := got[0].InstallID
	if c.ID() != id {
		t.Fatal("the install id should stay the same")
	}
}

func TestFailedFlushKeepsEvents(t *testing.T) {
	c := client(t, "http://127.0.0.1:1")
	c.Record(Event{Command: "save"})
	if err := c.Flush(context.Background()); err == nil {
		t.Fatal("expected an error")
	}
	if c.Due() {
		t.Fatal("a failed flush should wait before trying again")
	}
	n := 0
	for _, f := range []string{queueFile, sendingFile} {
		if b, err := os.ReadFile(filepath.Join(c.Dir, f)); err == nil {
			n += len(splitLines(b))
		}
	}
	if n != 1 {
		t.Fatalf("queued events = %d", n)
	}
}
