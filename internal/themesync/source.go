package themesync

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"runix/internal/theme"
)

// ArchiveSHA256 pins the full official codeload archive, not an extracted palette subset.
const ArchiveSHA256 = "e33b7258087790171f70a046d009365c85d67a199e63240ea270e7ba182d3e79"

// ReadCheckout accepts a complete, clean checkout at the requested full revision.
func ReadCheckout(ctx context.Context, root, revision string) (*Snapshot, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	git := func(args ...string) (string, error) {
		// #nosec G204 -- Git operations are fixed below; the local directory is a separate -C argument, not shell code.
		command := exec.CommandContext(ctx, "git", append([]string{"-C", absolute}, args...)...)
		output, commandErr := command.CombinedOutput()
		if commandErr != nil {
			return "", fmt.Errorf("read checkout: %w: %s", commandErr, strings.TrimSpace(string(output)))
		}
		return strings.TrimSpace(string(output)), nil
	}
	head, err := git("rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	if head != revision {
		return nil, fmt.Errorf("checkout revision: got %s; expected %s", head, revision)
	}
	top, err := git("rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, err
	}
	if top != resolved {
		return nil, errors.New("checkout: use the full repository root")
	}
	// Sparse checkouts can omit files even while Git reports a clean worktree.
	sparse, err := git("config", "--get", "--default", "false", "core.sparseCheckout")
	if err != nil {
		return nil, err
	}
	if sparse != "false" {
		return nil, errors.New("checkout: use a complete checkout, not a sparse checkout")
	}
	status, err := git("status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	if status != "" {
		return nil, errors.New("checkout: use a clean upstream checkout; local changes are not a pinned source")
	}
	return ReadSource(absolute, revision)
}

// ReadArchive verifies the complete pinned official archive before reading any themes.
// This alternate source works where Git checkout creation is unavailable.
func ReadArchive(name string) (*Snapshot, error) {
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return nil, err
	}
	if hex.EncodeToString(hash.Sum(nil)) != ArchiveSHA256 {
		return nil, errors.New("archive: SHA-256 does not match the complete pinned upstream archive")
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	compressed, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	defer compressed.Close()
	archive := tar.NewReader(compressed)
	prefix := "ray-so-" + theme.UpstreamRevision + "/"
	collection := prefix + theme.UpstreamCollection + "/"
	sources := make(map[string][]byte)
	var license []byte
	for {
		header, nextErr := archive.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return nil, nextErr
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		if header.Name == prefix+"LICENSE" || strings.HasPrefix(header.Name, collection) {
			// The verified archive has small JSON files and a small root license.
			if header.Size > 1<<20 {
				return nil, fmt.Errorf("archive: source file is too large: %s", header.Name)
			}
			data, readErr := io.ReadAll(io.LimitReader(archive, 1<<20))
			if readErr != nil {
				return nil, readErr
			}
			if header.Name == prefix+"LICENSE" {
				license = data
			} else {
				sources[strings.TrimPrefix(header.Name, collection)] = data
			}
		}
	}
	return New(sources, license, theme.UpstreamRevision)
}
