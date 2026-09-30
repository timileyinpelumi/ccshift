# ccshift design

Date: 2026-09-30
Status: draft, awaiting review

## What it is

ccshift is a CLI for people who keep several Claude Code sessions open in terminal tabs on Linux. It does three things:

1. Saves the open Claude sessions in the order their tabs are in, and restores them with one command after a reboot or crash.
2. Gives sessions distinct names and keeps those names in sync with Claude and the tab title.
3. Hands a session whose context has degraded over to a fresh session, through a written brief, in one step.

Around those: a session list with status and context usage, a context-usage warning, focusing a session's tab, and named workspaces.

## Why it's needed

Checked on 2026-09-30:

- No Linux tool restores native terminal tabs with each Claude session resumed in place. cmux does this on macOS only. The Linux tools (claude-squad, agent-deck, ccmanager, tmux-claude-hatch) all require working inside their own tmux setup.
- cctabs manages sessions as terminal tabs but needs Tabby on macOS.
- Claude Code's background sessions and `claude agents` don't restore a layout after a reboot and don't show context usage.
- Handoff tools (claude-handoff and various skills) write a brief but don't tie it to a context warning or start the new session.

## Scope

In v1:

- `save` and `restore` in tab order, per workspace
- autosave through Claude Code hooks
- naming: generated names, `new`, `rename`, `rename --edit`, tab title sync
- `handoff`: extract, brief, launch, bookkeeping
- `ls`, `focus`, workspaces
- context warning
- `init`, `init --remove`, `doctor`
- terminals: tmux, kitty, WezTerm, zellij, Konsole, gnome-terminal, Ptyxis, xfce4-terminal, Tilix, Ghostty, Alacritty, foot, and a generic fallback
- one-time import of `~/.claude/saved-sessions` from the old scripts

Not in v1:

- `find` (search past transcripts and resume the match). Planned for v1.1.
- Codex CLI, Gemini CLI, opencode. The `claude` package is kept separate so another agent can be added next to it.
- macOS and Windows.
- A TUI. Everything is plain CLI output, with `--json` where it makes sense.
- Restoring background (`--bg`) sessions. `restore` lists them and points to `claude respawn`.

## Invariants

- A hook never breaks or noticeably slows a Claude session. Hook commands always exit 0, have a time limit, and log failures to a file.
- A shutdown, crash or closed terminal never shrinks the `latest` snapshot. Only an explicit user exit or `ccshift forget` removes a session from it.
- `restore` never opens a second tab for a session that is already running.
- ccshift only writes to Claude Code's files through Claude Code itself (flags such as `--name`), with one exception: `init` merges hook and statusline entries into `~/.claude/settings.json`, after a backup.
- Reading Claude Code's formats (session files, `claude agents --json`, transcripts, statusline input) happens only in the `claude` package.

## Architecture

One Go binary, no daemon.

```
cmd/ccshift       CLI entry, subcommand wiring
internal/claude   live sessions, transcripts, statusline input; the only code that knows Claude's formats
internal/term     adapter interface and one file per terminal
internal/store    ccshift state on disk
internal/names    name generation and resolution of targets
internal/handoff  extract, brief, launch
internal/hooks    what each `ccshift hook <event>` does
internal/config   config file and defaults
```

Go was chosen over Bun/TypeScript because hooks call the binary after every turn in every session, so start-up time and a single-file install matter.

### Session source

`claude agents --json` is the primary source. On this machine (Claude Code 2.1.285) it returns interactive and background sessions with `pid`, `cwd`, `kind`, `startedAt`, `sessionId`, `name`, `status` / `state`. If the command fails or its output doesn't parse, ccshift reads `~/.claude/sessions/*.json` directly, which has the same fields for live sessions.

### Terminal adapters

```go
type Adapter interface {
    Name() string
    Detect(env map[string]string) bool      // is this process inside this terminal
    Tier() Tier                              // Exact, LaunchOnly, WindowsOnly
    List() ([]Tab, error)                    // tabs in on-screen order; Exact tier only
    TabOf(env map[string]string) (TabID, bool) // which tab a process belongs to, from its env
    Open(launches []Launch) error            // open tabs/windows in the given order
    SetTitle(tab TabID, title string) error
    Focus(tab TabID) error
}
```

Matching a Claude session to its tab: read `/proc/<claude pid>/environ` and pass it to `TabOf`. Each exact-tier terminal sets a variable in every shell that the Claude process inherits.

| Terminal | Tier | Tab id from env | List order | Open | Title | Focus |
|---|---|---|---|---|---|---|
| tmux | Exact | `TMUX_PANE` | `list-panes -a -F ...` | `new-window -c DIR -n NAME` | `rename-window` | `select-window` |
| kitty | Exact | `KITTY_WINDOW_ID` | `kitten @ ls` | `kitten @ launch --type=tab --cwd --tab-title` | `set-tab-title --match id:N` | `focus-tab --match id:N` |
| WezTerm | Exact | `WEZTERM_PANE` | `wezterm cli list --format json` | `wezterm cli spawn --cwd` | `wezterm cli set-tab-title` | `wezterm cli activate-tab` |
| zellij ≥0.44 | Exact | `ZELLIJ_PANE_ID` | `zellij action list-tabs --json`, `list-panes --json` | `zellij action new-tab --cwd --name` | `rename-tab --tab-id` | `go-to-tab-by-id` |
| Konsole | Exact | `KONSOLE_DBUS_SESSION` | D-Bus `sessionList` | `--tabs-from-file` | D-Bus `setTabTitleFormat` | D-Bus `setCurrentSession` |
| gnome-terminal | LaunchOnly | none | none | one command: `--window … --command … --tab … --command …` | via Claude session name | not supported |
| Ptyxis | LaunchOnly | none | none | `ptyxis --tab -d DIR -x CMD` | via Claude session name | not supported |
| xfce4-terminal | LaunchOnly | none | none | `xfce4-terminal --tab --working-directory -x cmd` | via Claude session name | not supported |
| Tilix | LaunchOnly | none | none | `tilix -a app-new-session -w DIR -e CMD` | via Claude session name | not supported |
| Ghostty | WindowsOnly | none | none | `ghostty +new-window` / `-e` | via Claude session name | not supported |
| Alacritty, foot | WindowsOnly | none | none | `alacritty msg create-window` / `footclient` | via Claude session name | not supported |
| generic | WindowsOnly | none | none | `$TERMINAL -e` or `x-terminal-emulator -e`, else print commands | none | not supported |

LaunchOnly and WindowsOnly entries are a table of launch templates, not separate code paths.

Setup requirements the adapters check and `doctor` reports:

- kitty: `allow_remote_control yes` (or `socket-only`) and `listen_on unix:...` in `kitty.conf`.
- zellij: version 0.44 or later.
- tmux: none. `rename-window` turns off `automatic-rename` for that window, which is what we want.

Tab titles: Claude Code writes the terminal title itself from the session name, using escape codes. On gnome-terminal, Ptyxis, xfce4-terminal, Tilix, Ghostty, Alacritty and foot, escape codes are the only way to set a title, so Claude wins, and the title is fixed by giving the Claude session a good name. On the exact-tier terminals, ccshift sets the tab title through the terminal's API, which is separate from the escape-code title and is not overwritten.

### Order on LaunchOnly and WindowsOnly terminals

Tab order can't be read back. ccshift keeps its own order: the order it last opened sessions in on restore, followed by any sessions it didn't open, sorted by start time. `save --edit` opens the order in `$EDITOR` to fix it by hand. After one restore through ccshift, the order stays stable across saves.

## Store

```
$XDG_STATE_HOME/ccshift/            (default ~/.local/state/ccshift)
  names.json                        session id → {name, source: user|generated, set_at}
  workspaces/<ws>/latest.json       current snapshot
  workspaces/<ws>/history/<ts>.json last 20 snapshots
  order.json                        last opened order per workspace, for LaunchOnly terminals
  context.json                      session id → {used_percentage, window_size, updated_at, warned_at_thresholds}
  handoffs/<session-id>.md          briefs
  lineage.json                      old session id → new session id
  ccshift.log
```

A snapshot:

```json
{
  "workspace": "work",
  "saved_at": "2026-09-30T10:12:00Z",
  "terminal": "kitty",
  "sessions": [
    {"session_id": "…", "cwd": "…", "name": "acme · PAY-2193", "position": 1, "exit": null}
  ]
}
```

Writes go to a temp file and are renamed into place so a crash mid-write never leaves a broken file. Concurrent hook calls take a file lock on the store.

## Commands

```
ccshift init [--remove]
ccshift doctor
ccshift new [name] [-- claude args]
ccshift ls [--all] [--json]
ccshift save [workspace] [--edit]
ccshift restore [workspace] [--pick] [--dry-run]
ccshift forget <target>
ccshift rename <target> <name>
ccshift rename --edit
ccshift focus <target>
ccshift handoff [target] [--edit] [--close-old] [--no-launch] [--model m]
ccshift ws
ccshift hook <event>              (called by Claude Code, not by people)
ccshift statusline                (called by Claude Code, not by people)
```

A `<target>` is resolved in this order: `.` (the Claude session that is the parent of this process, for use as `!ccshift ...` inside Claude), a tab position number from `ls`, an exact name, a unique name prefix, a session id prefix.

## Naming

A session's name is resolved fresh every time ccshift looks at it. First match wins:

1. A name set with `ccshift rename`, until Claude has it (after a resume) or the session is renamed inside Claude afterwards.
2. The name Claude has, when the user chose it (`-n`, `/rename`).
3. A generated name.

Generated name, first match wins:

1. `<repo> · <TICKET>` when the branch contains a ticket id. The pattern (`ticket_pattern`) is matched in any letter case and has two groups, prefix and number; the default is `\b([A-Z][A-Z0-9]{1,5})[-_](\d{2,})\b`. Prefixes in `ticket_ignore` (`fix`, `node`, `react`, `sha`, `hotfix` and similar) and version-like prefixes (`v2`) are skipped, so `node-18-upgrade`, `fix-123` and `v2-10` stay branch names.
2. `<repo> · <branch>` when the branch is not the default branch (`origin/HEAD`, else `main` or `master`).
3. `<repo> · <short title>` once Claude has written an `ai-title` record for the session.
4. `<repo>` until then.

`<repo>` is the main repository's directory name (so a linked worktree gets the repository's name), or the working directory's name outside git. Sessions with the same generated name get ` #2`, ` #3`. A session keeps its number when others close, and a new one takes the lowest free number.

When git can't answer in time, the directory is gone, or HEAD is detached (rebase, bisect), the last generated name is kept. Claude's title for a session is cached, because a long transcript's tail often has no title record.

Because the name is computed each time, a generated name follows the branch: switch branches and the name changes.

Where names are applied:

- `ccshift ls`, saved snapshots, target resolution and tab titles all use the resolved name.
- On tmux, kitty, WezTerm and zellij the tab title is set through the terminal whenever the name changes (`sync_tab_titles = false` turns this off).
- `restore` starts sessions with `--resume <id> --name=<name>`. Checked on Claude Code 2.1.285: this renames the session. When the name was a generated one, ccshift records that it handed it to Claude, so the session keeps a generated name afterwards instead of freezing on it.
- `ccshift new [name] [-- claude args]` starts Claude in the current tab with `--session-id` and `--name`, and sets the tab title. Without a name it uses the generated one.
- `ccshift rename <target> <name>` stores the name, updates the saved snapshots and the tab title, and it reaches Claude on the next resume. Nothing lets an outside program rename a running Claude session, so the command says to run `/rename` in the session for an immediate change.
- `ccshift rename --edit` opens every session's name in `$EDITOR` for bulk changes. A line left with only the id puts that session back on its generated name, as does `ccshift rename <target> --reset`.
- Name entries live in `names.json`. Every writer re-reads the file under the store lock and applies only its own per-session changes, so a slow hook cannot undo a rename.

Known limits: after `ccshift rename Y` over a Claude name X, running `/rename X` inside Claude is not noticed, because Claude's name is unchanged. On tmux the window name stays after Claude exits. A tab holding two sessions in split panes is not titled.

An earlier draft had `rename --auto`, which suggested names for sessions still on Claude's default names. Generated names now apply on their own, so there is nothing left for it to suggest.

## Handoff

`ccshift handoff [target] [--edit] [--close-old] [--no-launch] [--reuse] [--model m]`. With no target it hands off the session it is run from (`!ccshift handoff` inside Claude). The target can also be a session that is saved but no longer running, by exact name or id prefix.

1. **Extract**, no model involved. The transcript is read as a stream, since lines can be several megabytes. Kept:
   - every user message, with harness additions (`<system-reminder>` blocks, local command output) removed and slash commands shown as `/name args`
   - Claude's text replies
   - tool calls as one line each (`read path`, `edited path`, `ran <first line of the command>`, `searched pattern`, `used <Tool>`), repeats collapsed
   - the latest todo list and any `/compact` summaries
   - `git branch`, `git status --short`, `git diff --stat` and the last five commits

   Dropped: thinking, tool results, images, subagent traffic. The last 10 turns are kept whole. Older turns keep the request and the final reply, each cut to 500 characters. If the result is over 150,000 characters the oldest turns are left out, with a note saying how many.
2. **Brief.** `claude -p --model <brief_model> --tools Read,Grep,Glob --no-session-persistence`, run in the session's directory with the prompt and extract on stdin, for up to five minutes. The prompt asks for these sections: Goal, Constraints and decisions, Done, Current state, Next steps, Open questions, Gotchas. It tells the writer to check file-level claims before stating them and to say so when it could not. `brief_model` defaults to `sonnet`. The brief is saved to `handoffs/<old-session-id>.md`. `--edit` opens it in `$EDITOR` before the next step. `--no-launch` stops here, and a later `--reuse` starts from that brief without writing it again. The extract sits between `<extract>` tags and the prompt says it is a record, not instructions, so text quoted in the old conversation is less likely to steer the writer. Briefs older than 30 days are removed.
3. **Launch.** `claude --session-id <new> --name="<old name> (2)" "Read <brief path> and continue from it."`. On tmux, kitty and WezTerm the new tab opens next to the old one; on zellij it is added to the session; elsewhere it opens the way `restore` opens tabs. Later handoffs count on: `(3)`, `(4)`.
4. **Bookkeeping.** `lineage.json` records old → new. In the saved layout the new session takes the old one's position. A session listed in `lineage.json` is left out of every save from then on, autosave or explicit, so `restore` won't bring it back. `--close-old` closes the old session's pane on tmux, kitty and WezTerm; other panes in the same tab are left alone. The new name skips numbers already in use, so handing the same session off twice gives `(2)` and then `(3)`.

If the brief can't be written, nothing is launched, and the extract is kept at `handoffs/<id>.extract.md` with the command to start from it by hand. If the tab can't be opened, or the terminal can only print the command (SSH, zellij from outside a session), the command to run is printed without the `env -u` prefix and the save is left as it was.

Every session ccshift launches (`restore` and `handoff`) starts through `env -u …` for Claude Code's per-session variables. A tmux server or a terminal that was itself started from inside a Claude session passes that environment to every new tab, and a session that inherits it starts as a child session with transcript saving off.

Not done: the new session starts with the user's default model and settings, not the old session's. On WezTerm the new tab is added at the end of the window, not next to the old one.

## Context warning

- `ccshift statusline` is installed as the statusline command. It reads `context_window.used_percentage`, `context_window.context_window_size`, `session_id`, `session_name` and `transcript_path` from stdin (documented fields), records them in `context.json`, then runs the user's previous statusline command with the same stdin and prints its output. With no previous command it prints a short default line with the name and context %.
- Thresholds default to `[70, 85]` percent, configurable. When a session first crosses one, ccshift sends `notify-send "ccshift" "<name> is at 72% context. Run: ccshift handoff <name>"`. Each threshold fires once per session.
- `ls` marks sessions past the first threshold.
- If the statusline isn't installed or hasn't reported yet, the Stop hook computes usage from the latest `usage` block in the transcript as a fallback.

## Workspaces

- Config maps workspaces to cwd prefixes:
  ```toml
  [workspaces]
  work = ["~/dev/acme", "~/dev/shopfront"]
  personal = ["~/dev/personal"]
  ```
  Longest matching prefix wins. Anything unmatched goes to `default`.
- `save` and `restore` with no argument act on every workspace. Each workspace opens in its own window (exact tier: its own tmux session, kitty OS window, WezTerm window, zellij session).
- `ws` lists workspaces with session count and last save time.

## Autosave

- Runs from the SessionStart and Stop hooks. Stop is debounced to one save per 30 seconds. Nothing is written when the layout hasn't changed.
- Autosave never drops a saved session by itself. Running sessions are written in tab order, and saved sessions that are no longer running stay at their old position, marked `exit: unclean`.
- The SessionEnd hook removes a session when its `reason` is `prompt_input_exit`, `clear` or `resume`. Checked on Claude Code 2.1.285: `/exit`, Ctrl+D and Ctrl+C report `prompt_input_exit`; SIGTERM and SIGHUP (terminal closed, shutdown) report `other`. Any other reason keeps the session and marks it unclean.
- A session also leaves the save through `forget`, an explicit `save`, or after `stale_days` (default 14) without running.
- `restore` skips sessions that are already running, and sessions whose transcript no longer exists (with a message).

An earlier draft had a second guard ("a save that removes more than half the sessions is not promoted to latest"). It is not needed: since autosave cannot remove anything, a shutdown leaves `latest` as it was.

## init, doctor, config

`init`:

- backs up `~/.claude/settings.json` to `settings.json.ccshift-bak`
- adds `ccshift hook session-start`, `ccshift hook stop`, `ccshift hook session-end` entries, alongside existing hooks
- replaces `statusLine.command` with `ccshift statusline`, storing the old statusline in ccshift's state directory (`init.json`)
- detects the terminal and prints any config it needs
- imports `~/.claude/saved-sessions/*.json` into the `default` workspace history

`init --remove` puts back the old statusline command and removes only ccshift's hook entries.

`doctor` checks: binary on PATH, hooks present, statusline wired, terminal detected and its requirements met, `claude agents --json` parses, store writable. Each failed check prints what to do.

Config at `$XDG_CONFIG_HOME/ccshift/config.toml`, all optional:

```toml
warn_thresholds = [70, 85]
brief_model = "sonnet"
ticket_pattern = "\\b([A-Z][A-Z0-9]{1,5})[-_](\\d{2,})\\b"
sync_tab_titles = true
autosave_debounce_seconds = 30
stale_days = 14
history_keep = 20

[workspaces]
```

## Errors

- Hooks: any error is logged and the hook exits 0. Each hook has a 2 second budget; work that doesn't fit (such as reading a large transcript) is skipped for that call.
- A missing or broken adapter falls back to the generic one, and says so.
- `claude agents --json` failing falls back to the session files.
- Transcript lines that don't parse are skipped, never fatal. Unknown record types are ignored.
- `restore` prints each session it opens, skips, or fails to open, and exits non-zero if any failed.

## Testing

- Adapters: unit tests against recorded outputs from each terminal's CLI (fixtures checked in). tmux also gets integration tests against a real headless tmux server in CI.
- Claude parsing: fixtures from real transcripts, session files and `claude agents --json` output across Claude Code versions, with contents scrubbed.
- Handoff extract: golden-file tests.
- Autosave rules: tests for a simulated shutdown (all sessions end unclean), a mass exit over the 50% guard, and a normal user exit.
- Names: table tests for the generation rules and target resolution.
- `init`/`--remove`: round-trip test on a sample `settings.json` that already has hooks and a statusline.

## Distribution

- GitHub releases built with goreleaser (linux amd64 and arm64), and `go install github.com/timileyinpelumi/ccshift/cmd/ccshift@latest`.
- Later option: a Claude Code plugin that ships the hooks and a `/handoff` command, which would replace the settings.json edits `init` makes.

## Open items for implementation

1. Konsole: confirm that D-Bus `sessionList` returns tabs in on-screen order. If not, Konsole moves to LaunchOnly.
2. Confirm the SessionEnd `reason` values for `/exit`, Ctrl+D, a closed terminal and a killed process.
3. Confirm WezTerm's `cli list` JSON has what `TabOf` and ordering need (pane id, tab id, window id order).
4. Confirm zellij's `list-tabs --json` `position` field and `new-tab` flags on 0.44.
5. Confirm that the `claude agents --json` `status` values cover busy, idle and waiting for input.
