package main

import (
	"context"
	"time"

	"runix/internal/activation"
)

type command struct {
	Socket    string `required:"" help:"Path to the UI activation socket." placeholder:"PATH"`
	Operation string `arg:"" name:"command" enum:"show,hide,toggle" help:"Window visibility command."`
}

func (command *command) Run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return activation.Send(ctx, command.Socket, command.Operation)
}
