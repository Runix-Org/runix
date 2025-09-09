package desktop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/Runix-Org/runix/platform/fs"
	"github.com/Runix-Org/runix/platform/wayland"
)

// A sufficiently unique ID
func generateStartupID() string {
	return fmt.Sprintf("runix-%d-%d", os.Getpid(), time.Now().UnixNano())
}

type DesktopEntryLauncher struct {
	terminalPath string
}

func NewDesktopEntryLauncher(terminalPath string) *DesktopEntryLauncher {
	return &DesktopEntryLauncher{
		terminalPath: terminalPath,
	}
}

func (l *DesktopEntryLauncher) fillParamCode(code rune, urls []string, files []string) []string {
	switch code {
	case 'u':
		if len(urls) > 0 {
			return []string{urls[0]}
		}
	case 'U':
		return urls
	case 'f':
		if len(files) > 0 {
			return []string{files[0]}
		}
	case 'F':
		return files
	}

	return []string{}
}

func (l *DesktopEntryLauncher) buildLaunchArgs(exec string, urls []string, files []string) ([]string, error) {
	arg := []rune{}
	res := []string{}
	fieldCodeInd := -1
	inEscape := false
	inSingleQuote := false
	inDoubleQuote := false

	for ind, c := range strings.Replace(exec, "\\\\", "\\", -1) {
		if inEscape {
			inEscape = false
			arg = append(arg, c)
			continue
		}

		switch c {
		case 'u', 'U', 'f', 'F':
			if fieldCodeInd == ind {
				if len(arg) > 0 {
					arg = arg[:len(arg)-1]
					res = append(res, l.fillParamCode(c, urls, files)...)
				}
				continue
			}
		case '"':
			if inDoubleQuote {
				inDoubleQuote = false
				res = append(res, string(arg))
				arg = arg[:0]
				continue
			}
			if !inSingleQuote {
				inDoubleQuote = true
				continue
			}

		case '\'':
			if inSingleQuote {
				inSingleQuote = false
				res = append(res, string(arg))
				arg = arg[:0]
				continue
			}
			if !inDoubleQuote {
				inSingleQuote = true
				continue
			}

		case '\\':
			if inDoubleQuote {
				inEscape = true
				continue
			}

		case '%':
			if !(inDoubleQuote || inSingleQuote) {
				fieldCodeInd = ind + 1
			}

		case ' ':
			if !(inDoubleQuote || inSingleQuote) {
				if len(arg) != 0 {
					res = append(res, string(arg))
					arg = arg[:0]
				}
				continue
			}
		}

		arg = append(arg, c)
	}

	if len(arg) != 0 {
		if !(inEscape || inDoubleQuote || inSingleQuote) {
			res = append(res, string(arg))
		} else {
			return nil, fmt.Errorf("exec value contains an unbalanced number of quote characters: %s", exec)
		}
	}

	if len(res) == 0 {
		return nil, errors.New("empty exec string")
	}

	return res, nil
}

func (l *DesktopEntryLauncher) checkTryExec(tryExec string) error {
	if tryExec == "" {
		return nil
	}

	if strings.ContainsRune(tryExec, '/') {
		if !fs.ExistsFile(tryExec) {
			return fmt.Errorf("TryExec not found or not a file: %s", tryExec)
		}

		return nil
	}

	if _, err := exec.LookPath(tryExec); err != nil {
		return fmt.Errorf("TryExec not found in PATH: %s", tryExec)
	}
	return nil
}

func (l *DesktopEntryLauncher) LaunchFull(de *DesktopEntry, urls []string, files []string) error {
	if err := l.checkTryExec(de.TryExec); err != nil {
		return err
	}

	args, err := l.buildLaunchArgs(de.Exec, urls, files)
	if err != nil {
		return err
	}

	if de.Terminal {
		if len(l.terminalPath) == 0 {
			return fmt.Errorf("terminal value is true, but terminal path is empty")
		}
		args = append([]string{l.terminalPath, "-e"}, args...)
	}

	name := args[0]
	var cmd *exec.Cmd
	if len(args) == 1 {
		cmd = exec.Command(name)
	} else {
		cmd = exec.Command(name, args[1:]...)
	}

	env := os.Environ()
	env = append(env, "BAMF_DESKTOP_FILE_HINT="+de.FilePath)
	if de.StartupNotify {
		env = append(env, "DESKTOP_STARTUP_ID="+generateStartupID())
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if token := wayland.GenerateActivationToken(ctx); token != "" {
		env = append(env, "XDG_ACTIVATION_TOKEN="+token)
	}

	cmd.Env = env
	cmd.Dir = de.Path
	cmd.SysProcAttr = &syscall.SysProcAttr{
		// If the parent process does not exit correctly, then all child processes will also be killed
		// This code cancel this behavior
		Setpgid: true,
		// Detach fd 0 from controlling terminal
		Noctty: true,
	}

	if err = cmd.Start(); err != nil {
		return err
	}

	go func() {
		_ = cmd.Wait()
	}()

	return nil
}

func (l *DesktopEntryLauncher) Launch(de *DesktopEntry) error {
	return l.LaunchFull(de, []string{}, []string{})
}

func (l *DesktopEntryLauncher) LaunchWithURLs(de *DesktopEntry, urls ...string) error {
	return l.LaunchFull(de, urls, []string{})
}

func (l *DesktopEntryLauncher) LaunchWithFiles(de *DesktopEntry, files ...string) error {
	return l.LaunchFull(de, []string{}, files)
}
