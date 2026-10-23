// ui-probe sends bounded semantic UI debug requests to a local laboratory.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
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

func main() {
	socket := flag.String("socket", "", "Connect to the enabled laboratory debug socket")
	flag.Parse()
	if err := run(*socket, flag.Args()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(path string, args []string) error {
	if path == "" {
		return errors.New("--socket PATH is required")
	}
	if len(args) < 1 || (args[0] != "snapshot" && args[0] != "action") {
		return errors.New("usage: ui-probe --socket PATH snapshot | action TARGET ACTION [TEXT]")
	}
	req := uidebug.Request{
		Version: uidebug.Version, ID: fmt.Sprintf("probe-%d", time.Now().UnixNano()),
		Op: args[0], DeadlineMS: time.Now().Add(1500 * time.Millisecond).UnixMilli(),
	}
	if args[0] == "snapshot" {
		if len(args) != 1 {
			return errors.New("snapshot takes no arguments")
		}
	} else {
		if len(args) < 3 || len(args) > 4 {
			return errors.New("action requires TARGET ACTION and optional TEXT")
		}
		req.Target, req.Action = args[1], args[2]
		if len(args) == 4 {
			req.Text = args[3]
		}
	}
	dialer := net.Dialer{Timeout: 500 * time.Millisecond}
	conn, err := dialer.DialContext(context.Background(), "unix", path)
	if err != nil {
		return errors.New("cannot connect to debug socket")
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	encoded, err := json.Marshal(req)
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
	if json.Unmarshal(line, &resp) != nil || resp.Version != uidebug.Version || resp.ID != req.ID {
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
