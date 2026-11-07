package themesync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"

	"runix/internal/theme"
)

// Command inventories by default; replacing the bundle requires --write.
type Command struct {
	Checkout    string `help:"Read a complete clean local upstream checkout." placeholder:"PATH"`
	Archive     string `help:"Read the full pinned official archive; verify its SHA-256." placeholder:"PATH"`
	Revision    string `default:"1298a944166d53d59e6211609366cbfdd08da623" help:"Expected full checkout commit hash."`
	Destination string `default:"data/themes/bundled" help:"Bundle directory to compare or replace."`
	Write       bool   `help:"Replace bundled data after validation and the inventory report. Default: report only."`
}

// Run reports all changes before any replacement, with no runtime download behavior.
func (command *Command) Run(ctx context.Context, output io.Writer) error {
	if (command.Checkout == "") == (command.Archive == "") {
		return errors.New("use exactly one source: --checkout PATH or --archive PATH")
	}
	var snapshot *Snapshot
	var err error
	if command.Archive != "" {
		if command.Revision != theme.UpstreamRevision {
			return errors.New("--archive supports only the pinned revision; use --checkout for another revision")
		}
		if err = rejectOverlap(command.Archive, command.Destination); err != nil {
			return err
		}
		snapshot, err = ReadArchive(command.Archive)
	} else {
		if err = rejectOverlap(command.Checkout, command.Destination); err != nil {
			return err
		}
		snapshot, err = ReadCheckout(ctx, command.Checkout, command.Revision)
	}
	if err != nil {
		return err
	}
	previous, err := ReadBundle(command.Destination)
	if err != nil {
		return err
	}
	report := snapshot.Inventory(previous)
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	if err = encoder.Encode(report); err != nil {
		return fmt.Errorf("write inventory report: %w", err)
	}
	if command.Write {
		return snapshot.Write(command.Destination)
	}
	return nil
}

func rejectOverlap(source, destination string) error {
	paths := []*string{&source, &destination}
	for _, name := range paths {
		absolute, err := filepath.Abs(*name)
		if err != nil {
			return err
		}
		// Resolve existing ancestors too, including a not-yet-created bundle path.
		resolved, err := resolvePath(absolute)
		if err != nil {
			return err
		}
		*name = resolved
	}
	for _, pair := range [][2]string{{source, destination}, {destination, source}} {
		relative, err := filepath.Rel(pair[0], pair[1])
		if err != nil {
			return err
		}
		if relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return errors.New("source and destination must not overlap")
		}
	}
	return nil
}

func resolvePath(name string) (string, error) {
	resolved, err := filepath.EvalSymlinks(name)
	if err == nil {
		return resolved, nil
	}
	// Missing descendants are allowed; permission and other failures are not.
	if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	parent := filepath.Dir(name)
	if parent == name {
		return "", err
	}
	resolved, err = resolvePath(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolved, filepath.Base(name)), nil
}
