package alog

import (
	"strings"
	"testing"
)

// TestSetMirror pins the emit-mirror contract from the receiving side: every
// emitted record reaches the mirror as its fully formatted, ANSI-free line
// with the severity tag and verbosity rank intact — including under forced
// color, which the mirror must never see.
func TestSetMirror(t *testing.T) {
	type record struct {
		level  int32
		sevTag byte
		line   string
	}
	var got []record
	SetMirror(func(level int32, sevTag byte, line string) {
		got = append(got, record{level, sevTag, line})
	})
	colorWas := gUseColor.Load()
	SetColor(true) // force ANSI on stderr to prove the mirror stays clean
	t.Cleanup(func() {
		SetMirror(nil)
		SetColor(colorWas)
	})

	log := NewLogger("mirror-test")
	log.Infof(0, "hello %d", 7)
	log.Warnf("watch %s", "this")

	if len(got) != 2 {
		t.Fatalf("mirror saw %d records, want 2", len(got))
	}
	info, warn := got[0], got[1]
	if info.sevTag != 'I' || info.level != 0 || !strings.Contains(info.line, "hello 7") {
		t.Errorf("info record wrong: tag=%q level=%d line=%q", info.sevTag, info.level, info.line)
	}
	if warn.sevTag != 'W' || !strings.Contains(warn.line, "watch this") {
		t.Errorf("warn record wrong: tag=%q line=%q", warn.sevTag, warn.line)
	}
	for _, rec := range got {
		if strings.Contains(rec.line, "\x1b[") {
			t.Errorf("mirror line carries ANSI: %q", rec.line)
		}
		if !strings.Contains(rec.line, "[mirror-test") {
			t.Errorf("mirror line lost its source column: %q", rec.line)
		}
	}

	// A gated Info line never reaches the mirror (the mirror sees post-gate
	// volume, same as stderr).
	before := len(got)
	log.Infof(9, "gated line")
	if len(got) != before {
		t.Errorf("verbosity-gated line leaked to the mirror")
	}
}
