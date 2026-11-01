// ui-probe sends bounded semantic UI debug requests to a local laboratory.
package main

import "github.com/alecthomas/kong"

func main() {
	context := kong.Parse(&command{},
		kong.Name("ui-probe"),
		kong.Description("Inspect or control a laboratory with semantic debugging enabled."))
	context.FatalIfErrorf(context.Run())
}
