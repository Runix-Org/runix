// ui-lab exercises the Go/QML bridge without changing the root starter.
package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"runix/internal/ui/qmlbridge"
)

//go:embed Main.qml
var document []byte

func main() {
	hyprlandWindow := flag.Bool("hyprland-window", false, "Use Hyprland IPC to apply this window's requested sizes")
	debugSocket := flag.String("debug-socket", "", "Enable the development debug socket at this private path")
	flag.Parse()
	args := append([]string{os.Args[0]}, flag.Args()...)
	if err := qmlbridge.Run(args, document, runQuery, *hyprlandWindow, *debugSocket); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// The delay is a deterministic laboratory fixture, not adapter behavior.
func runQuery(ctx context.Context, query string) string {
	timer := time.NewTimer(750 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return "Canceled"
	case <-timer.C:
		return "Result: " + strings.ToUpper(query)
	}
}
