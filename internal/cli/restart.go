package cli

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"syscall"

	"runix/internal/ui/product"
	"runix/internal/uiscale"
)

// restartPlan is prepared on Apply, but executes only after the product's complete teardown.
// Exec keeps the PID and inherited stdio; the election lock is released by close-on-exec.
type restartPlan struct {
	command   *Command
	endpoint  string
	root      string
	rootError error
	path      string
	args      []string
	requested bool
}

func newRestartPlan(command *Command, endpoint string) *restartPlan {
	root, err := os.UserConfigDir()
	return &restartPlan{command: command, endpoint: endpoint, root: root, rootError: err}
}

func (p *restartPlan) scaleOptions() product.ScaleOptions {
	options := product.ScaleOptions{Percent: 100, Restart: p.prepare}
	options.LoadError = p.rootError
	if options.LoadError == nil {
		percent, saved, err := uiscale.Load(p.root)
		options.LoadError = err
		if err == nil && saved {
			options.LoadError = os.Setenv("QT_SCALE_FACTOR", strconv.FormatFloat(float64(percent)/100, 'f', -1, 64))
		}
	}
	// Preserve existing QT_SCALE_FACTOR behavior until the user saves a scale choice.
	if factor, err := strconv.ParseFloat(os.Getenv("QT_SCALE_FACTOR"), 64); err == nil &&
		factor > 0 && !math.IsInf(factor, 0) {
		options.Percent = factor * 100
	}
	return options
}

func (p *restartPlan) prepare(percent int) error {
	if p.rootError != nil {
		return p.rootError
	}
	path, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate runix for restart: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("inspect runix for restart: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("runix is no longer executable at %s", path)
	}
	args := []string{path, "--activation-socket", p.endpoint}
	if p.command.DisableDesktopIntegration {
		args = append(args, "--disable-desktop-integration")
	}
	args = p.command.appendDebugRestartArgs(args)
	args = append(args, "cmd", "full", "--page", "settings", "--section", "appearance")
	if err := uiscale.Save(p.root, percent); err != nil {
		return fmt.Errorf("save interface scale: %w", err)
	}
	p.path, p.args, p.requested = path, args, true
	return nil
}

func (p *restartPlan) execute() error {
	// #nosec G204 -- Restart the current executable with explicit, parser-validated product options.
	if err := syscall.Exec(p.path, p.args, os.Environ()); err != nil {
		return fmt.Errorf("restart runix: %w; start runix again to use the saved scale", err)
	}
	return nil
}
