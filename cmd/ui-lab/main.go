// ui-lab exercises the Go/QML bridge without changing the root starter.
package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"runix/internal/ui/qmlbridge"
)

//go:embed Main.qml
var document []byte

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	hyprlandWindow := flag.Bool("hyprland-window", false, "Use Hyprland IPC for window sizing, placement, and focus")
	debugSocket := flag.String("debug-socket", "", "Enable the development debug socket at this private path")
	activationSocket := flag.String("activation-socket", "",
		"Use this private activation path; default: a new temporary directory")
	startHidden := flag.Bool("start-hidden", false, "Start hidden and wait for activation")
	flag.Parse()
	if *activationSocket == "" {
		directory, err := os.MkdirTemp("", "runix-activation.")
		if err != nil {
			return fmt.Errorf("create activation directory: %w", err)
		}
		defer func() { _ = os.Remove(directory) }()
		*activationSocket = filepath.Join(directory, "ui.sock")
	}
	fmt.Printf("Activation socket: %s\n", *activationSocket)
	args := append([]string{os.Args[0]}, flag.Args()...)
	options := qmlbridge.Options{
		HyprlandWindow:   *hyprlandWindow,
		DebugSocket:      *debugSocket,
		ActivationSocket: *activationSocket,
		StartHidden:      *startHidden,
	}
	return qmlbridge.Run(args, document, runQuery, options)
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
