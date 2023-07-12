package main

import (
	"context"
	_ "embed"
	"github.com/raft-tech/syncd/cmd"
	"os"
	"os/signal"
)

//go:embed VERSION
var version string

func init() {
	cmd.Version = version
}

func main() {

	// Create a context that is canceled by OS signal
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Run the root command
	if err := cmd.New(cmd.DefaultOptions).ExecuteContext(ctx); err != nil {
		code := 1
		if err, ok := err.(cmd.Error); ok {
			code = err.Code()
		}
		os.Exit(code)
	}
}
