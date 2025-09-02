package fs

import (
	"fmt"
	"os"
)

func CreateDir(path string, perm os.FileMode) (existed bool, err error) {
	if existed, err = ExistsDirEx(path); err != nil {
		return existed, fmt.Errorf("getting info about dir (%s) ended with error: %s", path, err)
	}

	if !existed {
		if err = os.MkdirAll(path, perm); err != nil {
			return existed, fmt.Errorf("creating dir (%s) ended with error: %s", path, err)
		}
	}

	return existed, err
}
