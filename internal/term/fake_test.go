package term

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"time"
)

type fakeExec struct {
	calls   [][]string
	outputs map[string]string // command line prefix -> stdout
	missing map[string]bool   // binaries LookPath should not find
	fail    map[string]string // command line prefix -> error text
	out     bytes.Buffer
	env     map[string]string
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
