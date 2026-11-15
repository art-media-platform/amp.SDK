package platform

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// processDirAttempts bounds CreateProcessDir's retries: one is lost only when a
// concurrent ReapProcessDirs takes a directory between its creation and its
// lock.
const processDirAttempts = 4

// errLockHeld is tryLock's refusal: another open file description holds the lock.
var errLockHeld = errors.New("platform: lock held")

var (
	heldDirLocksMu sync.Mutex
	heldDirLocks   []*os.File // each process directory, open and locked for the process's life
)

// CreateProcessDir makes a directory under root named prefix+<random> and holds
// the directory's own lock for the rest of the process's life.  The kernel
// releases that lock when the process ends, however it ends, so ReapProcessDirs
// tells an ended process's directory from a live one's by the lock alone, with
// no age, no process ID and no file inside the directory.  On a platform with
// no file lock it returns "" and no error: the caller keeps its files where it
// otherwise would.
func CreateProcessDir(root, prefix string) (string, error) {
	if !fileLocks {
		return "", nil
	}
	for range processDirAttempts {
		dir, err := os.MkdirTemp(root, prefix+"*")
		if err != nil {
			return "", fmt.Errorf("platform: process dir under %q: %w", root, err)
		}
		dirFile, err := openDir(dir)
		if err != nil {
			continue // a reaper took the directory first
		}
		if tryLock(dirFile) == nil && sameFile(dirFile, dir) {
			heldDirLocksMu.Lock()
			heldDirLocks = append(heldDirLocks, dirFile)
			heldDirLocksMu.Unlock()
			return dir, nil
		}
		dirFile.Close()
	}
	return "", fmt.Errorf("platform: process dir under %q: lost %d attempts to a concurrent reap", root, processDirAttempts)
}

// ReapProcessDirs removes each prefix+* directory under root whose lock no
// process holds, and returns how many it removed.  A live process holds its
// directory's lock, this process included (the lock belongs to the holder's
// open file, so this process's own probe is refused too); a directory whose
// creator ended before locking it has no holder either.  Nothing is opened
// through a symlink.
func ReapProcessDirs(root, prefix string) int {
	if !fileLocks {
		return 0
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return 0
	}
	reaped := 0
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		dirFile, err := openDir(dir)
		if err != nil {
			continue
		}
		if tryLock(dirFile) == nil && os.RemoveAll(dir) == nil {
			reaped++
		}
		dirFile.Close()
	}
	return reaped
}

// sameFile reports whether pathname still names file: a reap that removed the
// directory between the create and the lock leaves file unlinked.
func sameFile(file *os.File, pathname string) bool {
	held, err := file.Stat()
	if err != nil {
		return false
	}
	named, err := os.Stat(pathname)
	return err == nil && os.SameFile(held, named)
}
