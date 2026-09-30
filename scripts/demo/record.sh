#!/bin/bash
set -e
export TERM=xterm-256color
/rec/setup.sh >/dev/null 2>&1
/rec/drive.sh &
asciinema rec -q --overwrite --cols 100 --rows 26 -c "tmux -2 attach -t default" /rec/out/raw.cast
wait
# Drop the lines tmux prints when the server exits.
head -n -2 /rec/out/raw.cast >/rec/out/demo.cast
agg --font-dir /fonts --font-family "JetBrains Mono,DejaVu Sans Mono" --font-size 16 --line-height 1.3 --theme 0F1B3D,c8d0e0,1c2541,e06c75,98c379,F2B84B,61afef,c678dd,56b6c2,c8d0e0,5c6370,e06c75,98c379,F2B84B,61afef,c678dd,56b6c2,ffffff /rec/out/demo.cast /rec/out/demo.gif
