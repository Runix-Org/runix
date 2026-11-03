package main

import (
	"runix/internal/cli"

	"github.com/alecthomas/kong"
)

func main() {
	context := kong.Parse(&cli.Command{},
		kong.Name("runix"),
		kong.Description("Show installed applications with a single runix instance.\n\n"+
			"Examples:\n  runix\n  runix cmd full\n  runix control quit\n\n"+
			"A cold invocation stays in the foreground. Later calls return after acceptance, not execution."))
	context.FatalIfErrorf(context.Run())
}
