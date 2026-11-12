//go:build !(darwin || linux)

package platform

import "os"

// fileLocks reports whether this platform has an advisory file lock: here it
// has none, so nothing is reaped by lock.
const fileLocks = false

func tryLock(*os.File) bool {
	return false
}
