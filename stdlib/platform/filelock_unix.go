//go:build darwin || linux

package platform

import (
	"os"
	"syscall"
)

// fileLocks reports whether this platform has the advisory file lock below.
const fileLocks = true

// tryLock takes f's exclusive lock without waiting.  The lock (flock) belongs
// to f's open file description: another open of the same file, in this
// process or another, is refused until f closes, and the kernel releases it
// when its holder ends, however it ends.
func tryLock(f *os.File) bool {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) == nil
}
