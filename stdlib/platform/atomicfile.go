package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// atomicTempInfix joins a target's base name and the random suffix of its
// WriteFileAtomic temp: <base>.tmp-<random>.
const atomicTempInfix = ".tmp-"

// orphanTempGrace keeps a temp younger than this out of the reap: it covers a
// live writer in another process between creating its temp and locking it.
const orphanTempGrace = time.Minute

// targetLocks serializes this process's writers of one target (pathname →
// *sync.Mutex), so a write never reaps a temp this process is still filling.
var targetLocks sync.Map

// WriteFileAtomic replaces pathname's content with data, all or nothing: a
// reader, a crash or a kill sees the complete old file or the complete new
// one, never a truncation or a mix.  The bytes go to a unique temp sibling,
// locked while it is written, synced and renamed over pathname; the directory
// is synced after, where the platform can sync a directory.  A temp a killed
// writer left behind is removed by the target's next write (reapOrphanTemps)
// or by a boot pass (ReapOrphanTemps).  The directory must exist.
func WriteFileAtomic(pathname string, data []byte, perm os.FileMode) error {
	lock, _ := targetLocks.LoadOrStore(pathname, &sync.Mutex{})
	lock.(*sync.Mutex).Lock()
	defer lock.(*sync.Mutex).Unlock()

	reapOrphanTemps(pathname)
	dir := filepath.Dir(pathname)
	tmp, err := os.CreateTemp(dir, filepath.Base(pathname)+atomicTempInfix+"*")
	if err != nil {
		return fmt.Errorf("platform: create temp for %q: %w", pathname, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once renamed
	tryLock(tmp)             // held until the close; a reaper leaves a locked temp

	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return fmt.Errorf("platform: chmod temp %q: %w", tmpName, err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("platform: write temp %q: %w", tmpName, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("platform: sync temp %q: %w", tmpName, err)
	}
	// Where a lock exists the temp stays locked through its rename; elsewhere
	// it closes first, since a platform may refuse to rename an open file.
	if !fileLocks {
		if err := tmp.Close(); err != nil {
			return fmt.Errorf("platform: close temp %q: %w", tmpName, err)
		}
	}
	if err := os.Rename(tmpName, pathname); err != nil {
		tmp.Close()
		return fmt.Errorf("platform: publish %q: %w", pathname, err)
	}
	if fileLocks {
		tmp.Close() // the bytes are synced and published; the close releases the lock
	}
	if dirFile, err := os.Open(dir); err == nil {
		dirFile.Sync() // a platform that cannot sync a directory refuses; the rename stands
		dirFile.Close()
	}
	return nil
}

// reapOrphanTemps removes pathname's temp siblings that a killed writer left.
func reapOrphanTemps(pathname string) {
	prefix := filepath.Base(pathname) + atomicTempInfix
	reapTemps(filepath.Dir(pathname), func(name string) bool {
		return strings.HasPrefix(name, prefix)
	})
}

// ReapOrphanTemps removes the temps directly in dir that killed WriteFileAtomic
// writers left, whatever their target, and returns how many it removed: a boot
// pass for a target written once, whose own next write never comes.
func ReapOrphanTemps(dir string) int {
	return reapTemps(dir, func(name string) bool {
		return strings.Contains(name, atomicTempInfix)
	})
}

// reapTemps removes each regular file in dir whose name matches, past
// orphanTempGrace and with no process holding its lock.  A live writer holds
// its temp's lock, so it is never touched; a platform with no file lock reaps
// nothing.
func reapTemps(dir string, match func(name string) bool) int {
	if !fileLocks {
		return 0
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	reaped := 0
	for _, entry := range entries {
		if !match(entry.Name()) || !entry.Type().IsRegular() {
			continue
		}
		info, err := entry.Info()
		if err != nil || time.Since(info.ModTime()) < orphanTempGrace {
			continue
		}
		if removeUnlocked(filepath.Join(dir, entry.Name())) {
			reaped++
		}
	}
	return reaped
}

// removeUnlocked removes the file at pathname when no process holds its lock,
// taking the lock for the removal; it reports whether it removed the file.
func removeUnlocked(pathname string) bool {
	file, err := os.Open(pathname)
	if err != nil {
		return false
	}
	defer file.Close()
	if tryLock(file) != nil {
		return false
	}
	return os.Remove(pathname) == nil
}
