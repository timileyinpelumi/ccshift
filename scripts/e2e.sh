#!/usr/bin/env bash
# End-to-end check against a real tmux with a stand-in claude: restore a saved layout, then read
# tab order back. Run by CI on Linux and macOS.   usage: scripts/e2e.sh <path to ccshift>
set -euo pipefail
bin=$(cd "$(dirname "$1")" && pwd -P)/$(basename "$1")
work=$(cd "$(mktemp -d)" && pwd -P)
export HOME=$work/home XDG_STATE_HOME=$work/state XDG_CONFIG_HOME=$work/config CLAUDE_CONFIG_DIR=$work/claude TMUX_TMPDIR=$work/tmux
unset TMUX TMUX_PANE
mkdir -p "$HOME" "$work/bin" "$work/one" "$work/two dir" "$CLAUDE_CONFIG_DIR/projects/p" "$TMUX_TMPDIR" "$XDG_STATE_HOME/ccshift/workspaces/default"
trap 'tmux kill-server 2>/dev/null || true; rm -rf "$work"' EXIT
fail() { echo "FAIL: $*" >&2; exit 1; }

cat > "$work/bin/claude" <<F
#!/bin/sh
if [ "\$1" = agents ]; then cat "$work/agents.json" 2>/dev/null || echo '[]'; exit 0; fi
exec sleep 300
F
chmod +x "$work/bin/claude"
export PATH=$work/bin:$PATH

for id in aaaa1111 bbbb2222; do echo '{}' > "$CLAUDE_CONFIG_DIR/projects/p/$id.jsonl"; done
cat > "$XDG_STATE_HOME/ccshift/workspaces/default/latest.json" <<F
{"workspace":"default","saved_at":"2026-09-30T12:00:00Z","terminal":"x","sessions":[
 {"session_id":"aaaa1111","cwd":"$work/one","name":"first one","position":1},
 {"session_id":"bbbb2222","cwd":"$work/two dir","name":"it's second","position":2}]}
F

echo "== restore into tmux"
"$bin" restore --terminal tmux
sleep 2
windows=$(tmux list-windows -a -F '#{window_name}|#{pane_current_path}|#{pane_current_command}')
echo "$windows"
[ "$(echo "$windows" | wc -l | tr -d ' ')" = 2 ] || fail "expected 2 windows"
echo "$windows" | sed -n 1p | grep -q "^first one|$work/one|sleep" || fail "first window is wrong"
echo "$windows" | sed -n 2p | grep -q "^it's second|$work/two dir|sleep" || fail "second window is wrong"

echo "== restore again opens nothing new"
# The stand-in sessions now count as running.
pids=$(tmux list-panes -a -F '#{pane_pid}')
p1=$(echo "$pids" | sed -n 1p); p2=$(echo "$pids" | sed -n 2p)
now=$(date +%s)000
cat > "$work/agents.json" <<F
[{"pid":$p1,"cwd":"$work/one","kind":"interactive","startedAt":$now,"sessionId":"aaaa1111","name":"first one","status":"idle"},
 {"pid":$p2,"cwd":"$work/two dir","kind":"interactive","startedAt":$((now + 1)),"sessionId":"bbbb2222","name":"it's second","status":"busy"}]
F
"$bin" restore --terminal tmux | tee "$work/again.txt"
grep -q "already running" "$work/again.txt" || fail "running sessions should be skipped"
[ "$(tmux list-windows -a | wc -l | tr -d ' ')" = 2 ] || fail "a second restore opened more windows"

echo "== ls follows tab order"
"$bin" ls --terminal tmux | tee "$work/ls1.txt"
sed -n 2p "$work/ls1.txt" | grep -q "first one.*tmux:default:0" || fail "ls did not match the first session to its tab"
tmux swap-window -d -s default:0 -t default:1
"$bin" ls --terminal tmux | tee "$work/ls2.txt"
sed -n 2p "$work/ls2.txt" | grep -q "it's second.*tmux:default:0" || fail "ls did not follow the moved tab"

echo "== save, rename, focus"
"$bin" save --terminal tmux >/dev/null
grep -q '"session_id": "bbbb2222"' "$XDG_STATE_HOME/ccshift/workspaces/default/latest.json" || fail "save did not write the layout"
"$bin" rename "first one" "renamed one" --terminal tmux >/dev/null
tmux list-windows -a -F '#{window_name}' | grep -qx "renamed one" || fail "rename did not set the tab title"
"$bin" focus "renamed one" --terminal tmux
[ "$(tmux display-message -p -t default '#{window_name}')" = "renamed one" ] || fail "focus did not switch windows"
echo "all checks passed on $(uname -s)"
