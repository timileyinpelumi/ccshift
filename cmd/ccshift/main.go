package main

import (
	"context"
	"fmt"
	"os"

	"github.com/timileyinpelumi/ccshift/internal/cli"
)

func main() {
	app, err := cli.NewApp()
	if err != nil {
		// A bad config must not break the Claude session that runs these.
		if len(os.Args) > 1 && (os.Args[1] == "hook" || os.Args[1] == "statusline") {
			os.Exit(0)
		}
		fmt.Fprintln(os.Stderr, "ccshift:", err)
		os.Exit(1)
	}
	os.Exit(app.Run(context.Background(), os.Args[1:]))
}
