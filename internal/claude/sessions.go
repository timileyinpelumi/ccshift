package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/timileyinpelumi/ccshift/internal/proc"
)

type Session struct {
	ID        string
	PID       int
	CWD       string
	Kind      string
	Name      string
	Status    string
	StartedAt time.Time
}

type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

func ExecRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

type Reader struct {
	Run         Runner
	SessionsDir string
	ProjectsDir string
	Alive       func(pid int) bool
	HasTTY      func(pid int) bool
	StartTime   func(pid int) (string, error)
}

func NewReader() *Reader {
	base := os.Getenv("CLAUDE_CONFIG_DIR")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".claude")
	}
	return &Reader{
		Run:         ExecRunner,
		SessionsDir: filepath.Join(base, "sessions"),
		ProjectsDir: filepath.Join(base, "projects"),
		Alive:       proc.Alive,
		HasTTY:      proc.HasTTY,
		StartTime:   proc.StartTime,
	}
}

type rawSession struct {
	PID       int    `json:"pid"`
	SessionID string `json:"sessionId"`
	CWD       string `json:"cwd"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	State     string `json:"state"`
	StartedAt int64  `json:"startedAt"`
	ProcStart string `json:"procStart"`
}

func (r rawSession) session() Session {
	status := r.Status
	if status == "" {
		status = r.State
	}
	return Session{
		ID: r.SessionID, PID: r.PID, CWD: r.CWD, Kind: r.Kind, Name: r.Name,
		Status: status, StartedAt: time.UnixMilli(r.StartedAt),
	}
}

func (r *Reader) Live(ctx context.Context) ([]Session, error) {
	raws, err := r.fromAgents(ctx)
	if err != nil {
		if raws, err = r.fromFiles(); err != nil {
			return nil, err
		}
	} else {
		r.fillProcStart(raws)
	}
	var out []Session
	for _, rs := range raws {
		if rs.SessionID == "" {
			continue
		}
		switch rs.Kind {
		case "interactive":
			if !r.Alive(rs.PID) || !r.HasTTY(rs.PID) {
				continue
			}
			// A session file can outlive its process and the pid can be reused.
			if rs.ProcStart != "" {
				st, err := r.StartTime(rs.PID)
				if !errors.Is(err, proc.ErrUnsupported) && (err != nil || st != rs.ProcStart) {
					continue
				}
			}
		case "background":
		default:
			continue
		}
		out = append(out, rs.session())
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].StartedAt.Before(out[j].StartedAt) })
	return out, nil
}

func (r *Reader) fromAgents(ctx context.Context) ([]rawSession, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	b, err := r.Run(ctx, "claude", "agents", "--json")
	if err != nil {
		return nil, err
	}
	var raws []rawSession
	if err := json.Unmarshal(b, &raws); err != nil {
		return nil, err
	}
	return raws, nil
}

func (r *Reader) fromFiles() ([]rawSession, error) {
	paths, err := filepath.Glob(filepath.Join(r.SessionsDir, "*.json"))
	if err != nil {
		return nil, err
	}
	var raws []rawSession
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var rs rawSession
		if json.Unmarshal(b, &rs) == nil {
			raws = append(raws, rs)
		}
	}
	return raws, nil
}

// claude agents --json omits procStart, so take it from the session file for the pid-reuse check.
func (r *Reader) fillProcStart(raws []rawSession) {
	for i, rs := range raws {
		if rs.Kind != "interactive" || rs.ProcStart != "" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(r.SessionsDir, strconv.Itoa(rs.PID)+".json"))
		if err != nil {
			continue
		}
		var f rawSession
		if json.Unmarshal(b, &f) == nil && f.SessionID == rs.SessionID {
			raws[i].ProcStart = f.ProcStart
		}
	}
}

func (r *Reader) TranscriptPath(sessionID string) (string, bool) {
	m, _ := filepath.Glob(filepath.Join(r.ProjectsDir, "*", sessionID+".jsonl"))
	if len(m) == 0 {
		return "", false
	}
	return m[0], true
}
