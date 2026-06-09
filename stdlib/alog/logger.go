// Package alog is amp's built-in logging primitive — a lean, dependency-free
// tag-scoped logger.  Each log line is a single-char severity tag, a timestamp,
// a bracketed source token, then the message, all space-separated and vertically
// aligned:
//
//	I 2026-05-24 15:04:05.123 [00..1234 app.www] some message
//
// The source token leads with the logger's owner id (a task.Context's id, say) when
// it has one, then the label; a logger with no label shows the log call-site file:line
// in the label's place, so every line still names either its context or its origin.
// The interior is right-padded to a sticky width: it grows to fit, holds a floor so
// short labels stay aligned, caps so one wide label can't swallow the message column,
// and relaxes toward the widest token recently seen rather than snapping narrow.
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

// Logger abstracts basic logging functions.  Levels follow Go's slog convention:
// Debug / Info / Warn / Error.  Info and Infof take a verbosity level; 0 always
// logs, higher levels log only when -v ≥ that level.
//
// No Success (UX affordance, not a severity), no Fatal (hiding os.Exit inside a
// log call is a footgun; exit at the call site), no structured-field variants
// (no callers needed them; fmt.Sprintf is fine).
type Logger interface {
	SetLogLabel(label string)
	GetLogLabel() string
	GetLogPrefix() string
	Debug(args ...any)
	Debugf(format string, args ...any)
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
	gVLevel      atomic.Int32
	gUseColor    atomic.Bool
	gLogFilePath string
	gLogFile     *os.File
	gFileOnce    sync.Once
	gOutMu       sync.Mutex
	gWidths      columnWidths
	gDefault     = logger{}
)

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
	// and creep wider line by line.
	labelColumnMin = 24

	// labelColumnMax caps the source column so one wide label can't swallow the message
	// column; a source longer than this overflows without realigning the rest.
	labelColumnMax = 44

	// widthRelaxLines is how long a width persists before the column relaxes toward the
	// widest source actually seen in that window (never to zero) — long enough that the
	// gutter settles instead of fighting between narrow and wide.
	widthRelaxLines = 512
)

// spacer supplies right-padding without per-line allocation; writePad repeats it
// for pads wider than its length.
const spacer = "                                "

const ellipsis = "…"

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
	sevDebug severity = iota
	sevInfo
	sevWarn
	sevError
)

type sevInfoEntry struct {
	tag  byte   // single-char code printed in the line header
	ansi string // ANSI SGR prefix (empty = no color)
}

// ANSI codes: reset = \x1b[0m.  Colors stay simple and human-readable.
var sevTable = [...]sevInfoEntry{
	sevDebug: {tag: 'D', ansi: "\x1b[90m"}, // bright black / grey
	sevInfo:  {tag: 'I', ansi: ""},         // default
	sevWarn:  {tag: 'W', ansi: "\x1b[33m"}, // yellow
	sevError: {tag: 'E', ansi: "\x1b[31m"}, // red
}

const ansiReset = "\x1b[0m"

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

// emit writes one log line.  depth selects which stack frame the file:line names:
// the public entry methods (Debug/Info/...) pass 1 to name their caller; a
// wrapper that forwards through another method passes 2.
func (l *logger) emit(sev severity, depth int, msg string) {
	// Build the bracket interior before taking the lock; its width drives alignment
	// under gOutMu, and the padding goes inside the brackets so the closing ']'
	// right-justifies against the message column.  The interior leads with idLabel
	// when present; the second token is the label, or — when unlabeled — the log
	// call-site file:line, so an anonymous logger still names its origin.
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

	entry := sevTable[sev]
	useColor := gUseColor.Load()
	now := time.Now()

	var sb strings.Builder
	sb.Grow(96 + len(msg) + len(source))

	gOutMu.Lock()

	sourceWidth := gWidths.observe(len(source))

	if useColor && entry.ansi != "" {
		sb.WriteString(entry.ansi)
	}
	sb.WriteByte(entry.tag)
	sb.WriteByte(' ')
	sb.WriteString(now.Format("2006-01-02 15:04:05.000"))
	sb.WriteByte(' ')
	sb.WriteByte('[')
	sb.WriteString(source)
	writePad(&sb, sourceWidth-len(source))
	sb.WriteByte(']')
	if useColor && entry.ansi != "" {
		sb.WriteString(ansiReset)
	}
	sb.WriteByte(' ')
	sb.WriteString(msg)
	if !strings.HasSuffix(msg, "\n") {
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
// Callers must hold gOutMu.
func (w *columnWidths) observe(sourceLen int) (sourceWidth int) {
	if sourceLen > w.windowMax {
		w.windowMax = sourceLen
	}
	if sourceLen > w.source {
		w.source = sourceLen
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
	return w.source
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

// writePad appends n spaces (clamped to the spacer's length) to sb.
func writePad(sb *strings.Builder, n int) {
	if n <= 0 {
		return
	}
	for n > len(spacer) {
		sb.WriteString(spacer)
		n -= len(spacer)
	}
	sb.WriteString(spacer[:n])
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

func (l *logger) Debug(args ...any)         { l.emit(sevDebug, 1, fmt.Sprint(args...)) }
func (l *logger) Debugf(f string, a ...any) { l.emit(sevDebug, 1, fmt.Sprintf(f, a...)) }
func (l *logger) Warn(args ...any)          { l.emit(sevWarn, 1, fmt.Sprint(args...)) }
func (l *logger) Warnf(f string, a ...any)  { l.emit(sevWarn, 1, fmt.Sprintf(f, a...)) }
func (l *logger) Error(args ...any)         { l.emit(sevError, 1, fmt.Sprint(args...)) }
func (l *logger) Errorf(f string, a ...any) { l.emit(sevError, 1, fmt.Sprintf(f, a...)) }

func (l *logger) Info(level int32, args ...any) {
	if l.LogV(level) {
		l.emit(sevInfo, 1, fmt.Sprint(args...))
	}
}

func (l *logger) Infof(level int32, f string, a ...any) {
	if l.LogV(level) {
		l.emit(sevInfo, 1, fmt.Sprintf(f, a...))
	}
}

// ────────────────────────── interrupt handling ──────────────────────────

// AwaitInterrupt returns two channels: the first closes on SIGINT/SIGTERM/SIGHUP,
// the second closes on a sustained burst (3 signals within 3 seconds) so long-
// running programs can distinguish graceful shutdown from user demanding exit now.
func AwaitInterrupt() (first <-chan struct{}, repeated <-chan struct{}) {
	onFirst := make(chan struct{})
	onRepeated := make(chan struct{})

	go func() {
		sigInbox := make(chan os.Signal, 1)
		signal.Notify(sigInbox, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)

		count := 0
		firstTime := int64(0)

		for sig := range sigInbox {
			count++
			curTime := time.Now().Unix()
			fmt.Println() // clear any un-terminated ^c
			gDefault.emit(sevWarn, 1, "received "+sig.String())

			if onFirst != nil {
				firstTime = curTime
				close(onFirst)
				onFirst = nil
			} else if onRepeated != nil {
				if curTime > firstTime+3 && count >= 3 {
					gDefault.emit(sevWarn, 1, "received repeated interrupts — forcing exit")
					close(onRepeated)
					onRepeated = nil
				}
			}
		}
	}()

	gDefault.emit(sevInfo, 1, fmt.Sprintf("to stop: \x1b[1m^C\x1b[0m or \x1b[1mkill -s SIGINT %d\x1b[0m", os.Getpid()))
	return onFirst, onRepeated
}
