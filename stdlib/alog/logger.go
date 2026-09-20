// Package alog is amp's built-in logging primitive — a lean, dependency-free
// tag-scoped logger.  Each log line is a two-char rank code, a timestamp, a
// bracketed source token, then the message, space-separated:
//
//	I0 2026-05-24 15:04:05.123 [0…123 app.www] some message
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
// ']' closes at the end of the token, so a record's text is a function of that record
// alone: the same call renders byte-identically in every session, and a line can be
// diffed, hashed, or golden-matched.  A pathologically long token is elided.
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

// InitFlags registers -v and -log_file on the given FlagSet.  Registration is
// side-effect-free (Var values backing onto synchronized globals), so any number
// of FlagSets — one per Environment in an embedding process that builds several —
// may register while live loggers emit, and a later registration never resets a
// value an earlier FlagSet configured.
func InitFlags(fs *flag.FlagSet) {
	fs.Var(verbosityFlag{}, "v", "log verbosity level (higher = more verbose)")
	fs.Var(logFileFlag{}, "log_file", "append log output to this file in addition to stderr")
}

// SetColor enables or disables ANSI color in console output.  Defaults to true
// when stderr is a terminal, false otherwise.  File output never carries color.
func SetColor(on bool) { gUseColor.Store(on) }

// MirrorFunc receives every emitted record — the fully formatted, ANSI-free
// line(s) — after the stderr write.  level is the record's verbosity rank;
// sevTag is the severity char leading the line ('I', 'W', 'E').  Called under
// the emit lock: a mirror must return quickly and must never log through alog
// itself.
type MirrorFunc func(level int32, sevTag byte, line string)

// SetMirror installs (nil removes) the process-wide emit mirror — the interface an
// embedding host (ampd-lib) uses to surface alog output where the embedder is
// looking (e.g. the Unity console), which stderr never reaches.
func SetMirror(mirror MirrorFunc) {
	gOutMu.Lock()
	gMirror = mirror
	gOutMu.Unlock()
}

// ────────────────────────── globals ──────────────────────────

var (
	gVLevel       atomic.Int32
	gUseColor     atomic.Bool
	gLogFilePath  string // -log_file path; guarded by gOutMu (written via logFileFlag)
	gLogFile      *os.File
	gFileOnce     sync.Once
	gOutMu        sync.Mutex
	gDefault      = logger{}
	gDestOnce     sync.Once
	gStampFormat  = stampFull
	gSyslogPrefix bool
	gMirror       MirrorFunc // guarded by gOutMu, like the writes it mirrors
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

// columnHardCap bounds a single token's contribution to the source column.  It is
// set wide enough that real id, file:line, and label values rarely reach it; a
// longer value is truncated with an ellipsis so one pathological entry can't blow
// out the line.
const columnHardCap = 48

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
// than widening the rank code and breaking the "^[EWI][0-9] " line anchor.
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
	// Build the bracket interior before taking the lock.  It leads with idLabel when
	// present; the second token is the label, or — when unlabeled — the log call-site
	// file:line, so an anonymous logger still names its origin.  ']' closes at the
	// token, then a single space separates it from the message.
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
	ansi := lineColor(sev, level)

	var sb strings.Builder
	sb.Grow(96 + len(msg) + len(source))

	gOutMu.Lock()

	gDestOnce.Do(resolveDestination)

	// The clock is read under the lock.  Sampled outside it, two goroutines could stamp
	// in one order and write in the other, producing a file whose timestamps contradict
	// its own line order.
	stamp := time.Now().Format(gStampFormat)

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
	if gMirror != nil {
		mirrored := lineBytes
		if useColor {
			mirrored = stripANSI(lineBytes)
		}
		gMirror(level, entry.tag, string(mirrored))
	}
	gOutMu.Unlock()
}

// capColumn bounds a column value to columnHardCap bytes, replacing the overflow
// tail with an ellipsis so a single huge value can't push the message off the line.
// The cut backs off any partial trailing rune to keep output valid UTF-8.
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

// logFileFlag backs -log_file onto gLogFilePath under gOutMu — the lock emit
// already holds for the reads (resolveDestination, openFileLogger), so a flag
// parse or Set concurrent with a live logger is ordered, and registering the
// flag itself writes nothing.
type logFileFlag struct{}

func (logFileFlag) String() string {
	gOutMu.Lock()
	defer gOutMu.Unlock()
	return gLogFilePath
}

func (logFileFlag) Set(path string) error {
	gOutMu.Lock()
	gLogFilePath = path
	gOutMu.Unlock()
	return nil
}

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

// AwaitInterrupt installs the process shutdown ladder and returns its two stages:
//
//	signal #1 — first closes: begin a graceful stop.
//	signal #2 — repeated closes: an operator escalation, graceful → forced.
//	signal #3 — the OS default disposition is restored, so the next signal
//	            terminates the process and a wedged shutdown is always killable.
//
// A repeated signal is never a no-op, at any interval.  A caller wanting only
// graceful stop ignores repeated; a caller needing a hard exit bound arms its own
// watchdog off these stages rather than re-implementing the ladder.
//
// A general-purpose tool for any daemon consuming this SDK; kept by ruling with
// no in-tree consumer (ampd owns its own richer contract — AOM O2 §2.1).
func AwaitInterrupt() (first <-chan struct{}, repeated <-chan struct{}) {
	onFirst := make(chan struct{})
	onRepeated := make(chan struct{})

	// Buffered: a rapid repeat must not be dropped while the rung goroutine emits
	// the previous rung.
	sigInbox := make(chan os.Signal, 4)
	// SIGHUP is deliberately excluded: catching it would override the SIG_IGN that
	// nohup installs, letting a closing controlling terminal or SSH session terminate
	// a backgrounded daemon.  SIGINT/SIGTERM are the shutdown signals; for a daemon a
	// hangup means "reload", not "die".
	//
	// Notify is installed HERE, synchronously — the ladder is enabled the moment
	// this function returns.  Enabled inside the goroutine instead, a signal
	// landing between return and the goroutine's first run takes the OS default
	// and kills the process.
	signal.Notify(sigInbox, syscall.SIGINT, syscall.SIGTERM)

	//amp:detached — the process-level interrupt wait; alog sits below stdlib/task, so no tree exists here to own it
	go func() {
		reportSignal(<-sigInbox, "stopping")
		close(onFirst)

		reportSignal(<-sigInbox, "again — abandoning the graceful stop")
		close(onRepeated)

		reportSignal(<-sigInbox, "a third time — restoring default handling; the next signal terminates")
		signal.Reset(syscall.SIGINT, syscall.SIGTERM)
	}()

	gDefault.emit(sevInfo, 0, 1, fmt.Sprintf("to stop: \x1b[1m^C\x1b[0m or \x1b[1mkill -s SIGINT %d\x1b[0m (repeat to escalate)", os.Getpid()))
	return onFirst, onRepeated
}

// reportSignal narrates one rung of the shutdown ladder.
func reportSignal(sig os.Signal, note string) {
	fmt.Println() // clear any un-terminated ^C
	gDefault.emit(sevWarn, 0, 2, "received "+sig.String()+" — "+note)
}
