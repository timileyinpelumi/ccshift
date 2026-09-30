#!/bin/bash
set -e
export HOME=/home/dev
mkdir -p /home/dev
mkdir -p /demo/agents /demo/names /home/dev/.config/ccshift
cp /rec/ccshift /usr/local/bin/ccshift
install -m 0755 /rec/claude /usr/local/bin/claude
printf 'auto_update = false\nsupport_note = false\n' >/home/dev/.config/ccshift/config.toml
git config --global user.email d@example.com; git config --global user.name demo; git config --global init.defaultBranch main
repo() { mkdir -p /home/dev/acme/$1; cd /home/dev/acme/$1; git init -q; git commit -q --allow-empty -m init; [ "$2" = main ] || git checkout -q -b "$2"; }
repo api PAY-2193-refund-webhooks
repo web main
repo infra OPS-664-rotate-certs
transcript() { # id dir mtime lines...
	local id=$1 dir=$2 when=$3; shift 3
	local p=/home/dev/.claude/projects/-home-dev-acme-$dir
	mkdir -p "$p"
	printf '%s\n' "$@" >"$p/$id.jsonl"
	touch -d "$when" "$p/$id.jsonl"
}
A=7f3c2a10-5b1e-4c8a-9d2f-1a2b3c4d5e6f W=c41d88e2-0a9b-4f3e-8c1d-2e3f4a5b6c7d I=e9a0b7c3-6d2f-4b1a-8e9c-3f4a5b6c7d8e O=2b8e41f0-9c3d-4e5f-a6b7-c8d9e0f1a2b3
transcript $A api "10 min ago" '{"type":"user","cwd":"/home/dev/acme/api","message":{"content":"the refund webhook retries forever when the provider returns 409"}}' '{"type":"custom-title","customTitle":"api · PAY-2193"}'
transcript $W web "25 min ago" '{"type":"user","cwd":"/home/dev/acme/web","message":{"content":"the checkout form loses the coupon on back navigation"}}' '{"type":"custom-title","customTitle":"web · checkout form"}'
transcript $I infra "40 min ago" '{"type":"user","cwd":"/home/dev/acme/infra","message":{"content":"rotate the ingress certs before friday"}}' '{"type":"custom-title","customTitle":"infra · OPS-664"}'
transcript $O api "6 days ago" '{"type":"user","cwd":"/home/dev/acme/api","message":{"content":"add idempotency keys to the refund webhook handler"}}' '{"type":"custom-title","customTitle":"api · PAY-2107"}'
echo "api · PAY-2193" >/demo/names/$A; echo "web · checkout form" >/demo/names/$W; echo "infra · OPS-664" >/demo/names/$I

tmux -2 -f /dev/null new-session -d -e HOME=/home/dev -s default -x 100 -y 26 -n shell "bash --rcfile /rec/bashrc"
tmux set -g default-terminal tmux-256color
tmux set -as terminal-features ",xterm-256color:RGB"
tmux set -g status-style 'bg=#1E2B57,fg=#c8d0e0'
tmux setw -g window-status-format ' #W '
tmux setw -g window-status-current-format '#[bg=#F2B84B,fg=#0F1B3D,bold] #W #[default]'
tmux setw -g window-status-current-style 'bg=#F2B84B,fg=#0F1B3D,bold'
tmux set -g status-left '' ; tmux set -g status-right '' ; tmux set -g window-status-separator ''
for x in "api $A" "web $W" "infra $I"; do
	set -- $x
	tmux new-window -d -t default: -n "$(cat /demo/names/$2)" -c /home/dev/acme/$1 "claude --session-id $2"
done
sleep 1
for x in "$A 34" "$W 12" "$I 78"; do
	set -- $x
	echo "{\"session_id\":\"$1\",\"context_window\":{\"used_percentage\":$2,\"context_window_size\":200000}}" | ccshift statusline >/dev/null
done
