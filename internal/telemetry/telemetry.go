// Package telemetry keeps anonymous usage events in a local queue and sends them at most once a
// day: the command, whether it worked, an error category, counts and the platform. Never paths,
// session names or content.
package telemetry

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	Endpoint    = "https://www.timileyin.dev/api/ccshift/events"
	queueFile   = "telemetry.jsonl"
	sendingFile = "telemetry.sending.jsonl"
	stampFile   = "telemetry.last"
	idFile      = "install-id"
	lockFile    = "telemetry.lock"
	maxQueue    = 256 << 10
	every       = 24 * time.Hour
	retry       = time.Hour
)

type Event struct {
	At       string `json:"at"`
	Command  string `json:"command"`
	OK       *bool  `json:"ok,omitempty"`
	Error    string `json:"error,omitempty"`
	Terminal string `json:"terminal,omitempty"`
	Agent    string `json:"agent,omitempty"`
	Method   string `json:"method,omitempty"`
	N        int    `json:"n,omitempty"`
	MS       int    `json:"ms,omitempty"`
}

type payload struct {
	InstallID string  `json:"install_id"`
	Version   string  `json:"version"`
	OS        string  `json:"os"`
	Arch      string  `json:"arch"`
	Events    []Event `json:"events"`
}

type Client struct {
	Dir, Endpoint, Version, OS, Arch string
	Now                              func() time.Time
}

// Record appends an event to the queue. It never fails loudly: usage data is not worth an error.
func (c *Client) Record(e Event) {
	if e.At == "" {
		e.At = c.Now().UTC().Format(time.RFC3339)
	}
	path := filepath.Join(c.Dir, queueFile)
	if fi, err := os.Stat(path); err == nil && fi.Size() > maxQueue {
		return
	}
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	os.MkdirAll(c.Dir, 0o700)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	f.Write(append(b, '\n'))
	f.Close()
}

// ID is a random id for this install, created on first use.
func (c *Client) ID() string {
	path := filepath.Join(c.Dir, idFile)
	if b, err := os.ReadFile(path); err == nil && len(bytes.TrimSpace(b)) > 0 {
		return string(bytes.TrimSpace(b))
	}
	buf := make([]byte, 16)
	rand.Read(buf)
	id := hex.EncodeToString(buf)
	os.MkdirAll(c.Dir, 0o700)
	os.WriteFile(path, []byte(id+"\n"), 0o600)
	return id
}

// Due reports whether it is time to send: a day after the last send, an hour after a failed one.
func (c *Client) Due() bool {
	b, err := os.ReadFile(filepath.Join(c.Dir, stampFile))
	if err != nil {
		return true
	}
	f := strings.Fields(string(b))
	if len(f) != 2 {
		return true
	}
	at, err := strconv.ParseInt(f[0], 10, 64)
	if err != nil {
		return true
	}
	wait := every
	if f[1] != "ok" {
		wait = retry
	}
	return c.Now().Sub(time.Unix(at, 0)) >= wait
}

func (c *Client) stamp(result string) {
	os.WriteFile(filepath.Join(c.Dir, stampFile), []byte(fmt.Sprintf("%d %s\n", c.Now().Unix(), result)), 0o600)
}

// Flush sends everything queued. New events recorded while it runs go to a fresh queue.
func (c *Client) Flush(ctx context.Context) error {
	// One sender at a time, or two would post the same events. A lock older than a minute is left over from a crash.
	lock := filepath.Join(c.Dir, lockFile)
	if fi, err := os.Stat(lock); err == nil && time.Since(fi.ModTime()) > time.Minute {
		os.Remove(lock)
	}
	f, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil
	}
	f.Close()
	defer os.Remove(lock)
	queue, sending := filepath.Join(c.Dir, queueFile), filepath.Join(c.Dir, sendingFile)
	if _, err := os.Stat(sending); errors.Is(err, fs.ErrNotExist) {
		if err := os.Rename(queue, sending); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	b, err := os.ReadFile(sending)
	if errors.Is(err, fs.ErrNotExist) {
		c.stamp("ok")
		return nil
	}
	if err != nil {
		return err
	}
	p := payload{InstallID: c.ID(), Version: c.Version, OS: c.OS, Arch: c.Arch}
	for _, line := range splitLines(b) {
		var e Event
		if json.Unmarshal(line, &e) == nil {
			p.Events = append(p.Events, e)
		}
	}
	if len(p.Events) == 0 {
		os.Remove(sending)
		c.stamp("ok")
		return nil
	}
	body, _ := json.Marshal(p)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err == nil {
		resp.Body.Close()
		if resp.StatusCode >= 300 {
			err = fmt.Errorf("status %d", resp.StatusCode)
		}
	}
	if err != nil {
		c.stamp("failed")
		return err
	}
	os.Remove(sending)
	c.stamp("ok")
	return nil
}

func splitLines(b []byte) [][]byte {
	var out [][]byte
	for _, l := range bytes.Split(b, []byte("\n")) {
		if len(bytes.TrimSpace(l)) > 0 {
			out = append(out, l)
		}
	}
	return out
}
