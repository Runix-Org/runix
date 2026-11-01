package main

import "github.com/alecthomas/kong"

func main() {
	context := kong.Parse(&command{},
		kong.Name("ui-activate"),
		kong.Description("Send a show, hide, or toggle command to a running runix UI."))
	context.FatalIfErrorf(context.Run())
}
