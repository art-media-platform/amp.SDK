package platform

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
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
// locked one (a live writer's), one within the grace, a name of the temp
// shape's prefix that os.CreateTemp never makes, and another target's temp.
func TestWriteFileAtomic_ReapsOrphanTemps(t *testing.T) {
	if !fileLocks {
		t.Skip("no file lock on this platform")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "epoch-keys.tome")
	old := time.Now().Add(-2 * orphanTempGrace)
	temps := map[string]time.Time{
		"101":    old,
		"202":    old,
		"303":    time.Now(),
		"backup": old,
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
	liveWriter, err := os.Open(target + atomicTempInfix + "202")
	if err != nil {
		t.Fatal(err)
	}
	defer liveWriter.Close()
	if tryLock(liveWriter) != nil {
		t.Fatal("could not lock the live writer's temp")
	}

	otherTarget := filepath.Join(dir, "other.key"+atomicTempInfix+"404")
	if err := os.WriteFile(otherTarget, []byte("other"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(otherTarget, old, old); err != nil {
		t.Fatal(err)
	}

	if err := WriteFileAtomic(target, []byte("tome"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(otherTarget); err != nil {
		t.Errorf("a target's write reaped another target's temp: %v", err)
	}
	want := map[string]bool{
		"101":    false,
		"202":    true,
		"303":    true,
		"backup": true,
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
// a live writer's temp, a fresh temp, a temp-named directory and every name
// os.CreateTemp never makes for WriteFileAtomic (a person's or another
// program's file).
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
		"notes.tmp-backup.txt":                   old,
		"report.tmp-2026-11-01.pdf":              old,
		".zsh_history.tmp-old":                   old,
		"session.tmp-":                           old,
		".tmp-12345":                             old,
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
		"notes.tmp-backup.txt":                   true,
		"report.tmp-2026-11-01.pdf":              true,
		".zsh_history.tmp-old":                   true,
		"session.tmp-":                           true,
		".tmp-12345":                             true,
	}
	for name, kept := range want {
		_, err := os.Stat(filepath.Join(dir, name))
		if (err == nil) != kept {
			t.Errorf("%q present = %v, want %v", name, err == nil, kept)
		}
	}
}

const atomicKillTargetEnv = "AMP_PLATFORM_ATOMIC_KILL_TARGET"

var (
	atomicKillOld = bytes.Repeat([]byte("old content "), 1000)
	atomicKillNew = bytes.Repeat([]byte("the new content, longer "), 4000)
)

// TestWriteFileAtomicKillHelper is the child of TestWriteFileAtomic_KilledBeforeRename:
// it writes the new content to the target and, with its temp written and synced
// but not yet renamed, reports the temp and waits to be killed.
func TestWriteFileAtomicKillHelper(t *testing.T) {
	target := os.Getenv(atomicKillTargetEnv)
	if target == "" {
		t.Skip("the child process of TestWriteFileAtomic_KilledBeforeRename")
	}
	testHookBeforeRename = func(tmpName string) {
		fmt.Println("TEMP", tmpName)
		time.Sleep(time.Hour)
	}
	err := WriteFileAtomic(target, atomicKillNew, 0o600)
	fmt.Println("ERR returned:", err)
	os.Exit(1)
}

// TestWriteFileAtomic_KilledBeforeRename: a writer killed (SIGKILL: no handler,
// no cleanup) between its temp write and the rename leaves the target whole and
// old, with the new bytes whole in the temp alone; the next write replaces it.
func TestWriteFileAtomic_KilledBeforeRename(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "role.member")
	if err := WriteFileAtomic(target, atomicKillOld, 0o600); err != nil {
		t.Fatal(err)
	}

	child := exec.Command(os.Args[0], "-test.run=^TestWriteFileAtomicKillHelper$")
	child.Env = append(os.Environ(), atomicKillTargetEnv+"="+target)
	childErr := &bytes.Buffer{}
	child.Stderr = childErr
	stdout, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	report := "ERR no report"
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		if line := scanner.Text(); strings.HasPrefix(line, "TEMP ") || strings.HasPrefix(line, "ERR ") {
			report = line
			break
		}
	}
	child.Process.Kill()
	waitErr := child.Wait()
	if !strings.HasPrefix(report, "TEMP ") {
		t.Fatalf("child: %s (exit: %v; stderr: %s)", report, waitErr, childErr)
	}
	tmpName := strings.TrimPrefix(report, "TEMP ")
	if base, ok := atomicTempTarget(filepath.Base(tmpName)); !ok || base != filepath.Base(target) {
		t.Fatalf("the temp %s is not of the shape the reaps match", filepath.Base(tmpName))
	}

	raw, err := os.ReadFile(target)
	if err != nil || !bytes.Equal(raw, atomicKillOld) {
		t.Fatalf("after the kill the target holds %d bytes (err %v), want the old %d whole", len(raw), err, len(atomicKillOld))
	}
	raw, err = os.ReadFile(tmpName)
	if err != nil || !bytes.Equal(raw, atomicKillNew) {
		t.Fatalf("after the kill the temp holds %d bytes (err %v), want the new %d whole", len(raw), err, len(atomicKillNew))
	}
	if err := WriteFileAtomic(target, atomicKillNew, 0o600); err != nil {
		t.Fatal(err)
	}
	if raw, err := os.ReadFile(target); err != nil || !bytes.Equal(raw, atomicKillNew) {
		t.Fatalf("the write after the kill: %d bytes (err %v), want the new %d whole", len(raw), err, len(atomicKillNew))
	}
}
