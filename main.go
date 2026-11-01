package main

import (
	"runix/internal/cli"

	"github.com/alecthomas/kong"
)

func main() {
	context := kong.Parse(&cli.Command{},
		kong.Name("runix"),
		kong.Description("Show a searchable list of installed applications."))
	context.FatalIfErrorf(context.Run())
}
