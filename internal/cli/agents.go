package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/timileyinpelumi/ccshift/internal/claude"
	"github.com/timileyinpelumi/ccshift/internal/proc"
	"github.com/timileyinpelumi/ccshift/internal/settings"
)

// Codex CLI and Gemini CLI keep no list of running sessions that ccshift could read, so their
// sessions are known from their own hooks: each hook records its session against the agent
// process that ran it, and the session counts as running while that process is alive.

const agentsFile = "agents.json"

type agentRecord struct {
	Agent     string `json:"agent"`
	PID       int    `json:"pid"`
	ProcStart string `json:"proc_start,omitempty"`
	CWD       string `json:"cwd"`
	Started   int64  `json:"started"`
}

type agentSetup struct {
	name, label, file string
	events            [][2]string // the agent's event name, the ccshift hook it runs
}

var agentSetups = []agentSetup{
	{"codex", "Codex CLI", "hooks.json", [][2]string{{"SessionStart", "session-start"}, {"Stop", "stop"}, {"SessionEnd", "session-end"}}},
	{"gemini", "Gemini CLI", "settings.json", [][2]string{{"SessionStart", "session-start"}, {"AfterAgent", "stop"}, {"SessionEnd", "session-end"}}},
}

func defaultAgentHomes() map[string]string {
	home, _ := os.UserHomeDir()
	codex := os.Getenv("CODEX_HOME")
	if codex == "" {
		codex = filepath.Join(home, ".codex")
	}
	gemini := filepath.Join(home, ".gemini")
	if h := os.Getenv("GEMINI_CLI_HOME"); h != "" {
		gemini = filepath.Join(h, ".gemini")
	}
	return map[string]string{"codex": codex, "gemini": gemini}
}

var shells = map[string]bool{"sh": true, "bash": true, "zsh": true, "dash": true, "fish": true, "ksh": true, "pwsh": true, "pwsh.exe": true, "powershell.exe": true, "cmd.exe": true}

// agentProcess is the nearest ancestor of this hook that is not a shell: the agent itself.
func (a *App) agentProcess() int {
	for _, pid := range a.AncestorPIDs(a.SelfPID) {
		if names := a.Comms([]int{pid}); len(names) == 1 && !shells[names[0]] {
			return pid
		}
	}
	return 0
}

func (a *App) registerAgent(agent string, in claude.HookInput) error {
	pid := a.agentProcess()
	if in.SessionID == "" || pid == 0 {
		return nil
	}
	start, _ := a.Claude.StartTime(pid)
	unlock, err := a.Store.TryLock(hookBudget / 4)
	if err != nil {
		return err
	}
	defer unlock()
	reg := map[string]agentRecord{}
	a.Store.Load(agentsFile, &reg)
	rec, had := reg[in.SessionID]
	if had && rec.PID == pid {
		return nil
	}
	cwd := in.CWD
	if cwd == "" {
		cwd = rec.CWD
	}
	reg[in.SessionID] = agentRecord{Agent: agent, PID: pid, ProcStart: start, CWD: cwd, Started: a.Now().UnixMilli()}
	for id, r := range reg {
		if !a.Claude.Alive(r.PID) && id != in.SessionID {
			delete(reg, id)
		}
	}
	return a.Store.Put(agentsFile, reg)
}

func (a *App) agentSessions() []claude.Session {
	reg := map[string]agentRecord{}
	a.Store.Load(agentsFile, &reg)
	var out []claude.Session
	for id, r := range reg {
		if !a.Claude.Alive(r.PID) {
			continue
		}
		if st, err := a.Claude.StartTime(r.PID); r.ProcStart != "" && !errors.Is(err, proc.ErrUnsupported) && (err != nil || st != r.ProcStart) {
			continue
		}
		out = append(out, claude.Session{ID: id, PID: r.PID, CWD: r.CWD, Kind: "interactive", Status: r.Agent, Agent: r.Agent, StartedAt: time.UnixMilli(r.Started)})
	}
	return out
}

// liveSessions is every running session: Claude Code's own list, plus Codex and Gemini sessions
// known from their hooks.
func (a *App) liveSessions(ctx context.Context) ([]claude.Session, error) {
	live, err := a.Claude.Live(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, s := range live {
		seen[s.ID] = true
	}
	for _, s := range a.agentSessions() {
		if !seen[s.ID] {
			live = append(live, s)
		}
	}
	return live, nil
}

// setupAgents adds (or, with remove, takes out) ccshift's hooks in the config of each other
// agent that is installed, and says what it did.
func (a *App) setupAgents(remove bool) error {
	for _, ag := range agentSetups {
		dir := a.AgentHomes[ag.name]
		if dir == "" || !a.DirExists(dir) {
			continue
		}
		f, err := settings.Load(filepath.Join(dir, ag.file))
		if err != nil {
			a.ui().Warn("Skipped %s: %v", ag.label, err)
			continue
		}
		before, _ := f.Bytes()
		if _, err := f.RemoveHooks(a.ours); err != nil {
			return err
		}
		if !remove {
			for _, ev := range ag.events {
				if _, err := f.AddHook(ev[0], a.command("hook "+ev[1]+" --agent "+ag.name), 0); err != nil {
					return err
				}
			}
		}
		after, _ := f.Bytes()
		if string(before) == string(after) {
			continue
		}
		if err := f.Save(); err != nil {
			return err
		}
		if remove {
			a.ui().OK("Removed the hooks from %s.", f.Path)
		} else {
			a.ui().OK("Added hooks for %s to %s.", ag.label, f.Path)
		}
	}
	return nil
}
