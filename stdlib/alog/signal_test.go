package alog

// signal_test.go — AwaitInterrupt's escalation ladder (F-SDK-SIGNAL-CONTRACT):
// signal #1 begins a graceful stop, signal #2 escalates graceful → forced, and
// signal #3 restores the OS default so a wedged process is always killable.
// The contract is process-global signal disposition, so it is driven in a child
// process: the child arms the ladder, prints a marker per rung, and then wedges
// — only the restored default can end it.

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// syncBuffer captures child stderr for failure forensics: with no Stderr set,
// exec sends the child's stderr to the null device, and an early child death
// (the alog beacon with no rung line after it) leaves no evidence at all.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

const (
	signalChildEnv = "ALOG_SIGNAL_LADDER_CHILD"

	// markerBound bounds the wait for one escalation marker from the child.
	markerBound = 5 * time.Second

	// killBound bounds "a wedged process is always killable" once the ladder is
	// exhausted; signals are re-sent at killRetry until the default disposition
	// takes the process.
	killBound = 5 * time.Second
	killRetry = 100 * time.Millisecond
)

// awaitInterruptChild is the child half: arm the ladder, mark each rung, wedge.
func awaitInterruptChild() {
	first, repeated := AwaitInterrupt()
	go func() {
		<-repeated
		fmt.Println("MARK:repeated")
	}()
	fmt.Println("MARK:armed")
	<-first
	fmt.Println("MARK:first")
	select {} // wedged: no rung of the ladder exits this process
}

// awaitMark waits for one MARK line, failing the test if it does not arrive.
func awaitMark(t *testing.T, child *exec.Cmd, childStderr *syncBuffer, lines <-chan string, want string) {
	t.Helper()
	deadline := time.After(markerBound)
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				waitErr := child.Wait()
				t.Fatalf("child stdout closed awaiting MARK:%s\nchild.Wait: %v\nchild stderr:\n%s",
					want, waitErr, childStderr.String())
			}
			if strings.TrimSpace(line) == "MARK:"+want {
				return
			}
		case <-deadline:
			t.Fatalf("no MARK:%s within %v — the ladder swallowed the signal", want, markerBound)
		}
	}
}

func TestAwaitInterruptEscalationLadder(t *testing.T) {
	if os.Getenv(signalChildEnv) == "1" {
		awaitInterruptChild()
		return
	}

	child := exec.Command(os.Args[0], "-test.run=^TestAwaitInterruptEscalationLadder$")
	child.Env = append(os.Environ(), signalChildEnv+"=1")
	childStderr := &syncBuffer{}
	child.Stderr = childStderr
	stdout, err := child.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := child.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	defer child.Process.Kill()

	lines := make(chan string, 32)
	go func() {
		scan := bufio.NewScanner(stdout)
		for scan.Scan() {
			lines <- scan.Text()
		}
		close(lines)
	}()

	signalChild := func(rung int) {
		t.Helper()
		if err := child.Process.Signal(syscall.SIGTERM); err != nil {
			t.Fatalf("signal #%d: %v", rung, err)
		}
	}

	awaitMark(t, child, childStderr, lines, "armed")

	signalChild(1)
	awaitMark(t, child, childStderr, lines, "first")

	signalChild(2) // graceful → forced; a second signal is never a no-op
	awaitMark(t, child, childStderr, lines, "repeated")

	signalChild(3) // restores the OS default disposition

	// The ladder is exhausted: a further signal must reach the OS default and
	// end the wedged child.  Re-sent until the bound so the assertion does not
	// ride the child's scheduling of rung #3.
	exited := make(chan error, 1)
	go func() { exited <- child.Wait() }()
	deadline := time.After(killBound)
	for {
		select {
		case <-exited:
			if child.ProcessState.Success() {
				t.Fatalf("wedged child exited 0 — it was not killed by the restored default")
			}
			t.Logf("wedged child ended: %v", child.ProcessState)
			return
		case <-deadline:
			t.Fatalf("wedged child survived every signal for %v — no rung restores the OS default", killBound)
		case <-time.After(killRetry):
			child.Process.Signal(syscall.SIGTERM)
		}
	}
}
