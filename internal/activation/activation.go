// Package activation sends show, hide, and toggle requests to a local UI process.
package activation

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"time"

	"runix/internal/localipc"
)

const maxMessage = 4096

type request struct {
	Command string `json:"command"`
}

type response struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// Listen serves one bounded activation request on each connection.
func Listen(path string, handler func(context.Context, string) error) (func(), error) {
	if handler == nil {
		return nil, errors.New("activation handler is required")
	}
	return localipc.Listen(path, func(ctx context.Context, conn net.Conn) {
		serve(ctx, conn, handler)
	})
}

// Send sends one activation command and returns any transport or handler error.
func Send(ctx context.Context, path, command string) error {
	if !validCommand(command) {
		return errors.New("command must be show, hide, or toggle")
	}
	if ctx == nil {
		return errors.New("context is required")
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", path)
	if err != nil {
		return err
	}
	defer conn.Close()
	deadline := time.Now().Add(2 * time.Second)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}
	interrupt := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })
	defer interrupt()
	if err := json.NewEncoder(conn).Encode(request{Command: command}); err != nil {
		return err
	}
	var resp response
	decoder := json.NewDecoder(io.LimitReader(conn, maxMessage))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&resp); err != nil {
		return err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return errors.New("activation response must contain one JSON value")
	}
	if resp.Success {
		return nil
	}
	if resp.Error == "" {
		return errors.New("activation request failed")
	}
	return errors.New(resp.Error)
}

func serve(parent context.Context, conn net.Conn, handler func(context.Context, string) error) {
	defer conn.Close()
	started := time.Now()
	_ = conn.SetDeadline(started.Add(2 * time.Second))
	interrupt := context.AfterFunc(parent, func() { _ = conn.SetDeadline(time.Now()) })
	defer interrupt()
	line, err := bufio.NewReaderSize(conn, maxMessage).ReadSlice('\n')
	resp := response{}
	if err != nil || len(line) > maxMessage {
		resp.Error = "Request must be one bounded JSON line."
	} else {
		var req request
		decoder := json.NewDecoder(bytes.NewReader(line[:len(line)-1]))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil || decoder.Decode(new(any)) != io.EOF || !validCommand(req.Command) {
			resp.Error = "Request must contain one supported command."
		} else {
			ctx, cancel := context.WithDeadline(parent, started.Add(2*time.Second))
			err := handler(ctx, req.Command)
			requestErr := ctx.Err()
			cancel()
			if err != nil {
				resp.Error = err.Error()
			} else if requestErr != nil {
				resp.Error = "Request was canceled or reached its deadline."
			} else {
				resp.Success = true
			}
		}
	}
	_ = conn.SetWriteDeadline(time.Now().Add(250 * time.Millisecond))
	_ = json.NewEncoder(conn).Encode(resp)
}

func validCommand(command string) bool {
	return command == "show" || command == "hide" || command == "toggle"
}
