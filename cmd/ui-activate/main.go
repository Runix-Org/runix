package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"runix/internal/activation"
)

func main() {
	flags := flag.NewFlagSet("ui-activate", flag.ContinueOnError)
	path := flags.String("socket", "", "Path to the UI activation socket")
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "Send a show, hide, or toggle command to a running runix UI.")
		fmt.Fprintln(flags.Output(), "Usage: ui-activate --socket PATH show|hide|toggle")
		flags.PrintDefaults()
	}
	if err := flags.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	args := flags.Args()
	if *path == "" || len(args) != 1 {
		flags.Usage()
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := activation.Send(ctx, *path, args[0]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
