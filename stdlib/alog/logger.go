// Package alog is amp's built-in logging primitive — a lean, dependency-free
// tag-scoped logger.  Each log line is a two-char rank code, a timestamp, a
// bracketed source token, then the message, all space-separated and vertically
// aligned:
//
//	I0 2026-05-24 15:04:05.123 [0…123 app.www    ·   ] some message
//
// The rank code is severity then verbosity level: E0 error, W0 warn, I0/I1/I2… for
// Info(n).  Lower is higher rank, and the digit reads uniformly as suppressibility —
// 0 always prints, ≥1 prints only once -v reaches it.  E and W carry 0 because they
// are unconditional.  A message carrying embedded newlines continues on '··' lines,
// so a record with a stack trace or a dump stays one record and "^[EWI][0-9] "
// remains a true line anchor.
//
// The timestamp is the transport's job wherever a transport does it.  Under systemd
// the journal stamps and stores every entry, so a line bound only for journald drops
// the date and carries 15:04:05.000 — which still earns its columns, because the
// stderr journal path records emit time nowhere else (it passes no source timeval, so
// __REALTIME_TIMESTAMP is receipt time).  Every other destination — a -log_file tee,
// an embedded host's stderr, go test — has no envelope at all and keeps the full
// date.  See AOM O2 §2.8.
//
// The source token leads with the logger's owner id (a task.Context's id, say) when
// it has one, then the label; a logger with no label shows the log call-site file:line
// in the label's place, so every line still names either its context or its origin.
// The interior is right-padded to a sticky width that steps in fours: a guide
// dot falls every fourth column and ']' lands where the next dot would, so the
// rails read vertically.  The width holds a floor so short labels stay aligned,
// caps so one wide label can't swallow the message column, and relaxes toward
// the widest token seen rather than snapping narrow.
// Optional ANSI color on TTY stderr; plain text when tee'd to a file.
//
// Verbosity: Info(n, …) / Infof(n, …) only print when n == 0 (unconditional)
// or n ≤ the global verbosity set via the -v flag.  This matches the pattern
// used throughout amp: high-signal messages log at level 0, progressively
// detailed diagnostics at higher levels.
//
// Named alog (not log) to avoid collision with the Go stdlib log package —
// eliminating the import-alias tax at every call site.
package alog

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unicode/utf8"
)

// Logger abstracts basic logging functions: Info / Warn / Error.  Info and Infof
// take a verbosity level; 0 always logs, higher levels log only when -v ≥ that
// level.
//
// No Success (UX affordance, not a severity), no Fatal (hiding os.Exit inside a
// log call is a footgun; exit at the call site), no Debug (Info's level digit
// already spans diagnostic depth, and a second axis for the same thing invites
// the two disagreeing), no structured-field variants (no callers needed them;
// fmt.Sprintf is fine).
type Logger interface {
	SetLogLabel(label string)
	GetLogLabel() string
	GetLogPrefix() string
	LogV(logLevel int32) bool
	Info(logLevel int32, args ...any)
	Infof(logLevel int32, format string, args ...any)
	Warn(args ...any)
	Warnf(format string, args ...any)
	Error(args ...any)
	Errorf(format string, args ...any)
}

// NewLogger creates a Logger that prefixes its messages with [label].
func NewLogger(label string) Logger {
	l := &logger{}
	l.SetLogLabel(label)
	return l
}

// NewLoggerWithID creates a Logger whose lines lead with id — a short, stable owner
// identifier such as a task.Context's TaskID.AsLabel() — followed by label.  When label
// is empty, the log call-site file:line takes the label's place, so every line still
// carries the id for disambiguation.
func NewLoggerWithID(id, label string) Logger {
	l := &logger{idLabel: id}
	l.SetLogLabel(label)
	return l
}

// InitFlags registers -v and -log_file on the given FlagSet.
func InitFlags(fs *flag.FlagSet) {
	fs.Var(verbosityFlag{}, "v", "log verbosity level (higher = more verbose)")
	fs.StringVar(&gLogFilePath, "log_file", "", "append log output to this file in addition to stderr")
}

// SetColor enables or disables ANSI color in console output.  Defaults to true
// when stderr is a terminal, false otherwise.  File output never carries color.
func SetColor(on bool) { gUseColor.Store(on) }

// ────────────────────────── globals ──────────────────────────

var (
	gVLevel       atomic.Int32
	gUseColor     atomic.Bool
	gLogFilePath  string
	gLogFile      *os.File
	gFileOnce     sync.Once
	gOutMu        sync.Mutex
	gWidths       columnWidths
	gDefault      = logger{}
	gDestOnce     sync.Once
	gStampFormat  = stampFull
	gSyslogPrefix bool
)

const (
	// stampFull dates every line, for a destination that keeps no envelope of its own.
	stampFull = "2006-01-02 15:04:05.000"

	// stampShort drops the date for a journald-only stderr, which stores and renders
	// its own.  Time-of-day stays: the stderr journal path passes no source timeval,
	// so nothing but this field records when the line was actually emitted.
	stampShort = "15:04:05.000"
)

// resolveDestination fixes the line format from the destination set — once, on the
// first emit, which is after flag parsing so -log_file is known.  The short stamp and
// the <N> prefix are systemd's to consume, so both are taken only when stderr IS the
// journal and no file tee competes for the same bytes; a tee would otherwise be handed
// dateless lines and a stray prefix.  Callers must hold gOutMu.
func resolveDestination() {
	if gLogFilePath == "" && stderrIsJournalStream() {
		gStampFormat = stampShort
		gSyslogPrefix = true
	}
}

// columnWidths tracks the sticky pad width of the source column interior (the id, label,
// and/or file:line inside the brackets).  The width grows to fit and holds a floor; once
// every widthRelaxLines it relaxes toward the widest interior seen in that window so a
// past burst of wide entries decays out without snapping the gutter to zero.  All fields
// are read and mutated only under gOutMu, which emit already holds for the write — no
// extra lock, atomic, or goroutine is introduced.
type columnWidths struct {
	source     int // current pad width for the bracket interior
	windowMax  int // widest interior seen in the current relax window
	sinceRelax int // emitted lines since the last relax
}

const (
	// columnHardCap bounds a single token's contribution to the source column.  It is
	// set wide enough that real id, file:line, and label values rarely reach it; a
	// longer value is truncated with an ellipsis so one pathological entry can't blow
	// out the line.
	columnHardCap = 48

	// labelColumnMin is the floor the source column pads to: the gutter never narrows
	// below this, so short labels stay aligned and the column doesn't start from zero
	// and creep wider line by line.  A dot-grid stop (see gridWidth).
	labelColumnMin = 23

	// labelColumnMax caps the source column so one wide label can't swallow the message
	// column; a source longer than this overflows without realigning the rest.
	// A dot-grid stop, like labelColumnMin (see gridWidth).
	labelColumnMax = 43

	// widthRelaxLines is how long a width persists before the column relaxes toward the
	// widest source actually seen in that window (never to zero) — long enough that the
	// gutter settles instead of fighting between narrow and wide, short enough that one
	// wide label doesn't hold the pad hostage for pages.
	widthRelaxLines = 100
)

const ellipsis = "…"

// gutterDot is the guide dot laid on every fourth column of a wide bracket
// gutter (see writeGutter); like ellipsis, one display column wide.
const gutterDot = "·"

func init() {
	gUseColor.Store(isTTY(os.Stderr))
}

func isTTY(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// ────────────────────────── severity ──────────────────────────

type severity int

const (
	sevInfo severity = iota
	sevWarn
	sevError
)

type sevInfoEntry struct {
	tag  byte   // severity char leading the two-char rank code
	ansi string // ANSI SGR prefix (empty = no color)
}

// ANSI codes: reset = \x1b[0m.  Colors stay simple and human-readable.
var sevTable = [...]sevInfoEntry{
	sevInfo:  {tag: 'I', ansi: ""},         // level decides — see lineColor
	sevWarn:  {tag: 'W', ansi: "\x1b[33m"}, // yellow
	sevError: {tag: 'E', ansi: "\x1b[31m"}, // red
}

const (
	ansiReset = "\x1b[0m"

	// ansiDim is worn by gated Info lines.  Level 0 stays undimmed: those are the
	// beacons, and dimming by severity would dim them along with the flood they are
	// meant to stand out from.
	ansiDim = "\x1b[90m" // bright black / grey
)

// lineColor picks a line's color from its rank rather than its severity alone, so the
// verbosity a caller chose is visible instead of merely obeyed.
func lineColor(sev severity, level int32) string {
	if sev == sevInfo && level >= 1 {
		return ansiDim
	}
	return sevTable[sev].ansi
}

// levelDigit renders a verbosity level into the rank code's second column.  The API
// takes an int32 while the column holds one digit, so anything past 9 saturates rather
// than widening the field and breaking every alignment below it.
func levelDigit(level int32) byte {
	switch {
	case level <= 0:
		return '0'
	case level >= 9:
		return '9'
	}
	return byte('0' + level)
}

// continuationMark stands in the rank column for the 2nd and later lines of a message
// carrying embedded newlines: the record stays one record, and a continuation can never
// match the "^[EWI][0-9] " anchor that greps and transcript maskers key on.
const continuationMark = "··"

// syslogPrefix maps a line's rank onto an sd-daemon(3) level prefix.  journald parses it
// into PRIORITY= and strips it before storage (SyslogLevelPrefix= defaults true), so it
// costs three bytes on the wire and nothing in the journal — and it is what lets
// `journalctl -p` cut a running node's output by rank without a restart.
func syslogPrefix(sev severity, level int32) string {
	switch sev {
	case sevError:
		return "<3>" // SD_ERR
	case sevWarn:
		return "<4>" // SD_WARNING
	}
	switch {
	case level <= 0:
		return "<5>" // SD_NOTICE — the unsuppressible identity facts of O2 §2.8
	case level == 1:
		return "<6>" // SD_INFO — the working spine
	}
	return "<7>" // SD_DEBUG — detail, present only because -v asked for it
}

// ────────────────────────── logger ──────────────────────────

type logger struct {
	idLabel string // short, stable owner id shown as the first source token (may be empty)
	label   string
	prefix  string
}

func (l *logger) SetLogLabel(label string) {
	l.label = label
	if label == "" {
		l.prefix = ""
		return
	}
	l.prefix = "[" + label + "]"
}

func (l *logger) GetLogLabel() string  { return l.label }
func (l *logger) GetLogPrefix() string { return l.prefix }

// ────────────────────────── emit ──────────────────────────

// emit writes one record — one line, or several when msg carries embedded newlines.
// level is the caller's verbosity; Warn and Error pass 0, being unconditional.  depth
// selects which stack frame the file:line names: the public entry methods pass 1 to
// name their caller; a wrapper forwarding through another method passes 2.
func (l *logger) emit(sev severity, level int32, depth int, msg string) {
	// Build the bracket interior before taking the lock; its width drives alignment
	// under gOutMu.  The interior is right-padded to that width before ']' so every
	// line's closing bracket lands at the same column, then a single space separates
	// ']' from the message.
	// The interior leads with idLabel when present; the second token is the label, or —
	// when unlabeled — the log call-site file:line, so an anonymous logger still names
	// its origin.
	var second string
	if l.label != "" {
		second = capColumn(l.label)
	} else {
		file, line := callerFileLine(depth + 2)
		second = capColumn(file + ":" + strconv.Itoa(line))
	}
	source := second
	if l.idLabel != "" {
		source = l.idLabel + " " + second // idLabel is the fixed-width AsLabel form
	}
	// Alignment is by display column, not byte: the AsLabel "first…last" id form and
	// capColumn's own elision both carry the 3-byte "…", so byte length over-counts the
	// gutter and short-pads ellipsis-bearing lines against ASCII-only ones.
	sourceCols := utf8.RuneCountInString(source)

	entry := sevTable[sev]
	useColor := gUseColor.Load()
	ansi := lineColor(sev, level)

	var sb strings.Builder
	sb.Grow(96 + len(msg) + len(source))

	gOutMu.Lock()

	gDestOnce.Do(resolveDestination)

	// The clock is read under the lock.  Sampled outside it, two goroutines could stamp
	// in one order and write in the other, producing a file whose timestamps contradict
	// its own line order.
	stamp := time.Now().Format(gStampFormat)

	sourceWidth := gWidths.observe(sourceCols)

	for i, line := range strings.Split(strings.TrimSuffix(msg, "\n"), "\n") {
		if gSyslogPrefix {
			sb.WriteString(syslogPrefix(sev, level))
		}
		if useColor && ansi != "" {
			sb.WriteString(ansi)
		}
		if i == 0 {
			sb.WriteByte(entry.tag)
			sb.WriteByte(levelDigit(level))
			sb.WriteByte(' ')
			sb.WriteString(stamp)
			sb.WriteByte(' ')
			sb.WriteByte('[')
			sb.WriteString(source)
			writeGutter(&sb, sourceCols, sourceWidth)
			sb.WriteByte(']')
		} else {
			// A continuation carries neither stamp nor source: repeating them would
			// squeeze a dump into the narrowest column on the line, which is the
			// opposite of why the dump was logged.
			sb.WriteString(continuationMark)
		}
		if useColor && ansi != "" {
			sb.WriteString(ansiReset)
		}
		sb.WriteByte(' ')
		sb.WriteString(line)
		sb.WriteByte('\n')
	}

	lineBytes := []byte(sb.String())
	os.Stderr.Write(lineBytes)
	if f := openFileLogger(); f != nil {
		// Strip ANSI codes for file output.  The only codes we insert are
		// sevTable[*].ansi and ansiReset — a simple strip is sufficient.
		if useColor && entry.ansi != "" {
			f.Write(stripANSI(lineBytes))
		} else {
			f.Write(lineBytes)
		}
	}
	gOutMu.Unlock()
}

// observe folds one line's source-column length into the pad width and returns the
// width to pad to.  The width grows immediately to fit, holds the labelColumnMin floor,
// caps at labelColumnMax so one wide label can't swallow the message column, and once
// every widthRelaxLines relaxes toward the widest interior actually seen in that window
// — never to zero, so the gutter settles instead of snapping narrow and re-widening.
// The result snaps up to the dot grid so ']' lands on a rail and the column
// resizes in steps of four; see gridWidth.  Callers must hold gOutMu.
func (w *columnWidths) observe(sourceCols int) (sourceWidth int) {
	if sourceCols > w.windowMax {
		w.windowMax = sourceCols
	}
	if sourceCols > w.source {
		w.source = sourceCols
	}
	w.sinceRelax++
	if w.sinceRelax >= widthRelaxLines {
		w.source = w.windowMax
		w.windowMax = 0
		w.sinceRelax = 0
	}
	if w.source > labelColumnMax {
		w.source = labelColumnMax
	}
	if w.source < labelColumnMin {
		w.source = labelColumnMin
	}
	return gridWidth(w.source)
}

// gridWidth rounds a column count up to the next dot-grid stop (interior col
// where col%4==3, a guide-dot column), so a bracket closed at that width lands
// ']' where the next dot would fall and the column resizes in steps of 4.
func gridWidth(cols int) int {
	return ((cols + 4) &^ 3) - 1
}

// capColumn bounds a column value to columnHardCap bytes, replacing the overflow
// tail with an ellipsis so a single huge value can't widen the gutter past the
// cap.  The cut backs off any partial trailing rune to keep output valid UTF-8.
func capColumn(value string) string {
	if len(value) <= columnHardCap {
		return value
	}
	cut := columnHardCap - len(ellipsis)
	for cut > 0 && !utf8.RuneStart(value[cut]) {
		cut--
	}
	return value[:cut] + ellipsis
}

// writeGutter fills the gutter [from, to) with spaces, laying a guide dot on
// every fourth interior column so a wide gutter reads as vertical rails.  The
// grid is anchored to interior column 0 (a fixed screen column) so dots align
// down the page; the first pad column is always a space so no dot hugs the
// label.  from >= to writes nothing (an over-cap source hugs ']').
func writeGutter(sb *strings.Builder, from, to int) {
	for col := from; col < to; col++ {
		if col != from && col&3 == 3 {
			sb.WriteString(gutterDot)
		} else {
			sb.WriteByte(' ')
		}
	}
}

func callerFileLine(skip int) (string, int) {
	_, file, line, ok := runtime.Caller(skip)
	if !ok {
		return "???", 0
	}
	return filepath.Base(file), line
}

func stripANSI(in []byte) []byte {
	out := make([]byte, 0, len(in))
	for i := 0; i < len(in); i++ {
		if in[i] == 0x1b && i+1 < len(in) && in[i+1] == '[' {
			j := i + 2
			for j < len(in) && in[j] != 'm' {
				j++
			}
			if j < len(in) {
				i = j
				continue
			}
		}
		out = append(out, in[i])
	}
	return out
}

func openFileLogger() *os.File {
	if gLogFilePath == "" {
		return nil
	}
	gFileOnce.Do(func() {
		f, err := os.OpenFile(gLogFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			gLogFile = f
		}
	})
	return gLogFile
}

// ────────────────────────── level-flag plumbing ──────────────────────────

type verbosityFlag struct{}

func (verbosityFlag) String() string { return fmt.Sprintf("%d", gVLevel.Load()) }
func (verbosityFlag) Set(s string) error {
	var n int32
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return err
	}
	gVLevel.Store(n)
	return nil
}
func (verbosityFlag) IsBoolFlag() bool { return false }

// ────────────────────────── interface impl ──────────────────────────

func (l *logger) LogV(level int32) bool { return level == 0 || level <= gVLevel.Load() }

func (l *logger) Warn(args ...any)          { l.emit(sevWarn, 0, 1, fmt.Sprint(args...)) }
func (l *logger) Warnf(f string, a ...any)  { l.emit(sevWarn, 0, 1, fmt.Sprintf(f, a...)) }
func (l *logger) Error(args ...any)         { l.emit(sevError, 0, 1, fmt.Sprint(args...)) }
func (l *logger) Errorf(f string, a ...any) { l.emit(sevError, 0, 1, fmt.Sprintf(f, a...)) }

func (l *logger) Info(level int32, args ...any) {
	if l.LogV(level) {
		l.emit(sevInfo, level, 1, fmt.Sprint(args...))
	}
}

func (l *logger) Infof(level int32, f string, a ...any) {
	if l.LogV(level) {
		l.emit(sevInfo, level, 1, fmt.Sprintf(f, a...))
	}
}

// ────────────────────────── interrupt handling ──────────────────────────

// AwaitInterrupt returns two channels: the first closes on SIGINT/SIGTERM; the
// second closes when a 3rd (or later) signal arrives more than 3 seconds after the
// first, so long-running programs can distinguish graceful shutdown from a user
// demanding exit now.
func AwaitInterrupt() (first <-chan struct{}, repeated <-chan struct{}) {
	onFirst := make(chan struct{})
	onRepeated := make(chan struct{})

	go func() {
		sigInbox := make(chan os.Signal, 1)
		// SIGHUP is deliberately excluded: catching it would override the SIG_IGN that
		// nohup installs, letting a closing controlling terminal or SSH session terminate
		// a backgrounded daemon.  SIGINT/SIGTERM are the shutdown signals; for a daemon a
		// hangup means "reload", not "die".
		signal.Notify(sigInbox, syscall.SIGINT, syscall.SIGTERM)

		count := 0
		firstTime := int64(0)

		for sig := range sigInbox {
			count++
			curTime := time.Now().Unix()
			fmt.Println() // clear any un-terminated ^c
			gDefault.emit(sevWarn, 0, 1, "received "+sig.String())

			if onFirst != nil {
				firstTime = curTime
				close(onFirst)
				onFirst = nil
			} else if onRepeated != nil {
				if curTime > firstTime+3 && count >= 3 {
					gDefault.emit(sevWarn, 0, 1, "received repeated interrupts — forcing exit")
					close(onRepeated)
					onRepeated = nil
				}
			}
		}
	}()

	gDefault.emit(sevInfo, 0, 1, fmt.Sprintf("to stop: \x1b[1m^C\x1b[0m or \x1b[1mkill -s SIGINT %d\x1b[0m", os.Getpid()))
	return onFirst, onRepeated
}
