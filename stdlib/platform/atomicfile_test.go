package platform

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestWriteFileAtomic: a write creates the file with perm, a rewrite replaces
// it whole, no temp sibling remains, and a failed write leaves the old content.
func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "role.member")

	if err := WriteFileAtomic(target, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(target, []byte("second, longer"), 0o640); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(target)
	if err != nil || !bytes.Equal(raw, []byte("second, longer")) {
		t.Fatalf("content = %q (err %v), want the second write whole", raw, err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("perm = %v, want 0640", info.Mode().Perm())
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("dir holds %d entries, want the target alone", len(entries))
	}

	// A rename onto a directory fails: the old content stands, no temp remains.
	blocked := filepath.Join(dir, "blocked")
	if err := os.MkdirAll(filepath.Join(blocked, "child"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(blocked, []byte("x"), 0o600); err == nil {
		t.Fatal("a write onto a non-empty directory succeeded")
	}
	entries, err = os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), atomicTempInfix) {
			t.Fatalf("a failed write left its temp %s", entry.Name())
		}
	}
}

// TestWriteFileAtomic_ReapsOrphanTemps: a target's write removes a temp sibling
// past the grace whose lock no process holds (a killed writer's), and leaves a
// locked one (a live writer's) and one within the grace.
func TestWriteFileAtomic_ReapsOrphanTemps(t *testing.T) {
	if !fileLocks {
		t.Skip("no file lock on this platform")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "epoch-keys.tome")
	old := time.Now().Add(-2 * orphanTempGrace)
	temps := map[string]time.Time{
		"orphan": old,
		"live":   old,
		"fresh":  time.Now(),
	}
	for name, mtime := range temps {
		path := target + atomicTempInfix + name
		if err := os.WriteFile(path, []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
	liveWriter, err := os.Open(target + atomicTempInfix + "live")
	if err != nil {
		t.Fatal(err)
	}
	defer liveWriter.Close()
	if tryLock(liveWriter) != nil {
		t.Fatal("could not lock the live writer's temp")
	}

	if err := WriteFileAtomic(target, []byte("tome"), 0o600); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"orphan": false,
		"live":   true,
		"fresh":  true,
	}
	for name, kept := range want {
		_, err := os.Stat(target + atomicTempInfix + name)
		if (err == nil) != kept {
			t.Errorf("temp %q present = %v, want %v", name, err == nil, kept)
		}
	}
}

// TestReapOrphanTemps: the boot pass removes every target's temp in the
// directory that a killed writer left (past the grace, lock unheld), and leaves
// a live writer's temp, a fresh temp, a temp-named directory and other files.
func TestReapOrphanTemps(t *testing.T) {
	if !fileLocks {
		t.Skip("no file lock on this platform")
	}
	dir := t.TempDir()
	old := time.Now().Add(-2 * orphanTempGrace)
	files := map[string]time.Time{
		"role.member":                            old,
		"role.member" + atomicTempInfix + "1":    old,
		"activation.key" + atomicTempInfix + "2": old,
		"live.key" + atomicTempInfix + "3":       old,
		"fresh.key" + atomicTempInfix + "4":      time.Now(),
		"unrelated":                              old,
	}
	for name, mtime := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
	subdir := filepath.Join(dir, "sub"+atomicTempInfix+"5")
	if err := os.Mkdir(subdir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(subdir, old, old); err != nil {
		t.Fatal(err)
	}
	liveWriter, err := os.Open(filepath.Join(dir, "live.key"+atomicTempInfix+"3"))
	if err != nil {
		t.Fatal(err)
	}
	defer liveWriter.Close()
	if tryLock(liveWriter) != nil {
		t.Fatal("could not lock the live writer's temp")
	}

	if reaped := ReapOrphanTemps(dir); reaped != 2 {
		t.Errorf("reaped %d, want the two orphans", reaped)
	}
	want := map[string]bool{
		"role.member":                            true,
		"role.member" + atomicTempInfix + "1":    false,
		"activation.key" + atomicTempInfix + "2": false,
		"live.key" + atomicTempInfix + "3":       true,
		"fresh.key" + atomicTempInfix + "4":      true,
		"unrelated":                              true,
		"sub" + atomicTempInfix + "5":            true,
	}
	for name, kept := range want {
		_, err := os.Stat(filepath.Join(dir, name))
		if (err == nil) != kept {
			t.Errorf("%q present = %v, want %v", name, err == nil, kept)
		}
	}
}
