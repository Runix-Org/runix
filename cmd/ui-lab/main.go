// ui-lab exercises the Go/QML bridge independently of the product launcher.
package main

import "github.com/alecthomas/kong"

func main() {
	context := kong.Parse(&command{},
		kong.Name("ui-lab"),
		kong.Description("Run the development UI laboratory."))
	context.FatalIfErrorf(context.Run())
}
