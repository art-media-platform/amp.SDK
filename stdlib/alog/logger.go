// Package alog is amp's built-in logging primitive — a lean, dependency-free
// tag-scoped logger.  Every log line carries a [Label] prefix (right-padded for
// alignment across the process) and a single-char severity tag.  Optional ANSI
// color on TTY stderr; plain text when tee'd to a file.
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
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
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
	gLongestLabel atomic.Int32
	gOutMu        sync.Mutex
	gDefault      = logger{}
)

const logLineSpacer = "                                                                "

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
	label  string
	prefix string
}

func (l *logger) SetLogLabel(label string) {
	l.label = label
	if label == "" {
		l.prefix = ""
		return
	}
	l.prefix = "[" + label + "]"
	current := int32(len(l.prefix))
	for {
		prev := gLongestLabel.Load()
		if current <= prev || gLongestLabel.CompareAndSwap(prev, current) {
			break
		}
	}
}

func (l *logger) GetLogLabel() string  { return l.label }
func (l *logger) GetLogPrefix() string { return l.prefix }

func (l *logger) padding() string {
	target := int(gLongestLabel.Load())
	have := len(l.prefix)
	if target <= have || target > len(logLineSpacer) {
		return " "
	}
	return logLineSpacer[:target-have+1]
}

// ────────────────────────── emit ──────────────────────────

// emit writes one log line.  depth controls how many stack frames to skip for
// file:line resolution: 0 = emit's caller (Debug/Info/...), 1 = that caller's
// caller (used when a wrapper like Debugw forwards through another method).
func (l *logger) emit(sev severity, depth int, msg string) {
	file, line := callerFileLine(depth + 2)

	var sb strings.Builder
	sb.Grow(96 + len(msg) + len(l.prefix))

	entry := sevTable[sev]
	useColor := gUseColor.Load()

	if useColor && entry.ansi != "" {
		sb.WriteString(entry.ansi)
	}
	sb.WriteByte(entry.tag)
	sb.WriteByte(' ')
	sb.WriteString(time.Now().Format("15:04:05.000"))
	sb.WriteByte(' ')
	sb.WriteString(file)
	sb.WriteByte(':')
	fmt.Fprintf(&sb, "%d", line)
	if useColor && entry.ansi != "" {
		sb.WriteString(ansiReset)
	}
	sb.WriteByte(' ')
	if l.prefix != "" {
		sb.WriteString(l.prefix)
		sb.WriteString(l.padding())
	} else {
		sb.WriteByte(' ')
	}
	sb.WriteString(msg)
	if !strings.HasSuffix(msg, "\n") {
		sb.WriteByte('\n')
	}

	line_bytes := []byte(sb.String())
	gOutMu.Lock()
	os.Stderr.Write(line_bytes)
	if f := openFileLogger(); f != nil {
		// Strip ANSI codes for file output.  The only codes we insert are
		// sevTable[*].ansi and ansiReset — a simple strip is sufficient.
		if useColor && entry.ansi != "" {
			plain := stripANSI(line_bytes)
			f.Write(plain)
		} else {
			f.Write(line_bytes)
		}
	}
	gOutMu.Unlock()
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

func (l *logger) Debug(args ...any)         { l.emit(sevDebug, 0, fmt.Sprint(args...)) }
func (l *logger) Debugf(f string, a ...any) { l.emit(sevDebug, 0, fmt.Sprintf(f, a...)) }
func (l *logger) Warn(args ...any)          { l.emit(sevWarn, 0, fmt.Sprint(args...)) }
func (l *logger) Warnf(f string, a ...any)  { l.emit(sevWarn, 0, fmt.Sprintf(f, a...)) }
func (l *logger) Error(args ...any)         { l.emit(sevError, 0, fmt.Sprint(args...)) }
func (l *logger) Errorf(f string, a ...any) { l.emit(sevError, 0, fmt.Sprintf(f, a...)) }

func (l *logger) Info(level int32, args ...any) {
	if l.LogV(level) {
		l.emit(sevInfo, 0, fmt.Sprint(args...))
	}
}

func (l *logger) Infof(level int32, f string, a ...any) {
	if l.LogV(level) {
		l.emit(sevInfo, 0, fmt.Sprintf(f, a...))
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
