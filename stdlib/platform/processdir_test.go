package platform

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const helperRootEnv = "AMP_PLATFORM_PROCESS_DIR_ROOT"

// TestProcessDirHelper is the child of TestProcessDir_KilledProcessReaped: it
// creates a process directory with one buffer in it, prints the directory and
// waits to be killed.
func TestProcessDirHelper(t *testing.T) {
	root := os.Getenv(helperRootEnv)
	if root == "" {
		t.Skip("the child process of TestProcessDir_KilledProcessReaped")
	}
	dir, err := CreateProcessDir(root, "amp-test-")
	if err != nil || dir == "" {
		fmt.Println("ERR", err)
		os.Exit(1)
	}
	if err := os.WriteFile(filepath.Join(dir, "buffer"), []byte("plaintext"), 0o600); err != nil {
		fmt.Println("ERR", err)
		os.Exit(1)
	}
	fmt.Println("DIR", dir)
	time.Sleep(time.Hour)
}

// TestProcessDir_KilledProcessReaped: a live process's directory survives a
// reap — another process's and this one's own — and once that process is
// killed (SIGKILL: no handler, no cleanup) the next reap removes it.
func TestProcessDir_KilledProcessReaped(t *testing.T) {
	if !fileLocks {
		t.Skip("no file lock on this platform")
	}
	root := t.TempDir()
	own, err := CreateProcessDir(root, "amp-test-")
	if err != nil || own == "" {
		t.Fatalf("CreateProcessDir = %q, %v", own, err)
	}

	child := exec.Command(os.Args[0], "-test.run=^TestProcessDirHelper$")
	child.Env = append(os.Environ(), helperRootEnv+"="+root)
	stdout, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer child.Process.Kill()
	reported := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if line := scanner.Text(); strings.HasPrefix(line, "DIR ") || strings.HasPrefix(line, "ERR ") {
				reported <- line
				return
			}
		}
		reported <- "ERR no report"
	}()
	childDir := ""
	select {
	case line := <-reported:
		if !strings.HasPrefix(line, "DIR ") {
			t.Fatalf("child: %s", line)
		}
		childDir = strings.TrimPrefix(line, "DIR ")
		t.Logf("child %d holds %s", child.Process.Pid, childDir)
	case <-time.After(30 * time.Second):
		t.Fatal("the child never reported its directory")
	}

	if reaped := ReapProcessDirs(root, "amp-test-"); reaped != 0 {
		t.Fatalf("a reap with both processes live removed %d directories", reaped)
	}
	if _, err := os.Stat(filepath.Join(childDir, "buffer")); err != nil {
		t.Fatalf("the live child's buffer: %v", err)
	}

	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	child.Wait()
	if reaped := ReapProcessDirs(root, "amp-test-"); reaped != 1 {
		t.Fatalf("the reap after the kill removed %d directories, want the child's alone", reaped)
	}
	if _, err := os.Stat(childDir); !os.IsNotExist(err) {
		t.Fatalf("the killed child's directory remains: %v", err)
	}
	if _, err := os.Stat(own); err != nil {
		t.Fatalf("this process's own directory: %v", err)
	}
}

// TestProcessDir_UnlockedReaped: a directory whose creator ended before its
// lock file existed is reaped too.
func TestProcessDir_UnlockedReaped(t *testing.T) {
	if !fileLocks {
		t.Skip("no file lock on this platform")
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "amp-test-bare"), 0o700); err != nil {
		t.Fatal(err)
	}
	if reaped := ReapProcessDirs(root, "amp-test-"); reaped != 1 {
		t.Fatalf("reaped %d, want the bare directory", reaped)
	}
}
