//go:build darwin || linux

package platform

import (
	"os"
	"syscall"
)

// fileLocks reports whether this platform has the advisory file lock below.
const fileLocks = true

// tryLock takes f's exclusive lock without waiting: nil, errLockHeld, or the
// error that kept the lock from being taken.  The lock (flock) belongs to f's
// open file description: another open of the same file or directory, in this
// process or another, is refused until f closes, and the kernel releases it
// when its holder ends, however it ends.
func tryLock(f *os.File) error {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err == syscall.EWOULDBLOCK {
		return errLockHeld
	}
	return err
}

// openDir opens the directory at pathname for its lock, refusing a symlink.
func openDir(pathname string) (*os.File, error) {
	return os.OpenFile(pathname, os.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
}
