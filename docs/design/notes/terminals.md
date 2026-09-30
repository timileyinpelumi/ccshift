# Terminal checks

What was run against each real terminal, on 2026-09-30, Ubuntu 24.04, Claude Code 2.1.285. Each check restored two throwaway sessions (one named `-rt beta's`, to cover a leading dash and a quote) through the `ccshift` binary with a scratch state directory, then ran `ccshift ls` and `ccshift focus`.

| Terminal | Version | Restore | ls tab order | Titles | Focus |
|---|---|---|---|---|---|
| tmux | 3.4 | ok | ok | ok | ok |
| zellij | 0.45.1 | ok, inside a session and from outside | ok | ok | ok |
| kitty | latest from kovidgoyal.net | ok, new OS window with tabs | ok | ok | ok |
| WezTerm | latest AppImage | ok, new window with tabs | ok | ok | ok |
| Konsole | 23.08 | ok, one window with tabs | ok, follows a moved tab | ok | ok |
| GNOME Terminal | 3.52 | ok, one window with tabs | not readable, kept from the last restore | from the Claude session name | not supported |
| Ptyxis | 49.1 | ok, one window with tabs | not readable | from the Claude session name | not supported |
| Tilix | Ubuntu 24.04 | ok, one window with two sessions | not readable | from the Claude session name | not supported |
| Xfce Terminal | Ubuntu 24.04 | ok, one window with tabs | not readable | set at launch | not supported |
| Alacritty | Ubuntu 24.04 | ok, one window per session | none | set at launch | not supported |
| foot | Ubuntu 25.10 | ok, one window per session | none | set at launch | not supported |
| Ghostty | | not run: not packaged for Ubuntu | | | |

tmux, zellij, kitty, WezTerm and GNOME Terminal were first run on the desktop. Konsole, GNOME Terminal, Tilix, Xfce Terminal, Alacritty and WezTerm focus were run in an Ubuntu 24.04 container under Xvfb and openbox; Ptyxis under Xvfb in Ubuntu 25.10; foot under headless sway. Those runs used a stand-in `claude` that sleeps, two saved sessions (one with a space in its directory, names with `·` and a quote), and checked the process count, each process's working directory, and the number of top-level windows.

What the container runs changed:

- Konsole and Xfce Terminal opened the second session in a second window. Konsole now gets its tabs from a file (`--tabs-from-file <file> -e true`), and Xfce Terminal no longer starts its one command with `--window`.
- GNOME Terminal opened two windows when tabs were added one at a time. It now opens the window and its tabs in one command, with the tab-by-tab form as the fallback because `--command` is deprecated.
- Konsole's `sessionList` follows tab order after `moveSessionLeft`, and sessions carry `KONSOLE_DBUS_SERVICE` and `KONSOLE_DBUS_SESSION`, so Konsole is an exact-order terminal. It needs `qdbus`. Its D-Bus interface has no call to close a tab or to start a tab running a command, so a handoff tab starts a shell and has it `exec` the command.
- Tilix refuses a command containing non-ASCII text under a non-UTF-8 locale. With a UTF-8 locale it works.
- WezTerm: `activate-pane` switches to the pane's tab, confirmed with `list-clients` in a focused window.

## tmux

Also covered by `TestTmuxIntegration` on a private server. A restore into a workspace whose tmux session already exists adds windows to it.

## zellij

Needs 0.44 or later for `list-panes --json`. Checked on 0.45.1:

- `zellij action list-panes --json` has `id`, `is_plugin`, `tab_id`, `tab_position`. The fixture is trimmed real output.
- `ZELLIJ_PANE_ID` in a pane's process matches `id` for non-plugin panes.
- `new-tab --name=<title> --cwd <dir> --layout <file>` opens a tab that keeps the tab bar.
- Renaming is `rename-tab-by-id <id> -- <name>`. `rename-tab` only renames the focused tab.
- `go-to-tab-by-id <id>` switches tabs.
- From outside a session, `zellij --session <ws> --new-session-with-layout <file>` starts the session with one tab per saved session.

## kitty

Needs remote control. `kitten @ ls`, `launch --type=os-window`, `launch --type=tab --match=window_id:<id>`, `set-tab-title --match=window_id:<id> -- <title>` and `focus-window --match=id:<id>` all behave as the adapter expects. `launch` prints the new window id, and the second launch opens a tab in the new OS window. `KITTY_PID` and `KITTY_WINDOW_ID` are set in each window's processes. The fixture is trimmed real output.

Commands sent from a process outside kitty need `listen_on` and `KITTY_LISTEN_ON`.

## WezTerm

`wezterm cli list --format json`, `spawn --new-window`, `spawn --window-id`, and `set-tab-title --pane-id <id> -- <title>` behave as expected. `spawn` prints the pane id. The fixture is trimmed real output.

`wezterm cli spawn` passes the caller's environment to the new pane. Run from inside a Claude session, that made the restored sessions start as child sessions with transcript saving off. ccshift now unsets Claude Code's per-session variables when it starts.

Focus runs `activate-tab --tab-id` and then `activate-pane --pane-id`. A handoff tab is added at the end of the window: `wezterm cli` has no way to place or move a tab.

## Found along the way

- Starting `claude` in a folder that has never been trusted shows the trust prompt in the restored tab.
- A terminal started from a shell that Claude Code launched inherits that session's variables. ccshift clears them for what it launches, but a terminal you start yourself with `!kitty` from inside Claude will not be clean.

