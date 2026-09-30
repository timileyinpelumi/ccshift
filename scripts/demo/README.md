# Demo recording

Records `assets/demo.gif` in a container: a real tmux with three stand-in `claude` sessions, typed into by `drive.sh` while asciinema records.

```
GOOS=linux CGO_ENABLED=0 go build -o scripts/demo/ccshift ./cmd/ccshift
docker build -t ccshift-rec scripts/demo
mkdir -p scripts/demo/out
docker run --rm -t -e TERM=xterm-256color -v /usr/share/fonts:/fonts:ro -v "$PWD/scripts/demo:/rec" ccshift-rec /rec/record.sh
cp scripts/demo/out/demo.gif scripts/demo/out/demo.cast assets/
```
