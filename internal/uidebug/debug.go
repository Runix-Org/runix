// Package uidebug provides a bounded local transport for semantic UI debugging.
package uidebug

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"regexp"
	"time"

	"runix/internal/localipc"
)

const (
	Version     = 1
	MaxMessage  = 65536
	maxLifetime = 2 * time.Second
)

type Request struct {
	Version    int    `json:"version"`
	ID         string `json:"id"`
	Op         string `json:"op"`
	Target     string `json:"target,omitempty"`
	Action     string `json:"action,omitempty"`
	Text       string `json:"text,omitempty"`
	DeadlineMS int64  `json:"deadline_ms"`
}
type (
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	Handler  func(context.Context, Request) (json.RawMessage, *Error)
	response struct {
		Version int             `json:"version"`
		ID      string          `json:"id"`
		Result  json.RawMessage `json:"result,omitempty"`
		Error   *Error          `json:"error,omitempty"`
	}
)

var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func Listen(path string, handler Handler) (func(), error) {
	if handler == nil {
		return nil, errors.New("debug socket requires an absolute path and handler")
	}
	serveStop, err := localipc.Listen(path, func(ctx context.Context, conn net.Conn) {
		serve(ctx, conn, handler)
	})
	if err != nil {
		return nil, errors.New("cannot create debug socket")
	}
	return serveStop, nil
}

func serve(parent context.Context, conn net.Conn, handler Handler) {
	defer conn.Close()
	started := time.Now()
	// Bound writes even for malformed clients and interrupt reads during shutdown.
	_ = conn.SetWriteDeadline(started.Add(maxLifetime + 250*time.Millisecond))
	interrupt := context.AfterFunc(parent, func() { _ = conn.SetDeadline(time.Now()) })
	defer interrupt()
	_ = conn.SetReadDeadline(started.Add(maxLifetime))
	line, err := bufio.NewReaderSize(conn, MaxMessage).ReadSlice('\n')
	if err != nil || len(line) > MaxMessage {
		writeResponse(conn, response{Version: Version, Error: &Error{
			Code: "invalid_request", Message: "Request must be one bounded JSON line.",
		}})
		return
	}
	var req Request
	decoder := json.NewDecoder(bytes.NewReader(line[:len(line)-1]))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&req); err != nil || decoder.Decode(new(any)) != io.EOF ||
		req.Version != Version || !idPattern.MatchString(req.ID) {
		writeResponse(conn, response{Version: Version, ID: req.ID, Error: &Error{
			Code: "invalid_request", Message: "Request JSON, version, or ID is invalid.",
		}})
		return
	}
	resp := response{Version: Version, ID: req.ID}
	if req.DeadlineMS <= started.UnixMilli() || req.DeadlineMS > started.Add(maxLifetime).UnixMilli() {
		resp.Error = &Error{Code: "invalid_deadline", Message: "Deadline must be within two seconds."}
	} else if req.Op == "" || len(req.Op) > 64 || len(req.Target) > 256 ||
		len(req.Action) > 64 || len(req.Text) > 4096 {
		resp.Error = &Error{Code: "invalid_request", Message: "Operation, target, action, or text exceeds its limit."}
	} else {
		ctx, cancel := context.WithDeadline(parent, time.UnixMilli(req.DeadlineMS))
		readingDone := make(chan struct{})
		go func() {
			defer close(readingDone)
			var extra [1]byte
			_, readErr := conn.Read(extra[:])
			if readErr == nil || !errors.Is(readErr, os.ErrDeadlineExceeded) {
				cancel()
			}
		}()
		result, handlerError := handler(ctx, req)
		requestErr := ctx.Err()
		cancel()
		_ = conn.SetReadDeadline(time.Now())
		<-readingDone
		if handlerError != nil {
			resp.Error = handlerError
		} else if requestErr != nil {
			resp.Error = &Error{Code: "timeout", Message: "Request was canceled or reached its deadline."}
		} else if !json.Valid(result) {
			resp.Error = &Error{Code: "invalid_result", Message: "Handler returned an invalid result."}
		} else {
			resp.Result = result
		}
	}
	_ = conn.SetWriteDeadline(time.Now().Add(250 * time.Millisecond))
	writeResponse(conn, resp)
}

func writeResponse(conn net.Conn, resp response) {
	b, err := json.Marshal(resp)
	if err != nil || len(b)+1 > MaxMessage {
		resp.Result = nil
		resp.Error = &Error{Code: "response_too_large", Message: "Response exceeds the protocol size limit."}
		b, err = json.Marshal(resp)
	}
	if err == nil && len(b)+1 <= MaxMessage {
		_, _ = conn.Write(append(b, '\n'))
	}
}
