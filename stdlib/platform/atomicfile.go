package platform

import (
	"fmt"
	"os"
	"path/filepath"
)

// atomicTempInfix joins a target's base name and the random suffix of its
// WriteFileAtomic temp: <base>.tmp-<random>.
const atomicTempInfix = ".tmp-"

// WriteFileAtomic replaces pathname's content with data, all or nothing: a
// reader, a crash or a kill sees the complete old file or the complete new
// one, never a truncation or a mix.  The bytes go to a unique temp sibling,
// are synced, and the temp is renamed over pathname; the directory is synced
// after, where the platform can sync a directory.  The directory must exist.
func WriteFileAtomic(pathname string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(pathname)
	tmp, err := os.CreateTemp(dir, filepath.Base(pathname)+atomicTempInfix+"*")
	if err != nil {
		return fmt.Errorf("platform: create temp for %q: %w", pathname, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once renamed

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
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("platform: close temp %q: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, pathname); err != nil {
		return fmt.Errorf("platform: publish %q: %w", pathname, err)
	}
	if dirFile, err := os.Open(dir); err == nil {
		dirFile.Sync() // a platform that cannot sync a directory refuses; the rename stands
		dirFile.Close()
	}
	return nil
}
