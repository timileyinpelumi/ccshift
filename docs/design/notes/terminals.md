# Terminal checks

What was run against each real terminal, on 2026-09-30, Ubuntu 24.04, Claude Code 2.1.285. Each check restored two throwaway sessions (one named `-rt beta's`, to cover a leading dash and a quote) through the `ccshift` binary with a scratch state directory, then ran `ccshift ls` and `ccshift focus`.

| Terminal | Version | Restore | ls tab order | Titles | Focus |
|---|---|---|---|---|---|
| tmux | 3.4 | ok | ok | ok | commands accepted |
| zellij | 0.45.1 | ok, inside a session and from outside | ok | ok | ok (active tab confirmed with `list-tabs`) |
| kitty | latest from kovidgoyal.net | ok, new OS window with tabs | ok | ok | ok (focused tab confirmed with `kitten @ ls`) |
| WezTerm | latest AppImage | ok, new window with tabs | ok | ok | commands accepted, not confirmed on screen |
| gnome-terminal | Ubuntu 24.04 | ok | not readable, kept from the last restore | from the Claude session name | not supported |
| Konsole, Tilix, xfce4-terminal | | not run, need `sudo apt install` | | | |
| Ptyxis | | not run, not packaged for Ubuntu 24.04 | | | |
| Ghostty, Alacritty, foot | | not run | | | |

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

Focus runs `activate-tab --tab-id` and then `activate-pane --pane-id`. Both commands succeed. I could not confirm on screen that the tab switched, because `list-clients` only reports the focused pane of a window that has keyboard focus.

## gnome-terminal

`--window`, then `--tab` for each further session, with `--working-directory` and `--`. Both sessions started and `ls` listed them in the saved order. Whether the second one landed as a tab in the new window was not confirmed on screen.

## Found along the way

- Starting `claude` in a folder that has never been trusted shows the trust prompt in the restored tab.
- A terminal started from a shell that Claude Code launched inherits that session's variables. ccshift clears them for what it launches, but a terminal you start yourself with `!kitty` from inside Claude will not be clean.

## Still to run

```
sudo apt install -y konsole tilix xfce4-terminal
```

Then, in each: `ccshift save`, close the window, `ccshift restore`, and check that the first session opens a new window and the rest open as tabs in it, in order, in the right directories.

For Konsole, also check whether D-Bus reports tabs in on-screen order, which would let it move to exact order. Open four tabs, drag the last to second place, then:

```
qdbus $KONSOLE_DBUS_SERVICE $KONSOLE_DBUS_WINDOW sessionList
for s in $(qdbus $KONSOLE_DBUS_SERVICE $KONSOLE_DBUS_WINDOW sessionList); do qdbus $KONSOLE_DBUS_SERVICE /Sessions/$s title 1; done
```
