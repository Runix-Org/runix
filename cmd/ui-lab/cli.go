package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"runix/internal/search"
	"runix/internal/ui/qmlbridge"
)

//go:embed Main.qml
var document []byte

type command struct {
	HyprlandWindow   bool     `help:"Use Hyprland IPC to control window size, position and focus."`
	DebugSocket      string   `help:"Enable the development debug socket at this private path." placeholder:"PATH"`
	ActivationSocket string   `help:"Use this private activation path. By default, use a new temporary directory."`
	StartHidden      bool     `help:"Start hidden and wait for an activation request."`
	QtArgs           []string `arg:"" optional:"" passthrough:"partial" help:"Arguments to pass to Qt after --."`
}

func (command *command) Run() error {
	endpoint := command.ActivationSocket
	if endpoint == "" {
		directory, err := os.MkdirTemp("", "runix-activation.")
		if err != nil {
			return fmt.Errorf("create activation directory: %w", err)
		}
		defer func() { _ = os.Remove(directory) }()
		endpoint = filepath.Join(directory, "ui.sock")
	}
	fmt.Printf("Activation socket: %s\n", endpoint)
	iconDirectory, iconRefs, iconErr := iconFixture()
	if iconErr != nil {
		return iconErr
	}
	defer func() { _ = os.RemoveAll(iconDirectory) }()
	args := append([]string{os.Args[0]}, command.QtArgs...)
	options := qmlbridge.Options{
		HyprlandWindow:   command.HyprlandWindow,
		DebugSocket:      command.DebugSocket,
		ActivationSocket: endpoint,
		StartHidden:      command.StartHidden,
		Activate:         func(id string) { fmt.Printf("Activated record: %s\n", id) },
		TableRecords:     tableFixture(),
		IconThemePaths:   []string{iconDirectory},
		IconThemes:       []string{"runix-a", "runix-b"},
		DetailContent:    detailFixture,
		DetailLink:       func(url string) { fmt.Printf("Allowed detail link: %s\n", url) },
	}
	index := search.NewIndex(searchFixture(iconRefs), search.DefaultPolicy())
	return qmlbridge.Run(args, document, index.Search, options)
}
