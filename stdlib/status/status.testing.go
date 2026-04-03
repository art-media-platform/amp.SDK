package status

import (
	"fmt"
	"testing"
	"time"
)

// Require fatals the test if got != expected.
func Require(t testing.TB, got, expected any) {
	t.Helper()
	if got != expected {
		t.Fatalf("expected %v, got %v", expected, got)
	}
}

// RequireEventually polls condition at the given interval, fataling if it doesn't return true within timeout.
func RequireEventually(t testing.TB, condition func() bool, timeout, poll time.Duration, msgAndArgs ...any) {
	t.Helper()
	deadline := time.After(timeout)
	tick := time.NewTicker(poll)
	defer tick.Stop()

	for {
		if condition() {
			return
		}
		select {
		case <-deadline:
			msg := "condition not met"
			if len(msgAndArgs) > 0 {
				msg = fmt.Sprintf(fmt.Sprint(msgAndArgs[0]), msgAndArgs[1:]...)
			}
			t.Fatalf("timed out after %v: %s", timeout, msg)
			return
		case <-tick.C:
		}
	}
}
