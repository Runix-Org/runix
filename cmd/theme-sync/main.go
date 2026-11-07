// theme-sync is a development tool; it is not part of the launcher runtime.
package main

import (
	"context"
	"os"

	"runix/internal/themesync"

	"github.com/alecthomas/kong"
)

func main() {
	var command themesync.Command
	parsed := kong.Parse(&command,
		kong.Description("Inventory original Raycast v1 themes. No data changes without --write. "+
			"Example: theme-sync --checkout /tmp/ray-so"))
	parsed.FatalIfErrorf(command.Run(context.Background(), os.Stdout))
}
