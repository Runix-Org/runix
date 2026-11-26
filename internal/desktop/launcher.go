package desktop

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

// HandoffTimeout bounds queueing, connection setup, and default-action dispatch.
const HandoffTimeout = 5 * time.Second

// Launcher owns a lazy session-bus connection and serializes handoffs. Use it off
// the GUI thread. Close cancels and joins handoff work, not launched applications.
type Launcher struct {
	environment []string
	address     string
	ctx         context.Context
	cancel      context.CancelFunc
	gate        chan struct{}
	closeOnce   sync.Once
	conn        *sessionConnection
}

type sessionConnection struct {
	*dbus.Conn
	socket net.Conn
}

// NewLauncher captures a session environment, normally os.Environ(). It does not
// connect or launch anything. Linux session buses use unix:path or unix:abstract.
// No implicit bus discovery, autolaunch, or process-global environment is used.
func NewLauncher(environment []string) *Launcher {
	ctx, cancel := context.WithCancel(context.Background())
	launcher := &Launcher{
		environment: slices.Clone(environment), ctx: ctx, cancel: cancel,
		gate: make(chan struct{}, 1),
	}
	launcher.gate <- struct{}{}
	for _, variable := range environment {
		if address, ok := strings.CutPrefix(variable, "DBUS_SESSION_BUS_ADDRESS="); ok {
			launcher.address = address
		}
	}
	return launcher
}

// Close is idempotent and waits for active handoff work to stop.
func (l *Launcher) Close() {
	l.closeOnce.Do(func() {
		l.cancel()
		<-l.gate
		if l.conn != nil {
			_ = l.conn.Close()
		}
		l.gate <- struct{}{}
	})
}

// Launch dispatches a selected visible entry. Success means Start or Activate
// returned, not window readiness. A sent D-Bus request may activate an application
// even if cancellation or timeout is reported. Never retry it or fall back to Exec.
// The caller must validate the current catalog generation before submission.
func (l *Launcher) Launch(ctx context.Context, entry Entry) error {
	ctx, cancel := context.WithTimeout(ctx, HandoffTimeout)
	defer cancel()
	stop := context.AfterFunc(l.ctx, cancel)
	defer stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-l.gate:
	}
	defer func() { l.gate <- struct{}{} }()
	if err := l.ctx.Err(); err != nil {
		return fmt.Errorf("application launcher is closed: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if entry.Disposition != Visible || entry.Type != "Application" {
		return fmt.Errorf("selected desktop entry is not a visible application")
	}
	if !entry.DBusActivatable {
		return launchExec(ctx, entry, l.environment)
	}
	name, path, err := applicationBusTarget(entry.Source.ID)
	if err != nil {
		return err
	}
	if l.conn == nil || l.conn.Context().Err() != nil {
		if l.conn != nil {
			_ = l.conn.Close()
		}
		l.conn, err = connectSession(ctx, l.address)
		if err != nil {
			return fmt.Errorf("cannot connect to session D-Bus: %w", err)
		}
	}
	socket := l.conn.socket
	interrupt := context.AfterFunc(ctx, func() { _ = socket.Close() })
	// Normal flags permit bus service activation; platform data never reuses tokens.
	err = l.conn.Object(name, path).CallWithContext(ctx,
		"org.freedesktop.Application.Activate", 0, map[string]dbus.Variant{}).Err
	stopped := interrupt()
	if !stopped || ctx.Err() != nil {
		_ = socket.Close()
		_ = l.conn.Close()
		return fmt.Errorf("D-Bus activation canceled (application may already have started): %w", ctx.Err())
	}
	if err != nil {
		return fmt.Errorf("cannot activate D-Bus application %q (it may already have started): %w", name, err)
	}
	return nil
}

func applicationBusTarget(id string) (string, dbus.ObjectPath, error) {
	name, suffix := strings.CutSuffix(id, ".desktop")
	valid := suffix && len(name) <= 255 && strings.ContainsRune(name, '.')
	for _, part := range strings.Split(name, ".") {
		if part == "" {
			valid = false
		}
		for index, char := range part {
			switch {
			case char >= 'a' && char <= 'z', char >= 'A' && char <= 'Z', char == '_', char == '-',
				index > 0 && char >= '0' && char <= '9':
			default:
				valid = false
			}
		}
	}
	if !valid {
		return "", "", fmt.Errorf("invalid D-Bus application desktop ID %q", id)
	}
	path := "/" + strings.ReplaceAll(strings.ReplaceAll(name, ".", "/"), "-", "_")
	return name, dbus.ObjectPath(path), nil
}

// godbus Dial does not accept a dialing context. Dial the Linux Unix transport
// explicitly so connect, authentication, and Hello all share the handoff bound.
func connectSession(ctx context.Context, address string) (*sessionConnection, error) {
	var lastErr error
	for _, candidate := range strings.Split(address, ";") {
		path, err := sessionSocket(candidate)
		if err != nil {
			lastErr = err
			continue
		}
		var dialer net.Dialer
		socket, dialErr := dialer.DialContext(ctx, "unix", path)
		if dialErr != nil {
			lastErr = dialErr
			continue
		}
		stop := context.AfterFunc(ctx, func() { _ = socket.Close() })
		unixSocket, ok := socket.(*net.UnixConn)
		if !ok {
			stop()
			_ = socket.Close()
			return nil, fmt.Errorf("session D-Bus connection is not a Unix socket")
		}
		conn, connectErr := dbus.ConnectUnix(unixSocket)
		stopped := stop()
		if connectErr == nil && stopped && ctx.Err() == nil {
			return &sessionConnection{Conn: conn, socket: socket}, nil
		}
		_ = socket.Close()
		if conn != nil {
			_ = conn.Close()
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		lastErr = connectErr
	}
	return nil, lastErr
}

func sessionSocket(address string) (string, error) {
	transport, fields, _ := strings.Cut(address, ":")
	if transport != "unix" {
		return "", fmt.Errorf("set DBUS_SESSION_BUS_ADDRESS to a Unix session bus address")
	}
	var path, abstract string
	for _, field := range strings.Split(fields, ",") {
		key, encoded, _ := strings.Cut(field, "=")
		value, err := url.PathUnescape(encoded)
		if err != nil {
			return "", fmt.Errorf("invalid session D-Bus address: %w", err)
		}
		switch key {
		case "path":
			path = value
		case "abstract":
			abstract = value
		}
	}
	if path != "" && abstract == "" && strings.HasPrefix(path, "/") {
		return path, nil
	}
	if abstract != "" && path == "" {
		return "@" + abstract, nil
	}
	return "", fmt.Errorf("session D-Bus address needs one absolute path or abstract socket")
}
