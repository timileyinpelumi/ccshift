# ccshift

Save the Claude Code sessions you have open in your terminal, in tab order, and bring them all back after a restart with one command.

Site: https://www.timileyin.dev/ccshift

Linux only for now. Works with tmux, kitty, WezTerm and zellij (exact tab order), and gnome-terminal, Ptyxis, Konsole, Tilix, xfce4-terminal, Ghostty, Alacritty and foot (tabs or windows are opened in the saved order, but the order can't be read back from the terminal).

## Install

```
curl -fsSL https://www.timileyin.dev/ccshift/install.sh | sh
```

This downloads the latest release for your machine (x86_64 or arm64), checks it against the published checksum, and puts `ccshift` in `~/.local/bin`. Set `CCSHIFT_INSTALL_DIR` to install somewhere else.

Debian and Ubuntu:

```
curl -fsSLO https://github.com/timileyinpelumi/ccshift/releases/latest/download/ccshift_amd64.deb
sudo apt install ./ccshift_amd64.deb
```

Fedora and RHEL:

```
sudo dnf install https://github.com/timileyinpelumi/ccshift/releases/latest/download/ccshift_amd64.rpm
```

On arm64, replace `amd64` with `arm64`. With Go 1.25 or newer you can also run `go install github.com/timileyinpelumi/ccshift/cmd/ccshift@latest`.

Then set it up and check it:

```
ccshift init
ccshift doctor
```

To upgrade, run the install command again. If the binary moves to a different path, run `ccshift init` again so the hooks point at it.

## Use

```
ccshift ls                  running sessions in tab order
ccshift save                save every workspace
ccshift restore             reopen every saved workspace
ccshift restore work --pick choose an older save of "work"
ccshift focus api           switch to the tab running "api"
ccshift forget old-thing    drop a session from the latest save
ccshift ws                  list workspaces
```

On terminals that can't report tab order, `ccshift save --edit` lets you set the order by hand. A running session whose line you delete there stays out of later autosaves until you run a plain `ccshift save`. After one `restore` through ccshift the order is kept.

## Names

Sessions get a name you can tell apart without setting one:

- `acme · PAY-2193` when the branch has a ticket id in it
- `cvx · compose-v2` on any other branch
- `trueset · Add dark mode` on the default branch, from Claude's own title for the session
- a second session with the same name gets ` #2`

The name follows the branch. A name you set yourself, with `/rename` in Claude or `ccshift rename`, always wins.

```
ccshift new                      start Claude in this tab with a generated name
ccshift new login fix            ...or with your own
ccshift rename 3 login fix       rename session 3 from ccshift ls
ccshift rename . login fix       rename the session you are in (run as !ccshift rename . login fix)
ccshift rename 3 --reset         go back to the generated name
ccshift rename --edit            edit every name in your editor
```

On tmux, kitty, WezTerm and zellij the tab title is kept in step with the name. Claude itself takes a new name when the session is next resumed; for an immediate change run `/rename` there.

A session keeps its ` #2` when another one closes. On a branch like `node-18-upgrade` or `fix-123` the words before the number are not ticket prefixes, so the branch name is used; add your own with `ticket_ignore`.

```toml
ticket_pattern = "\\b([A-Z][A-Z0-9]{1,5})[-_](\\d{2,})\\b"   # prefix and number of a ticket id in a branch name
ticket_ignore = ["fix", "node", "react"]                       # replaces the built-in list
sync_tab_titles = true
```

## Autosave and the context warning

```
ccshift init      add autosave hooks and the statusline to Claude Code
ccshift doctor    check the setup
```

After `init`, the layout is saved as you work, so an unplanned restart doesn't lose it. A session leaves the save when you exit it yourself (`/exit`, Ctrl+D, Ctrl+C). A session that was killed, or whose terminal was closed, stays in and comes back on the next `restore`. Remove one for good with `ccshift forget`.

`ccshift ls` shows how much of its context window each session has used. You get one desktop notification when a session passes 70% and another at 85%. Your own statusline keeps working: ccshift runs it and shows its output.

`ccshift init --remove` takes the hooks and statusline back out.

Settings, in `~/.config/ccshift/config.toml`:

```toml
warn_thresholds = [70, 85]
autosave_debounce_seconds = 30
stale_days = 14          # a saved session that hasn't run for this long is dropped
```

## Handoff

When a session has used most of its context window, continue it in a fresh one:

```
ccshift handoff api              hand off the session named "api"
!ccshift handoff                 ...or, typed inside Claude, the session you are in
ccshift handoff api --edit       read and edit the brief before the new session starts
ccshift handoff api --close-old  close the old tab afterwards (tmux, kitty, WezTerm)
ccshift handoff api --no-launch  only write the brief
ccshift handoff api --reuse      start the new session from the brief already written
```

ccshift reads the session's transcript, has a fresh Claude write a brief from it (goal, decisions, what's done, current state, next steps, open questions, gotchas), and opens a new session named `api (2)` that reads the brief and carries on. The new session takes the old one's place in the saved layout. The brief writer can read the repository but can't change anything.

The context warning tells you when it is time, and names the command.

```toml
brief_model = "sonnet"   # the model that writes the brief
```

## Workspaces

Optional. In `~/.config/ccshift/config.toml`:

```toml
[workspaces]
work = ["~/dev/work"]
personal = ["~/dev/personal"]
```

Sessions outside these paths go into `default`. Each workspace is saved separately and restored into its own window.

## kitty

kitty needs remote control on. Add to `kitty.conf`:

```
allow_remote_control socket-only
listen_on unix:/tmp/kitty-{kitty_pid}
```
