package alog

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// recordAnchor is the contract every consumer keys on — greps, and the transcript
// masker in amp.planet: a record starts with a severity letter and a level digit, and
// nothing else in the stream may match it.  Continuation lines exist so that a message
// carrying embedded newlines cannot forge one.
var recordAnchor = regexp.MustCompile(`(?m)^[EWI][0-9] `)

// captureStderr swaps os.Stderr for a pipe, runs fn, and returns what alog wrote.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	saved := os.Stderr
	os.Stderr = write

	drained := make(chan string, 1)
	go func() {
		var sb strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := read.Read(buf)
			sb.Write(buf[:n])
			if err != nil {
				break
			}
		}
		drained <- sb.String()
	}()

	fn()
	write.Close()
	os.Stderr = saved
	return <-drained
}

func TestRankCodeAndContinuation(t *testing.T) {
	SetColor(false)
	log := NewLogger("test.rank")

	out := captureStderr(t, func() {
		log.Infof(0, "beacon")
		log.Warnf("careful")
		log.Errorf("broken")
		log.Infof(0, "dump follows\nline two\nline three")
	})

	for _, want := range []string{"I0 ", "W0 ", "E0 "} {
		if !strings.Contains(out, want) {
			t.Errorf("missing rank code %q in:\n%s", want, out)
		}
	}
	if got := strings.Count(out, continuationMark+" "); got != 2 {
		t.Errorf("continuation lines = %d, want 2:\n%s", got, out)
	}
	// Four records were emitted; the two continuation lines must not read as a fifth.
	if got := len(recordAnchor.FindAllString(out, -1)); got != 4 {
		t.Errorf("record anchors = %d, want 4:\n%s", got, out)
	}
}

func TestLevelDigitTracksVerbosity(t *testing.T) {
	SetColor(false)
	gVLevel.Store(2)
	t.Cleanup(func() { gVLevel.Store(0) })

	log := NewLogger("test.level")
	out := captureStderr(t, func() {
		log.Infof(1, "spine")
		log.Infof(2, "detail")
	})

	if !strings.Contains(out, "I1 ") || !strings.Contains(out, "I2 ") {
		t.Errorf("level digits missing from:\n%s", out)
	}
}

func TestJournaldShape(t *testing.T) {
	SetColor(false)
	// The prod path: stderr is the journal, so the date belongs to the transport and the
	// line leads with an sd-daemon(3) priority prefix.  Forced here rather than detected,
	// because $JOURNAL_STREAM cannot be honestly simulated off Linux.
	gDestOnce.Do(func() {}) // spend the Once so resolveDestination cannot overwrite this
	savedFormat, savedPrefix := gStampFormat, gSyslogPrefix
	gStampFormat, gSyslogPrefix = stampShort, true
	t.Cleanup(func() { gStampFormat, gSyslogPrefix = savedFormat, savedPrefix })

	log := NewLogger("test.journald")
	out := captureStderr(t, func() {
		log.Infof(0, "serving on :5192")
		log.Errorf("seal failed")
	})

	if !strings.Contains(out, "<5>I0 ") {
		t.Errorf("beacon missing SD_NOTICE prefix:\n%s", out)
	}
	if !strings.Contains(out, "<3>E0 ") {
		t.Errorf("error missing SD_ERR prefix:\n%s", out)
	}
	if regexp.MustCompile(`\d{4}-\d{2}-\d{2}`).MatchString(out) {
		t.Errorf("journald line still carries a date the journal already stores:\n%s", out)
	}
}

func TestLevelDigitSaturates(t *testing.T) {
	// The API takes an int32 while the column holds one digit; past 9 it must saturate
	// rather than widen the field and break every alignment below it.
	for _, probe := range []struct {
		level int32
		want  byte
	}{{-1, '0'}, {0, '0'}, {3, '3'}, {9, '9'}, {12, '9'}} {
		if got := levelDigit(probe.level); got != probe.want {
			t.Errorf("levelDigit(%d) = %q, want %q", probe.level, got, probe.want)
		}
	}
}

func TestSyslogPrefixRanks(t *testing.T) {
	// journald parses these into PRIORITY=; a wrong mapping silently mis-files a whole
	// severity for `journalctl -p`.
	for _, probe := range []struct {
		sev   severity
		level int32
		want  string
	}{
		{sevError, 0, "<3>"},
		{sevWarn, 0, "<4>"},
		{sevInfo, 0, "<5>"},
		{sevInfo, 1, "<6>"},
		{sevInfo, 2, "<7>"},
		{sevInfo, 7, "<7>"},
	} {
		if got := syslogPrefix(probe.sev, probe.level); got != probe.want {
			t.Errorf("syslogPrefix(%v, %d) = %s, want %s", probe.sev, probe.level, got, probe.want)
		}
	}
}
