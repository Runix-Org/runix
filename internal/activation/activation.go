// Package activation sends validated requests to a local UI process.
package activation

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"runix/internal/commands"
	"runix/internal/localipc"
)

const maxMessage = 4096

type response struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// Listen serves one bounded request on each connection.
func Listen(path string, handler func(context.Context, commands.Request) (func(), error)) (func(), error) {
	if handler == nil {
		return nil, errors.New("activation handler is required")
	}
	return localipc.Listen(path, func(ctx context.Context, conn net.Conn) {
		serve(ctx, conn, handler)
	})
}

// Send sends one validated request. Errors after connection are wrapped as
// uncertain delivery because the receiver may already have acted.
func Send(ctx context.Context, path string, request commands.Request) error {
	if _, err := commands.Resolve(request); err != nil {
		return err
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
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return fmt.Errorf("activation delivery is uncertain: %w", err)
	}
	interrupt := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })
	defer interrupt()
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return fmt.Errorf("activation delivery is uncertain: %w", err)
	}
	var resp response
	decoder := json.NewDecoder(io.LimitReader(conn, maxMessage))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&resp); err != nil {
		return fmt.Errorf("activation delivery is uncertain: %w", err)
	}
	if decoder.Decode(new(any)) != io.EOF {
		return errors.New("activation delivery is uncertain: response must contain one JSON value")
	}
	if resp.Success {
		return nil
	}
	if resp.Error == "" {
		return errors.New("activation request failed")
	}
	return errors.New(resp.Error)
}

func serve(parent context.Context, conn net.Conn, handler func(context.Context, commands.Request) (func(), error)) {
	defer conn.Close()
	started := time.Now()
	_ = conn.SetDeadline(started.Add(2 * time.Second))
	interrupt := context.AfterFunc(parent, func() { _ = conn.SetDeadline(time.Now()) })
	defer interrupt()
	line, err := bufio.NewReaderSize(conn, maxMessage).ReadSlice('\n')
	resp := response{}
	var afterReply func()
	if err != nil || len(line) > maxMessage {
		resp.Error = "Request must be one bounded JSON line."
	} else {
		var request commands.Request
		decoder := json.NewDecoder(bytes.NewReader(line[:len(line)-1]))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil || decoder.Decode(new(any)) != io.EOF {
			resp.Error = "Request must contain one valid JSON object."
		} else if _, err := commands.Resolve(request); err != nil {
			resp.Error = err.Error()
		} else {
			ctx, cancel := context.WithDeadline(parent, started.Add(2*time.Second))
			afterReply, err = handler(ctx, request)
			cancel()
			if err != nil {
				afterReply = nil
				resp.Error = err.Error()
			} else {
				resp.Success = true
			}
		}
	}
	_ = conn.SetWriteDeadline(time.Now().Add(250 * time.Millisecond))
	_ = json.NewEncoder(conn).Encode(resp)
	if afterReply != nil {
		afterReply()
	}
}
