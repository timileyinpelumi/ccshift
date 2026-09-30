package term

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"time"
)

type fakeExec struct {
	calls     [][]string
	outputs   map[string]string // command line prefix -> stdout
	missing   map[string]bool   // binaries LookPath should not find
	fail      map[string]string // command line prefix -> error text
	paths     string            // what "qdbus <service>" with no path prints
	spawnFail map[string]bool   // Spawn fails when any argument starts with one of these
	out       bytes.Buffer
	env       map[string]string
}

func (f *fakeExec) exec() Exec {
	if f.env == nil {
		f.env = map[string]string{"DISPLAY": ":0"}
	}
	return Exec{
		Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			argv := append([]string{name}, args...)
			f.calls = append(f.calls, argv)
			line := strings.Join(argv, " ")
			if len(argv) == 2 && strings.HasPrefix(argv[0], "qdbus") {
				return []byte(f.paths), nil
			}
			for k, v := range f.fail {
				if strings.HasPrefix(line, k) {
					return nil, errors.New(v)
				}
			}
			for k, v := range f.outputs {
				if strings.HasPrefix(line, k) {
					return []byte(v), nil
				}
			}
			return nil, nil
		},
		Spawn: func(name string, args ...string) error {
			f.calls = append(f.calls, append([]string{name}, args...))
			for _, arg := range args {
				for p := range f.spawnFail {
					if strings.HasPrefix(arg, p) {
						return errors.New("exited right away")
					}
				}
			}
			return nil
		},
		LookPath: func(s string) (string, error) {
			if f.missing[s] {
				return "", errors.New("not found")
			}
			return "/usr/bin/" + s, nil
		},
		Sleep: func(time.Duration) {},
		Env:   f.env,
		Out:   &f.out,
	}
}
