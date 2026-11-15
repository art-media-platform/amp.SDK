//go:build !(darwin || linux)

package platform

import (
	"errors"
	"os"
)

// fileLocks reports whether this platform has an advisory file lock: here it
// has none, so nothing is reaped by lock.
const fileLocks = false

var errNoFileLock = errors.New("platform: no file lock on this platform")

func tryLock(*os.File) error {
	return errNoFileLock
}

func openDir(string) (*os.File, error) {
	return nil, errNoFileLock
}

func openRegular(string) (*os.File, error) {
	return nil, errNoFileLock
}
