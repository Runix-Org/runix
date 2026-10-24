// ui-lab exercises the Go/QML bridge without changing the root starter.
package main

import (
	_ "embed"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"runix/internal/search"
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
		Activate:         func(id string) { fmt.Printf("Activated record: %s\n", id) },
	}
	return qmlbridge.Run(args, document, search.NewIndex(searchFixture()).Search, options)
}

// Deliberate title collisions exercise stable IDs and cross-field ranking.
func searchFixture() []search.Record {
	names := [...]string{"Atlas", "Bridge", "Console", "Delta", "Editor", "Files", "Graph", "Help"}
	categories := [...]string{"Application", "Document", "Command", "Bookmark"}
	records := make([]search.Record, 50_000)
	for index := range records {
		records[index] = search.Record{
			ID:       fmt.Sprintf("record-%05d", index),
			Title:    fmt.Sprintf("%s %03d", names[index%len(names)], index%100),
			Subtitle: fmt.Sprintf("Workspace %02d project %03d", index%17, index%113),
			Category: categories[(index/len(names))%len(categories)],
		}
	}
	return records
}
