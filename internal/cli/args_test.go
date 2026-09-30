package cli

import (
	"flag"
	"io"
	"reflect"
	"testing"
)

func TestParseArgsFlagsAfterPositionals(t *testing.T) {
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	edit := fs.Bool("edit", false, "")
	term := fs.String("terminal", "", "")
	pos, err := parseArgs(fs, []string{"work", "--edit", "--terminal", "kitty", "extra"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(pos, []string{"work", "extra"}) {
		t.Fatalf("positionals = %v", pos)
	}
	if !*edit || *term != "kitty" {
		t.Fatalf("flags not parsed: edit=%v terminal=%q", *edit, *term)
	}
}

func TestParseArgsUnknownFlag(t *testing.T) {
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if _, err := parseArgs(fs, []string{"--nope"}); err == nil {
		t.Fatal("expected error for unknown flag")
	}
}
