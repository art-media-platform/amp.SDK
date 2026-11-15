//go:build darwin || linux

package platform

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestRemoveUnlocked_SkipsSymlinkAndFIFO: a temp's name swapped for a symlink,
// a directory or a FIFO is skipped — the symlink is not followed or unlinked,
// the directory is kept, and the FIFO's open does not block the reap.
func TestRemoveUnlocked_SkipsSymlinkAndFIFO(t *testing.T) {
	dir := t.TempDir()
	swappedDir := filepath.Join(dir, "role.member"+atomicTempInfix+"3")
	if err := os.Mkdir(swappedDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if removeUnlocked(swappedDir) {
		t.Error("removeUnlocked removed a directory")
	}
	outside := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(outside, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "role.member"+atomicTempInfix+"1")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if removeUnlocked(link) {
		t.Error("removeUnlocked removed a symlink")
	}
	if _, err := os.Lstat(link); err != nil {
		t.Errorf("the symlink: %v", err)
	}

	fifo := filepath.Join(dir, "role.member"+atomicTempInfix+"2")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	removed := make(chan bool, 1)
	go func() { removed <- removeUnlocked(fifo) }()
	select {
	case got := <-removed:
		if got {
			t.Error("removeUnlocked removed a FIFO")
		}
	case <-time.After(10 * time.Second):
		writer, err := os.OpenFile(fifo, os.O_WRONLY, 0) // releases the blocked open
		if err == nil {
			writer.Close()
		}
		t.Fatal("removeUnlocked blocked on a FIFO")
	}
}

// TestOpenDir_RefusesSymlink: openDir does not follow a symlink to a directory.
func TestOpenDir_RefusesSymlink(t *testing.T) {
	link := filepath.Join(t.TempDir(), "amp-test-link")
	if err := os.Symlink(t.TempDir(), link); err != nil {
		t.Fatal(err)
	}
	if dirFile, err := openDir(link); err == nil {
		dirFile.Close()
		t.Fatal("openDir followed a symlink")
	}
}

// TestLockOpenedDir_ReapedBetweenOpenAndLock: a directory a reap removed after
// its creator opened it, and that the creator's lock then finds unheld, is
// reported reaped, never returned as the creator's live directory.
func TestLockOpenedDir_ReapedBetweenOpenAndLock(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "amp-test-1")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	dirFile, err := openDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	held, reaped, err := lockOpenedDir(dirFile, dir)
	if err == nil {
		held.Close()
		t.Fatal("the lock of a removed directory succeeded")
	}
	if !reaped {
		t.Errorf("a removed directory reads as a real error (%v), want reaped", err)
	}
}
