package desktop

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// launchExec starts a parsed non-D-Bus entry using an explicit session environment
// (normally os.Environ()). Success means Start succeeded, not application readiness.
// The caller owns a bounded handoff context and catalog generation validation.
// No child is joined at shutdown; a background waiter reaps each direct child.
func launchExec(ctx context.Context, entry Entry, environment []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if entry.DBusActivatable {
		return fmt.Errorf("D-Bus applications must use Launcher.Launch")
	}
	argv, err := CompileExec(entry)
	if err != nil {
		return err
	}
	childEnvironment := make([]string, 0, len(environment))
	path := ""
	for _, variable := range environment {
		key, value, _ := strings.Cut(variable, "=")
		switch key {
		case "XDG_ACTIVATION_TOKEN", "DESKTOP_STARTUP_ID":
			continue
		case "PATH":
			path = value
		}
		childEnvironment = append(childEnvironment, variable)
	}
	program, err := resolveExecutable(argv[0], path)
	if err != nil {
		return err
	}
	argv[0] = program
	if entry.Terminal {
		helper, helperErr := resolveExecutable("xdg-terminal-exec", path)
		if helperErr != nil {
			return fmt.Errorf("install xdg-terminal-exec and add it to PATH to launch terminal applications: %w",
				helperErr)
		}
		arguments := []string{helper}
		if entry.Path != "" {
			arguments = append(arguments, "--dir="+entry.Path)
		}
		arguments = append(arguments, "--")
		argv = append(arguments, argv...)
		program = helper
	}
	// Do not use CommandContext: cancellation after Start must not kill the app.
	// Nil streams connect to /dev/null, never runix pipes, sockets, or buffers.
	command := &exec.Cmd{
		Path:        program,
		Args:        argv,
		Env:         childEnvironment,
		Dir:         entry.Path,
		SysProcAttr: &syscall.SysProcAttr{Setsid: true},
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = command.Start(); err != nil {
		return fmt.Errorf("cannot start application: %w", err)
	}
	go func() { _ = command.Wait() }()
	return nil
}

func resolveExecutable(value, path string) (string, error) {
	if filepath.IsAbs(value) {
		if executable(value) {
			return value, nil
		}
	} else if !strings.ContainsRune(value, filepath.Separator) {
		for _, directory := range filepath.SplitList(path) {
			candidate := filepath.Join(directory, value)
			if filepath.IsAbs(directory) && executable(candidate) {
				return candidate, nil
			}
		}
	}
	return "", fmt.Errorf("cannot find executable %q in absolute PATH directories", value)
}
