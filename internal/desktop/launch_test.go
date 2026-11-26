package desktop

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type launchReport struct {
	Args         []string
	Directory    string
	Token        string
	StartupID    string
	Session      string
	PID          int
	ProcessGroup int
}

// This subprocess is the actual application (or terminal CLI capture), not a shell.
func init() {
	if os.Getenv("RUNIX_LAUNCH_HELPER") == "parent" {
		binary, err := os.Executable()
		if err != nil {
			os.Exit(5)
		}
		environment := os.Environ()
		for index, variable := range environment {
			if strings.HasPrefix(variable, "RUNIX_LAUNCH_HELPER=") {
				environment[index] = "RUNIX_LAUNCH_HELPER=1"
			}
		}
		entry := Entry{Type: "Application", Exec: ExecValue{Decoded: `"` + binary + `" child`}}
		if launchExec(context.Background(), entry, environment) != nil {
			os.Exit(6)
		}
		// #nosec G703 -- private fixture marker supplied by the parent test.
		if os.WriteFile(os.Getenv("RUNIX_REPORT")+".parent", []byte(strconv.Itoa(os.Getpid())), 0o600) != nil {
			os.Exit(7)
		}
		os.Exit(0)
	}
	if os.Getenv("RUNIX_LAUNCH_HELPER") != "1" {
		return
	}
	directory, err := os.Getwd()
	if err != nil {
		os.Exit(2)
	}
	group, err := syscall.Getpgid(0)
	if err != nil {
		os.Exit(3)
	}
	report := launchReport{
		Args: os.Args[1:], Directory: directory, PID: os.Getpid(), ProcessGroup: group,
		Token: os.Getenv("XDG_ACTIVATION_TOKEN"), StartupID: os.Getenv("DESKTOP_STARTUP_ID"),
		Session: os.Getenv("RUNIX_SESSION_VALUE"),
	}
	data, err := json.Marshal(report)
	// The parent supplies an owner-private fixture path, not external input.
	// #nosec G703 -- this subprocess only writes the parent test's report fixture.
	if err != nil || os.WriteFile(os.Getenv("RUNIX_REPORT"), data, 0o600) != nil {
		os.Exit(4)
	}
	for {
		// #nosec G703 -- this is the parent test's private release marker.
		if _, err = os.Stat(os.Getenv("RUNIX_RELEASE")); err == nil {
			os.Exit(0)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

type launchFixtures struct{ suite.Suite }

type LaunchSuite struct{ launchFixtures }

func TestLaunchSuite(t *testing.T) { suite.Run(t, new(LaunchSuite)) }

func (s *launchFixtures) fixture() (Entry, []string, string, string) {
	directory := s.T().TempDir()
	binary, err := os.Executable()
	s.Require().NoError(err)
	// A private executable link lets terminal and direct cases use the same real helper.
	s.Require().NoError(os.Symlink(binary, filepath.Join(directory, "app")))
	s.Require().NoError(os.Symlink(binary, filepath.Join(directory, "xdg-terminal-exec")))
	report := filepath.Join(directory, "report")
	release := filepath.Join(directory, "release")
	entry := Entry{
		Type: "Application", Path: directory,
		Exec: ExecValue{Decoded: `app -test.run=TestDesktopLaunchHelper -- "two words" "" %c`},
		Name: Text{Selected: "Имя %i"},
	}
	environment := []string{
		"PATH=" + directory, "RUNIX_LAUNCH_HELPER=1", "RUNIX_REPORT=" + report,
		"RUNIX_RELEASE=" + release, "RUNIX_SESSION_VALUE=session kept",
		"XDG_ACTIVATION_TOKEN=stale", "DESKTOP_STARTUP_ID=stale",
	}
	s.T().Cleanup(func() { _ = os.WriteFile(release, nil, 0o600) })
	return entry, environment, report, release
}

func (s *launchFixtures) capture(path string) launchReport {
	var report launchReport
	s.Require().Eventually(func() bool {
		data, err := os.ReadFile(path)
		return err == nil && json.Unmarshal(data, &report) == nil
	}, 10*time.Second, 10*time.Millisecond)
	return report
}

func (s *launchFixtures) finish(report launchReport, release string) {
	s.Require().NoError(os.WriteFile(release, nil, 0o600))
	s.Require().Eventually(func() bool {
		_, err := os.Stat("/proc/" + strconv.Itoa(report.PID))
		return os.IsNotExist(err)
	}, 10*time.Second, 10*time.Millisecond, "child must exit and be reaped")
}

func (s *LaunchSuite) TestDirectHandoffSurvivesCancellationAndReaps() {
	entry, environment, path, release := s.fixture()
	ctx, cancel := context.WithCancel(context.Background())
	s.Require().NoError(launchExec(ctx, entry, environment))
	cancel()
	report := s.capture(path)
	s.Equal([]string{"-test.run=TestDesktopLaunchHelper", "--", "two words", "", "Имя %i"}, report.Args)
	s.Equal(entry.Path, report.Directory)
	s.Equal("session kept", report.Session)
	s.Empty(report.Token)
	s.Empty(report.StartupID)
	s.Equal(report.PID, report.ProcessGroup)
	s.NoError(syscall.Kill(report.PID, 0))
	s.finish(report, release)
}

func (s *LaunchSuite) TestTerminalCLIAndWorkingDirectory() {
	entry, environment, path, release := s.fixture()
	entry.Terminal = true
	s.Require().NoError(launchExec(context.Background(), entry, environment))
	report := s.capture(path)
	s.Equal([]string{
		"--dir=" + entry.Path, "--", filepath.Join(entry.Path, "app"),
		"-test.run=TestDesktopLaunchHelper", "--", "two words", "", "Имя %i",
	}, report.Args)
	s.Equal(entry.Path, report.Directory)
	s.finish(report, release)
}

func (s *LaunchSuite) TestStartFailuresAndDispatchBoundary() {
	entry, environment, path, _ := s.fixture()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.ErrorIs(launchExec(ctx, entry, environment), context.Canceled)
	entry.DBusActivatable = true
	s.ErrorContains(launchExec(context.Background(), entry, environment), "D-Bus")
	entry.DBusActivatable = false
	entry.Path = filepath.Join(entry.Path, "missing")
	s.ErrorContains(launchExec(context.Background(), entry, environment), "cannot start")
	entry.Path = filepath.Dir(path)
	entry.Exec.Decoded = "missing"
	s.ErrorContains(launchExec(context.Background(), entry, environment), "cannot find executable")
	entry.Exec.Decoded = "app"
	entry.Terminal = true
	s.Require().NoError(os.Remove(filepath.Join(entry.Path, "xdg-terminal-exec")))
	s.ErrorContains(launchExec(context.Background(), entry, environment), "install xdg-terminal-exec")
	_, err := os.Stat(path)
	s.True(os.IsNotExist(err))
}

func (s *LaunchSuite) TestRelativePATHNeverFindsCurrentDirectory() {
	entry, environment, _, _ := s.fixture()
	for index, variable := range environment {
		if strings.HasPrefix(variable, "PATH=") {
			environment[index] = "PATH=:.:relative"
		}
	}
	s.ErrorContains(launchExec(context.Background(), entry, environment), "cannot find executable")
	entry.Exec.Decoded = "./app"
	s.ErrorContains(launchExec(context.Background(), entry, environment), "cannot find executable")
}

func (s *LaunchSuite) TestApplicationSurvivesLauncherExit() {
	entry, environment, path, release := s.fixture()
	for index, variable := range environment {
		if strings.HasPrefix(variable, "RUNIX_LAUNCH_HELPER=") {
			environment[index] = "RUNIX_LAUNCH_HELPER=parent"
		}
	}
	s.Require().NoError(launchExec(context.Background(), entry, environment))
	report := s.capture(path)
	var parentPID string
	s.Require().Eventually(func() bool {
		data, err := os.ReadFile(path + ".parent")
		parentPID = string(data)
		return err == nil && parentPID != ""
	}, 10*time.Second, 10*time.Millisecond)
	s.Require().Eventually(func() bool {
		_, err := os.Stat("/proc/" + parentPID)
		return os.IsNotExist(err)
	}, 10*time.Second, 10*time.Millisecond)
	s.NoError(syscall.Kill(report.PID, 0), "application must outlive its launcher")
	s.Require().NoError(os.WriteFile(release, nil, 0o600))
}
