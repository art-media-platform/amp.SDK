//go:build !linux

package alog

// stderrIsJournalStream is always false off Linux — journald is a systemd facility,
// so every other platform takes the self-dated line format.
func stderrIsJournalStream() bool { return false }
