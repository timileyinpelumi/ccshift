#!/bin/bash
t() { local s=$1 i; for ((i = 0; i < ${#s}; i++)); do tmux send-keys -t default:0 -l "${s:i:1}"; sleep 0.045; done; }
enter() { tmux send-keys -t default:0 Enter; }
sleep 1.2
t "ccshift ls"; sleep 0.3; enter; sleep 3.8
t "ccshift save"; sleep 0.3; enter; sleep 2
t "# restart the machine"; sleep 0.4; enter
sleep 0.6
for w in 3 2 1; do tmux kill-window -t default:$w; sleep 0.35; done
rm -f /demo/agents/*.json
sleep 1
tmux send-keys -t default:0 C-l; sleep 0.5
t "ccshift restore"; sleep 0.3; enter; sleep 1.6
tmux select-window -t default:1; sleep 1.4
tmux select-window -t default:0; sleep 2.4
tmux send-keys -t default:0 C-l; sleep 0.4
t "ccshift find refund webhook"; sleep 0.3; enter; sleep 3.4
enter; sleep 0.8
tmux send-keys -t default:0 C-l; sleep 0.4
t "ccshift"; sleep 0.3; enter; sleep 1.6
t "infra"; sleep 1.2
enter; sleep 2.2
tmux kill-server
