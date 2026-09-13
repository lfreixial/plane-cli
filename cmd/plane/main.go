package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/lfreixial/plane-cli/internal/cli"
)

var version = "dev"

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if err := cli.New(version, os.Stdin, os.Stdout, os.Stderr).ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", cli.Safe(err.Error()))
		os.Exit(1)
	}
}
