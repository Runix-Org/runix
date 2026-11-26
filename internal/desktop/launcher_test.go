package desktop

import (
	"bufio"
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/stretchr/testify/suite"
)

type DBusLaunchSuite struct{ launchFixtures }

func TestDBusLaunchSuite(t *testing.T) { suite.Run(t, new(DBusLaunchSuite)) }

func (s *DBusLaunchSuite) privateBus() string {
	program, err := exec.LookPath("dbus-daemon")
	if err != nil {
		if os.IsNotExist(err) || errors.Is(err, exec.ErrNotFound) {
			s.T().Skip("verification requires dbus-daemon")
		}
		s.Require().NoError(err)
	}
	directory := s.T().TempDir()
	socketDirectory, err := os.MkdirTemp("", "runix-bus-")
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = os.RemoveAll(socketDirectory) })
	address := "unix:path=" + filepath.Join(socketDirectory, "bus")
	config := `<busconfig><type>session</type><listen>` + address + `</listen><auth>EXTERNAL</auth>` +
		`<servicedir>` + directory + `</servicedir><policy context="default">` +
		`<allow send_destination="*"/><allow receive_sender="*"/><allow own="*"/></policy></busconfig>`
	path := filepath.Join(directory, "bus.conf")
	s.Require().NoError(os.WriteFile(path, []byte(config), 0o600))
	binary, err := os.Executable()
	s.Require().NoError(err)
	service := "[D-BUS Service]\nName=org.runix.Auto\nExec=" + binary + " -test.run=TestDBusActivationProcess\n"
	s.Require().NoError(os.WriteFile(filepath.Join(directory, "org.runix.Auto.service"), []byte(service), 0o600))
	command := exec.CommandContext(s.T().Context(), program, "--nofork", "--config-file="+path, "--print-address=1")
	command.Env = append(os.Environ(), "RUNIX_DBUS_REPORT="+filepath.Join(socketDirectory, "activation"))
	command.Stderr = os.Stderr
	stdout, err := command.StdoutPipe()
	s.Require().NoError(err)
	s.Require().NoError(command.Start())
	s.T().Cleanup(func() {
		_ = command.Process.Kill()
		_ = command.Wait()
	})
	ready := make(chan bool, 1)
	go func() { ready <- bufio.NewScanner(stdout).Scan() }()
	select {
	case ok := <-ready:
		s.Require().True(ok, "private bus must print its address")
	case <-time.After(5 * time.Second):
		s.T().Fatal("private bus did not start")
	}
	return address
}

// The daemon starts this actual service via its private .service file, without a shell.
func TestDBusActivationProcess(t *testing.T) {
	report := os.Getenv("RUNIX_DBUS_REPORT")
	if report == "" {
		return
	}
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	err = conn.ExportMethodTable(map[string]interface{}{
		"Activate": func(data map[string]dbus.Variant) *dbus.Error {
			if len(data) != 0 {
				return dbus.NewError("org.runix.BadData", nil)
			}
			// #nosec G703 -- the private daemon supplies its test-owned marker path.
			if writeErr := os.WriteFile(report, []byte("activated"), 0o600); writeErr != nil {
				return dbus.MakeFailedError(writeErr)
			}
			return nil
		},
	}, "/org/runix/Auto", "org.freedesktop.Application")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = conn.RequestName("org.runix.Auto", dbus.NameFlagDoNotQueue); err != nil {
		t.Fatal(err)
	}
	<-conn.Context().Done()
}

func (s *DBusLaunchSuite) launcher(address string) *Launcher {
	launcher := NewLauncher([]string{
		"DBUS_SESSION_BUS_ADDRESS=" + address,
		"XDG_ACTIVATION_TOKEN=stale", "DESKTOP_STARTUP_ID=stale",
	})
	s.T().Cleanup(launcher.Close)
	return launcher
}

func (s *DBusLaunchSuite) service(address, name string, method interface{}) *dbus.Conn {
	conn, err := dbus.Connect(address)
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = conn.Close() })
	_, path, err := applicationBusTarget(name + ".desktop")
	s.Require().NoError(err)
	s.Require().NoError(conn.ExportMethodTable(map[string]interface{}{"Activate": method},
		path, "org.freedesktop.Application"))
	reply, err := conn.RequestName(name, dbus.NameFlagDoNotQueue)
	s.Require().NoError(err)
	s.Require().Equal(dbus.RequestNameReplyPrimaryOwner, reply)
	return conn
}

func (s *DBusLaunchSuite) TestLogicalSymlinkIdentityAndConnectionReuse() {
	address := s.privateBus()
	calls := make(chan dbus.Message, 2)
	s.service(address, "org.runix.Dash-app", func(message dbus.Message, data map[string]dbus.Variant) *dbus.Error {
		if len(data) != 0 {
			return dbus.NewError("org.runix.StaleTokens", nil)
		}
		calls <- message
		return nil
	})
	directory := s.T().TempDir()
	target := filepath.Join(directory, "different.desktop")
	s.Require().NoError(os.WriteFile(target, []byte(
		"[Desktop Entry]\nType=Application\nName=Fixture\nDBusActivatable=true\n"), 0o600))
	logical := filepath.Join(directory, "org.runix.Dash-app.desktop")
	s.Require().NoError(os.Symlink(target, logical))
	data, err := os.ReadFile(logical)
	s.Require().NoError(err)
	entry, err := Parse(data, Source{ID: filepath.Base(logical), Path: logical}, Environment{})
	s.Require().NoError(err)
	launcher := s.launcher(address)
	s.Require().NoError(launcher.Launch(context.Background(), entry))
	first := <-calls
	s.Equal("org.runix.Dash-app", first.Headers[dbus.FieldDestination].Value())
	s.Equal(dbus.ObjectPath("/org/runix/Dash_app"), first.Headers[dbus.FieldPath].Value())
	s.Equal("org.freedesktop.Application", first.Headers[dbus.FieldInterface].Value())
	s.Equal("Activate", first.Headers[dbus.FieldMember].Value())
	s.Equal(dbus.SignatureOf(map[string]dbus.Variant{}), first.Headers[dbus.FieldSignature].Value())
	s.Require().NoError(launcher.Launch(context.Background(), entry))
	second := <-calls
	s.Equal(first.Headers[dbus.FieldSender], second.Headers[dbus.FieldSender], "reuse owned connection")
	launcher.Close()
	s.ErrorContains(launcher.Launch(context.Background(), entry), "closed")
}

func (s *DBusLaunchSuite) TestBusStartsUnownedService() {
	address := s.privateBus()
	entry := Entry{Type: "Application", Source: Source{ID: "org.runix.Auto.desktop"}, DBusActivatable: true}
	s.Require().NoError(s.launcher(address).Launch(context.Background(), entry))
	marker := filepath.Join(filepath.Dir(strings.TrimPrefix(address, "unix:path=")), "activation")
	data, err := os.ReadFile(marker)
	s.Require().NoError(err)
	s.Equal("activated", string(data))
}

func (s *DBusLaunchSuite) TestErrorsAndInvalidIDsNeverRunExec() {
	address := s.privateBus()
	s.service(address, "org.runix.Failure", func(map[string]dbus.Variant) *dbus.Error {
		return dbus.NewError("org.runix.Refused", []interface{}{"fixture refused activation"})
	})
	entry, environment, report, _ := s.fixture()
	environment = append(environment, "DBUS_SESSION_BUS_ADDRESS="+address)
	launcher := NewLauncher(environment)
	defer launcher.Close()
	entry.DBusActivatable = true
	for _, id := range []string{"org.runix.Failure.desktop", "org.runix.Missing.desktop"} {
		s.Run(id, func() {
			entry.Source.ID = id
			err := launcher.Launch(context.Background(), entry)
			s.Require().Error(err)
			if id == "org.runix.Failure.desktop" {
				s.ErrorContains(err, "fixture refused activation")
			}
		})
	}
	_, err := os.Stat(report)
	s.True(os.IsNotExist(err), "observable Exec fallback must never run")
}

func (s *DBusLaunchSuite) TestInvalidIDsFailBeforeConnecting() {
	entry, environment, report, _ := s.fixture()
	launcher := NewLauncher(environment)
	defer launcher.Close()
	entry.DBusActivatable = true
	for _, id := range []string{
		"bad.desktop", "1org.runix.App.desktop", "org..App.desktop",
		"org.runix.1App.desktop", "org.runix.Äpp.desktop", "org.runix.App",
		strings.Repeat("a", 256) + ".App.desktop",
	} {
		s.Run(id, func() {
			entry.Source.ID = id
			s.ErrorContains(launcher.Launch(context.Background(), entry), "invalid D-Bus application desktop ID")
		})
	}
	_, err := os.Stat(report)
	s.True(os.IsNotExist(err), "invalid IDs must not execute fallback")
}

func (s *DBusLaunchSuite) TestTimeoutCancellationAndShutdown() {
	for _, mode := range []string{"deadline", "cancel", "shutdown"} {
		s.Run(mode, func() {
			address := s.privateBus()
			started := make(chan struct{}, 1)
			release := make(chan struct{})
			defer close(release)
			s.service(address, "org.runix.Wait", func(map[string]dbus.Variant) *dbus.Error {
				started <- struct{}{}
				<-release
				return nil
			})
			entry, environment, report, _ := s.fixture()
			environment = append(environment, "DBUS_SESSION_BUS_ADDRESS="+address)
			launcher := NewLauncher(environment)
			defer launcher.Close()
			entry.Source.ID, entry.DBusActivatable = "org.runix.Wait.desktop", true
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			result := make(chan error, 1)
			go func() { result <- launcher.Launch(ctx, entry) }()
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				s.T().Fatal("Activate was not received")
			}
			switch mode {
			case "cancel":
				cancel()
			case "shutdown":
				launcher.Close()
			}
			select {
			case err := <-result:
				s.Require().Error(err)
				if mode == "deadline" {
					s.ErrorIs(err, context.DeadlineExceeded)
				} else {
					s.ErrorIs(err, context.Canceled)
				}
			case <-time.After(HandoffTimeout + 2*time.Second):
				s.T().Fatal("handoff did not stop")
			}
			_, err := os.Stat(report)
			s.True(os.IsNotExist(err), "ambiguous calls must not execute fallback")
		})
	}
}

func (s *DBusLaunchSuite) TestUnavailableBusDoesNotBlockExec() {
	entry, environment, report, release := s.fixture()
	environment = append(environment, "DBUS_SESSION_BUS_ADDRESS=unix:path="+filepath.Join(s.T().TempDir(), "absent"))
	launcher := NewLauncher(environment)
	defer launcher.Close()
	entry.DBusActivatable, entry.Source.ID = true, "org.runix.Absent.desktop"
	s.ErrorContains(launcher.Launch(context.Background(), entry), "session D-Bus")
	entry.DBusActivatable = false
	s.Require().NoError(launcher.Launch(context.Background(), entry))
	s.finish(s.capture(report), release)
}

func (s *DBusLaunchSuite) TestAuthenticationCanBeCanceled() {
	path := filepath.Join(s.T().TempDir(), "silent")
	var config net.ListenConfig
	listener, err := config.Listen(s.T().Context(), "unix", path)
	s.Require().NoError(err)
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		socket, acceptErr := listener.Accept()
		if acceptErr == nil {
			accepted <- socket
		}
	}()
	launcher := s.launcher("unix:path=" + path)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		result <- launcher.Launch(ctx, Entry{
			Type: "Application", DBusActivatable: true,
			Source: Source{ID: "org.runix.Silent.desktop"},
		})
	}()
	select {
	case socket := <-accepted:
		defer socket.Close()
	case <-time.After(3 * time.Second):
		s.T().Fatal("connection was not accepted")
	}
	cancel()
	select {
	case err = <-result:
		s.ErrorIs(err, context.Canceled)
	case <-time.After(2 * time.Second):
		s.T().Fatal("authentication did not cancel")
	}
}

func (s *DBusLaunchSuite) TestDisconnectedBusStillAllowsExec() {
	address := s.privateBus()
	s.service(address, "org.runix.Disconnect", func(map[string]dbus.Variant) *dbus.Error { return nil })
	entry, environment, report, release := s.fixture()
	environment = append(environment, "DBUS_SESSION_BUS_ADDRESS=unix:path=/missing-runix-bus;"+address)
	launcher := NewLauncher(environment)
	defer launcher.Close()
	entry.Source.ID, entry.DBusActivatable = "org.runix.Disconnect.desktop", true
	s.Require().NoError(launcher.Launch(context.Background(), entry))
	s.Require().NoError(launcher.conn.Close())
	entry.DBusActivatable = false
	s.Require().NoError(launcher.Launch(context.Background(), entry))
	s.finish(s.capture(report), release)
}
