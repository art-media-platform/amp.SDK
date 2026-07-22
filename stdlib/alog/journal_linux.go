//go:build linux

package alog

import (
	"os"
	"strconv"
	"strings"
	"syscall"
)

// stderrIsJournalStream reports whether stderr is connected to the systemd journal.
//
// systemd.exec(5) sets $JOURNAL_STREAM to the "DEVICE:INODE" pair of that connection
// so a process can detect the journal without asking.  Testing the variable alone is
// not sufficient — a service may spawn a child that replaces its streams while the
// variable survives — so the pair is compared against fstat(2) of stderr, the check
// systemd.io/JOURNAL_NATIVE_PROTOCOL specifies.
func stderrIsJournalStream() bool {
	spec := os.Getenv("JOURNAL_STREAM")
	if spec == "" {
		return false
	}
	devText, inoText, found := strings.Cut(spec, ":")
	if !found {
		return false
	}
	wantDev, err := strconv.ParseUint(devText, 10, 64)
	if err != nil {
		return false
	}
	wantIno, err := strconv.ParseUint(inoText, 10, 64)
	if err != nil {
		return false
	}
	var stat syscall.Stat_t
	if err := syscall.Fstat(int(os.Stderr.Fd()), &stat); err != nil {
		return false
	}
	return uint64(stat.Dev) == wantDev && uint64(stat.Ino) == wantIno
}
