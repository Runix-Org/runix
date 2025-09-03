//go:build linux

package fs

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// ResolveSymlink resolves a path to an absolute canonical path on Linux,
// expanding symlinks step by step, and returns the final absolute path
// together with the final os.Stat result.
//
// Contracts and behavior (Linux):
//   - Input can be absolute or relative. Empty string is treated as ".".
//   - Returns an absolute path to an existing final object (file or directory)
//     and its FileInfo. If anything is missing or inaccessible along the way,
//     returns an error.
//   - Symlink targets are processed component-by-component, without cleaning
//     the target string in advance. Dots (".", "..") in targets are handled
//     in the same order as the kernel does.
//   - maxHops > 0: limits number of symlink expansions, returns ELOOP on excess.
//     maxHops <= 0: unlimited hops; cycles are still detected and reported as ELOOP.
//   - This function is not race-free against concurrent filesystem changes.
//     It is safe for common use but not hardened against deliberate TOCTOU attacks.
func ResolveSymlink(path string, maxHops int) (string, fs.FileInfo, error) {
	if path == "" {
		path = "."
	}
	if !filepath.IsAbs(path) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", nil, fmt.Errorf("getwd: %w", err)
		}
		path = filepath.Join(cwd, path)
	}
	// Clean only the input path (never clean symlink targets).
	abs := filepath.Clean(path)

	// We'll track the current absolute path as a string (currentPath),
	// plus a stack of previous lengths to allow efficient ".." handling.
	currentPath := "/"
	var lens []int

	// Remaining components to process. For an absolute path, first element is "".
	rest := splitComponents(abs)
	i := 1 // skip leading "" for absolute paths

	hops := 0

	// For cycle detection: set of visited symlink (dev, ino).
	type key struct {
		dev uint64
		ino uint64
	}
	visited := make(map[key]struct{})

	for i < len(rest) {
		comp := rest[i]
		i++

		if comp == "" || comp == "." {
			continue
		}
		if comp == ".." {
			// Go one level up, but not above root.
			if len(lens) > 0 {
				prevLen := lens[len(lens)-1]
				lens = lens[:len(lens)-1]
				currentPath = currentPath[:prevLen]
			} else {
				currentPath = "/"
			}
			continue
		}

		candidate := joinCandidate(currentPath, comp)

		fi, err := os.Lstat(candidate)
		if err != nil {
			return "", nil, fmt.Errorf("lstat %q: %w", candidate, err)
		}

		// Symlink?
		if fi.Mode()&os.ModeSymlink != 0 {
			// hop limit
			if maxHops > 0 {
				hops++
				if hops > maxHops {
					return "", nil, fmt.Errorf("too many symlink hops (%d) at %q: %w", maxHops, candidate, syscall.ELOOP)
				}
			}

			// cycle detection via (dev, ino) of the symlink itself
			if st, ok := fi.Sys().(*syscall.Stat_t); ok {
				k := key{dev: uint64(st.Dev), ino: uint64(st.Ino)}
				if _, seen := visited[k]; seen {
					return "", nil, fmt.Errorf("symlink cycle detected at %q: %w", candidate, syscall.ELOOP)
				}
				visited[k] = struct{}{}
			}

			target, err := os.Readlink(candidate)
			if err != nil {
				return "", nil, fmt.Errorf("readlink %q: %w", candidate, err)
			}

			// Never clean the target. Just split and continue resolving.
			tcomps := splitComponents(target)
			if filepath.IsAbs(target) {
				// Absolute target: reset to root.
				currentPath = "/"
				lens = lens[:0]
				// Replace the rest with target's components + remaining tail.
				rest = append(tcomps, rest[i:]...)
				i = 1 // skip leading "" for absolute
			} else {
				// Relative target: relative to directory containing candidate.
				rest = append(tcomps, rest[i:]...)
				i = 0 // start from beginning of target components
			}
			continue
		}

		// Regular path component, commit it.
		prevLen := len(currentPath)
		if currentPath == "/" {
			currentPath = "/" + comp
		} else {
			currentPath = currentPath + "/" + comp
		}
		lens = append(lens, prevLen)
	}

	// Final Stat to verify that the final object exists and is accessible.
	finalPath := currentPath
	fi, err := os.Stat(finalPath)
	if err != nil {
		return "", nil, fmt.Errorf("stat %q: %w", finalPath, err)
	}

	return finalPath, fi, nil
}

func splitComponents(p string) []string {
	// Keep components verbatim; do not clean. This preserves "." and ".."
	// in symlink targets, and empty components for "//" which kernel ignores.
	// strings.Split("", "/") returns [""], which is fine.
	return strings.Split(p, "/")
}

func joinCandidate(base string, comp string) string {
	if base == "/" {
		return "/" + comp
	}
	return base + "/" + comp
}
