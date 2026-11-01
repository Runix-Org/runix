package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"runix/internal/uidebug"
)

type response struct {
	Version int             `json:"version"`
	ID      string          `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *uidebug.Error  `json:"error"`
}

func sendRequest(path string, request uidebug.Request) error {
	request.Version = uidebug.Version
	request.ID = fmt.Sprintf("probe-%d", time.Now().UnixNano())
	request.DeadlineMS = time.Now().Add(1500 * time.Millisecond).UnixMilli()
	dialer := net.Dialer{Timeout: 500 * time.Millisecond}
	conn, err := dialer.DialContext(context.Background(), "unix", path)
	if err != nil {
		return errors.New("cannot connect to debug socket")
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	encoded, err := json.Marshal(request)
	if err != nil {
		return errors.New("cannot encode request")
	}
	if len(encoded)+1 > uidebug.MaxMessage {
		return errors.New("request is too large")
	}
	if _, err = conn.Write(append(encoded, '\n')); err != nil {
		return errors.New("cannot send request")
	}
	line, err := bufio.NewReaderSize(conn, uidebug.MaxMessage).ReadSlice('\n')
	if err != nil || len(line) > uidebug.MaxMessage {
		return errors.New("debug socket returned no valid response")
	}
	var resp response
	if json.Unmarshal(line, &resp) != nil || resp.Version != uidebug.Version || resp.ID != request.ID {
		return errors.New("debug socket returned an invalid response")
	}
	if resp.Error != nil {
		return fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
	}
	if !json.Valid(resp.Result) {
		return errors.New("debug socket returned an invalid result")
	}
	_, err = os.Stdout.Write(append(resp.Result, '\n'))
	return err
}
