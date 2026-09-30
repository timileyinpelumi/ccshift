# ccshift

Keep your Claude Code sessions across restarts.

ccshift saves every Claude Code session you have open, in the order of their tabs, and restores them all with one command. It also gives sessions readable names, warns you when one is running out of context, and hands a full session over to a fresh one.

[![ci](https://github.com/timileyinpelumi/ccshift/actions/workflows/ci.yml/badge.svg)](https://github.com/timileyinpelumi/ccshift/actions/workflows/ci.yml)
[![release](https://img.shields.io/github/v/release/timileyinpelumi/ccshift)](https://github.com/timileyinpelumi/ccshift/releases/latest)
[![licence](https://img.shields.io/github/license/timileyinpelumi/ccshift)](LICENSE)

```
$ ccshift ls
#  NAME                   STATUS   CTX   TAB
1  api · PAY-2193         idle     41%   kitty:1.1
2  web · checkout-v2      busy     72%!  kitty:1.2
3  docs · Install guide   waiting  18%   kitty:1.3

# after a restart
$ ccshift restore
work: opening 3 sessions in kitty
```

Website: https://www.timileyin.dev/ccshift

## Contents

- [Install](#install)
- [Quick start](#quick-start)
- [Features](#features)
- [Commands](#commands)
- [Terminals](#terminals)
- [Configuration](#configuration)
- [How it works](#how-it-works)
- [Platform status](#platform-status)
- [Uninstall](#uninstall)
- [Development](#development)

## Install

**macOS and Linux**

```sh
curl -fsSL https://www.timileyin.dev/ccshift/install.sh | sh
```

**Windows** (PowerShell)

```powershell
irm https://www.timileyin.dev/ccshift/install.ps1 | iex
```

**Debian and Ubuntu**

```sh
curl -fsSLO https://github.com/timileyinpelumi/ccshift/releases/latest/download/ccshift_amd64.deb
sudo apt install ./ccshift_amd64.deb
```

**Fedora and RHEL**

```sh
sudo dnf install https://github.com/timileyinpelumi/ccshift/releases/latest/download/ccshift_amd64.rpm
```

**With Go** (1.25 or newer)

```sh
go install github.com/timileyinpelumi/ccshift/cmd/ccshift@latest
```

The install scripts pick the build for your machine (x86_64 or arm64), check it against the published checksum, and install to `~/.local/bin` on macOS and Linux or `%LOCALAPPDATA%\Programs\ccshift` on Windows. Set `CCSHIFT_INSTALL_DIR` to change the location and `CCSHIFT_VERSION` to pin a release. On arm64, use the `arm64` packages.

Requires [Claude Code](https://claude.com/claude-code).

## Quick start

```sh
ccshift init      # add the autosave hooks and statusline to Claude Code
ccshift doctor    # check the setup
```

Restart any Claude Code sessions that were already running so they pick up the hooks. From then on your layout is saved as you work.

After a reboot or a crash:

```sh
ccshift restore
```

## Features

### Restore

`ccshift restore` reopens every saved session in its own tab, in the saved order, each resumed with `claude --resume`. Sessions that are already running are skipped, so running it twice is safe.

```sh
ccshift restore              # every workspace
ccshift restore work         # one workspace
ccshift restore work --pick  # choose an older save
ccshift restore --dry-run    # show what would open
```

### Autosave

After `ccshift init`, the layout is saved when a session starts and after each turn. Autosave never drops a session by itself:

- A session you exit (`/exit`, Ctrl+D, Ctrl+C) leaves the save.
- A session that was killed, or whose terminal was closed, stays in the save and comes back on the next restore.
- `ccshift forget <name>` removes a session for good.
- A saved session that has not run for `stale_days` (default 14) is dropped.

`ccshift save` saves on demand. `ccshift save --edit` lets you reorder or remove sessions in your editor first.

### Names

Claude Code calls a new session something like `api-d7`. ccshift gives it a name built from where it is running:

| Situation | Name |
|---|---|
| Branch contains a ticket id | `api · PAY-2193` |
| Any other branch | `web · checkout-v2` |
| Default branch, once Claude has titled the session | `docs · Install guide` |
| Two sessions with the same name | `api · PAY-2193 #2` |

The name follows the branch. A name you set yourself, with `/rename` in Claude or `ccshift rename`, always wins. On terminals that allow it, the tab title is kept in step.

```sh
ccshift new                     # start Claude in this tab with a generated name
ccshift new login fix           # ...or your own
ccshift rename 3 login fix      # rename session 3 from ccshift ls
ccshift rename . login fix      # rename the session you are in: !ccshift rename . login fix
ccshift rename 3 --reset        # back to the generated name
ccshift rename --edit           # edit every name in your editor
```

Claude itself takes a new name when the session is next resumed. For an immediate change, run `/rename` in that session.

### Context warning

`ccshift ls` shows how much of its context window each session has used. You get a desktop notification when a session passes 70% and again at 85%. Your own statusline keeps working: ccshift runs it and shows its output.

### Handoff

When a session is nearly full, continue it in a fresh one:

```sh
ccshift handoff api              # hand off the session named "api"
ccshift handoff api --edit       # read and edit the brief first
ccshift handoff api --close-old  # close the old tab afterwards
ccshift handoff api --no-launch  # only write the brief
ccshift handoff api --reuse      # start from the brief already written
```

Inside Claude, `!ccshift handoff` hands off the session you are in.

ccshift reads the session's transcript, has a fresh Claude write a brief (goal, constraints and decisions, what is done, current state, next steps, open questions, gotchas), and opens a new session named `api (2)` that reads the brief and carries on, using the same model as the old session. The brief writer can read the repository but cannot change anything. The new session takes the old one's place in the saved layout.

### Workspaces

Group sessions by directory. Each workspace is saved separately and restored into its own window.

```toml
[workspaces]
work = ["~/dev/work"]
personal = ["~/dev/personal"]
```

Sessions outside these paths go into `default`. `ccshift ws` lists workspaces.

## Commands

| Command | What it does |
|---|---|
| `ccshift ls [--all] [--json]` | List running sessions in tab order, with status and context used |
| `ccshift save [workspace] [--edit] [--force]` | Save the layout now |
| `ccshift restore [workspace] [--pick] [--dry-run]` | Reopen saved sessions |
| `ccshift new [name] [-- claude args]` | Start a named session in this tab |
| `ccshift rename <target> <name>` | Name a session. Also `--reset` and `--edit` |
| `ccshift focus <target>` | Switch to a session's tab |
| `ccshift handoff [target]` | Continue a session in a fresh one |
| `ccshift forget <name>` | Remove a session from the save |
| `ccshift ws` | List workspaces |
| `ccshift init [--remove]` | Add or remove the hooks and statusline |
| `ccshift doctor` | Check the setup |
| `ccshift version` | Print the version |

A `<target>` is a number from `ccshift ls`, a name, the start of a name, the start of a session id, or `.` for the session the command is run from. Most commands take `--terminal <name>` to override terminal detection.

## Terminals

| Terminal | System | Restore opens | Tab order | Titles and focus |
|---|---|---|---|---|
| tmux, kitty, WezTerm, zellij 0.44+ | Linux, macOS | tabs | read from the terminal | yes |
| Konsole | Linux | tabs | read from the terminal | yes |
| iTerm2 | macOS | tabs | read from the terminal | yes |
| Terminal | macOS | windows | read from the terminal | yes |
| WezTerm | Windows | tabs | read from the terminal | yes |
| Windows Terminal | Windows | tabs | kept from the last restore | no |
| GNOME Terminal, Ptyxis, Tilix, Xfce Terminal | Linux | tabs | kept from the last restore | no |
| Alacritty, foot, Ghostty | Linux, macOS | windows | kept from the last restore | no |

Some terminals cannot report their tab order to another program. For those, ccshift keeps the order it last restored, and `ccshift save --edit` sets it by hand.

Setup notes:

- **kitty** needs remote control. Add to `kitty.conf`:
  ```
  allow_remote_control socket-only
  listen_on unix:/tmp/kitty-{kitty_pid}
  ```
- **Konsole** needs `qdbus` for tab order.
- **zellij** needs 0.44 or newer.
- **WSL** runs the Linux version.

`ccshift doctor` reports anything your terminal is missing.

## Configuration

Optional. `~/.config/ccshift/config.toml` on macOS and Linux, `%AppData%\ccshift\config.toml` on Windows.

```toml
warn_thresholds = [70, 85]        # context percentages that notify
autosave_debounce_seconds = 30    # at most one autosave per this many seconds
stale_days = 14                   # drop a saved session that has not run for this long
history_keep = 20                 # manual saves kept per workspace
sync_tab_titles = true            # keep tab titles in step with session names
brief_model = "sonnet"            # model that writes handoff briefs
ticket_pattern = "\\b([A-Z][A-Z0-9]{1,5})[-_](\\d{2,})\\b"   # prefix and number of a ticket id
ticket_ignore = ["fix", "node", "react"]                     # prefixes that are not tickets

[workspaces]
work = ["~/dev/work"]
```

## How it works

ccshift is a single binary with no daemon. `ccshift init` adds three hooks to Claude Code's `settings.json` (`SessionStart`, `Stop`, `SessionEnd`) and wraps your statusline command. The hooks call `ccshift hook …`, which saves the layout and records which tab each session is in. The statusline wrapper records context usage and then runs your own statusline.

- Hooks always exit 0, run under a two second budget and log failures to a file. They cannot break a Claude session.
- State lives in `~/.local/state/ccshift` (`%LocalAppData%\ccshift\state` on Windows): saved layouts and their history, session names, context usage, handoff briefs and a log.
- `init` backs up `settings.json` to `settings.json.ccshift-bak`, keeps its key order, and leaves your other hooks alone.

The design is written up in [docs/design](docs/design/2026-09-30-ccshift-design.md).

## Platform status

| System | Status |
|---|---|
| Linux | Stable. Every listed terminal except Ghostty has been run for real. |
| macOS | Beta. Automated tests pass, including a real tmux restore. iTerm2 and Terminal have not been run on a Mac yet. |
| Windows | Beta. Automated tests pass. Windows Terminal has not been run on a PC yet. |

If you use ccshift on macOS or Windows, an [issue](https://github.com/timileyinpelumi/ccshift/issues) saying what worked and what did not is very useful.

## Uninstall

```sh
ccshift init --remove    # take the hooks and statusline out of settings.json
```

Then delete the binary, and optionally the state and config directories listed above.

## Development

```sh
go test ./...                              # unit tests
go build -o ccshift ./cmd/ccshift
scripts/e2e.sh ./ccshift                   # end to end against a real tmux
```

CI runs the tests on Linux, macOS and Windows. Pushing a `v*` tag builds and publishes a release.

## Licence

[MIT](LICENSE). An independent project, not affiliated with Anthropic.
