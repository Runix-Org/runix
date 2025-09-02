package fs

import (
	"os"
)

// Follows symlinks; broken symlink == error
func ExistsEx(path string) (bool, error) {
	_, err := os.Stat(ExpandUser(path))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Follows symlinks; broken symlink == false
func Exists(path string) bool {
	ok, err := ExistsEx(path)
	return err == nil && ok
}

// Follows symlinks; broken symlink == error
func ExistsDirEx(path string) (bool, error) {
	fi, err := os.Stat(ExpandUser(path))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return fi.IsDir(), nil
}

// Follows symlinks; broken symlink == false
func ExistsDir(path string) bool {
	ok, err := ExistsDirEx(path)
	return err == nil && ok
}

// Follows symlinks; broken symlink == error
func ExistsFileEx(path string) (bool, error) {
	fi, err := os.Stat(ExpandUser(path))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return fi.Mode().IsRegular(), nil
}

// Follows symlinks; broken symlink == false
func ExistsFile(path string) bool {
	ok, err := ExistsFileEx(path)
	return err == nil && ok
}

// Do not follow symlink
func ExistsSymlinkEx(path string) (bool, error) {
	fi, err := os.Lstat(ExpandUser(path))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return fi.Mode()&os.ModeSymlink != 0, nil
}

// Do not follow symlink
func ExistsSymlink(path string) bool {
	ok, err := ExistsSymlinkEx(path)
	return err == nil && ok
}
