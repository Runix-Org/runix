package main

import "runix/internal/uidebug"

type command struct {
	Socket   string          `required:"" help:"Connect to an enabled UI debug socket." placeholder:"PATH"`
	Snapshot snapshotCommand `cmd:"" help:"Read the current semantic UI snapshot."`
	Action   actionCommand   `cmd:"" help:"Send an action to a registered UI control."`
}

type snapshotCommand struct{}

func (snapshot *snapshotCommand) Run(command *command) error {
	return sendRequest(command.Socket, uidebug.Request{Op: "snapshot"})
}

type actionCommand struct {
	Target string `arg:"" help:"Registered control ID."`
	Action string `arg:"" help:"Action supported by this control."`
	Text   string `arg:"" optional:"" passthrough:"all" help:"Optional action text."`
}

func (action *actionCommand) Run(command *command) error {
	return sendRequest(command.Socket, uidebug.Request{
		Op: "action", Target: action.Target, Action: action.Action, Text: action.Text,
	})
}
