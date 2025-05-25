package log

import (
	"bytes"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/art-media-platform/amp.SDK/stdlib/klog"
)

type Severity byte

const (
	Severity_Debug   Severity = 0
	Severity_Info    Severity = 1
	Severity_Warning Severity = 2
	Severity_Error   Severity = 3
	Severity_Fatal   Severity = 4
)

var (
	SeverityNames = []string{
		"DEBUG",
		"INFO",
		"WARNING",
		"ERROR",
		"FATAL",
	}
)

// Formatter specifies how each log entry header should be formatted.=
type Formatter interface {
	FormatHeader(severity string, filename string, lineNum int, ioBuf *bytes.Buffer)
}

// Logger abstracts basic logging functions.
type Logger interface {
	SetLogLabel(inLabel string)
	GetLogLabel() string
	GetLogPrefix() string
	Debug(args ...interface{})
	Debugf(format string, args ...interface{})
	Debugw(format string, fields Fields)
	Success(args ...interface{})
	Successf(format string, args ...interface{})
	Successw(format string, fields Fields)
	LogV(logLevel int32) bool
	Info(logLevel int32, args ...interface{})
	Infof(logLevel int32, inFormat string, args ...interface{})
	Infow(format string, fields Fields)
	Warn(args ...interface{})
	Warnf(format string, args ...interface{})
	Warnw(format string, fields Fields)
	Error(args ...interface{})
	Errorf(format string, args ...interface{})
	Errorw(format string, fields Fields)
	Fatalf(format string, args ...interface{})
}

func Flush() {
	klog.Flush()
}

type logger struct {
	logPrefix string
	logLabel  string
}

var (
	gLongestLabel int
	gSpacing      = "                                                               "
)

func (l *logger) Padding() string {
	labelLen := len(l.logLabel)
	if labelLen >= gLongestLabel {
		return " "
	} else {
		return gSpacing[:gLongestLabel-labelLen]
	}
}

func (l *logger) PrefixLabel() string {
	if len(l.logPrefix) == 0 {
		return ""
	}
	return l.logPrefix + l.Padding()
}

// NewLogger creates and inits a new Logger with the given label.
func NewLogger(label string) Logger {
	l := &logger{}
	l.SetLogLabel(label)
	return l
}

// Fatalf -- see Fatalf (above)
func Fatalf(inFormat string, args ...interface{}) {
	gLogger.Fatalf(inFormat, args...)
}

var gLogger = logger{}

// SetLogLabel sets the label prefix for all entries logged.
func (l *logger) SetLogLabel(inLabel string) {
	l.logLabel = inLabel
	if len(inLabel) > 0 {
		l.logPrefix = fmt.Sprintf("[%s] ", inLabel)

		// Find length of longest line
		{
			longest := gLongestLabel
			max := len(gSpacing) - 1
			N := len(l.logPrefix)
			for pos := 0; pos < N; {
				lineEnd := strings.IndexByte(l.logPrefix[pos:], '\n')
				if lineEnd < 0 {
					pos = N
				}
				lineLen := min(max, 1+lineEnd-pos)
				if lineLen > longest {
					longest = lineLen
					gLongestLabel = longest
				}
				pos += lineEnd + 1
			}
		}
	}
}

// GetLogLabel returns the label last set via SetLogLabel()
func (l *logger) GetLogLabel() string {
	return l.logLabel
}

// GetLogPrefix returns the the text that prefixes all log messages for this context.
func (l *logger) GetLogPrefix() string {
	return l.logPrefix
}

// LogV returns true if logging is currently enabled for log verbose level.
func (l *logger) LogV(inVerboseLevel int32) bool {
	return bool(klog.V(klog.Level(inVerboseLevel)))
}

func (l *logger) Debug(args ...interface{}) {
	klog.DebugDepth(1, l.PrefixLabel(), fmt.Sprint(args...))
}

func (l *logger) Debugf(inFormat string, args ...interface{}) {
	klog.DebugDepth(1, l.PrefixLabel(), fmt.Sprintf(inFormat, args...))
}

func (l *logger) Debugw(msg string, fields Fields) {
	klog.DebugDepth(1, l.PrefixLabel(), fmt.Sprintf(msg+" %v", fields))
}

func (l *logger) Success(args ...interface{}) {
	klog.SuccessDepth(1, l.PrefixLabel(), fmt.Sprint(args...))
}

func (l *logger) Successf(inFormat string, args ...interface{}) {
	klog.SuccessDepth(1, l.PrefixLabel(), fmt.Sprintf(inFormat, args...))
}

func (l *logger) Successw(msg string, fields Fields) {
	klog.SuccessDepth(1, l.PrefixLabel(), fmt.Sprintf(msg+" %v", fields))
}

// Info logs to the INFO log.
// Arguments are handled like fmt.Print(); a newline is appended if missing.
//
// Verbose level conventions:
//  0. Enabled during production and field deployment.  Use this for important high-level info.
//  1. Enabled during testing and development. Use for high-level changes in state, mode, or connection.
//  2. Enabled during low-level debugging and troubleshooting.
func (l *logger) Info(inVerboseLevel int32, args ...interface{}) {
	logIt := true
	if inVerboseLevel > 0 {
		logIt = bool(klog.V(klog.Level(inVerboseLevel)))
	}

	if logIt {
		klog.InfoDepth(1, l.PrefixLabel(), fmt.Sprint(args...))
	}
}

// Infof logs to the INFO log.
// Arguments are handled like fmt.Printf(); a newline is appended if missing.
//
// See comments above for Info() for guidelines for inVerboseLevel.
func (l *logger) Infof(inVerboseLevel int32, inFormat string, args ...interface{}) {
	logIt := true
	if inVerboseLevel > 0 {
		logIt = bool(klog.V(klog.Level(inVerboseLevel)))
	}

	if logIt {
		klog.InfoDepth(1, l.PrefixLabel(), fmt.Sprintf(inFormat, args...))
	}
}

func (l *logger) Infow(msg string, fields Fields) {
	klog.InfoDepth(1, l.PrefixLabel(), fmt.Sprintf(msg+" %v", fields))
}

// Warn logs to the WARNING and INFO logs.
// Arguments are handled like fmt.Print(); a newline is appended if missing.
//
// Warnings are reserved for situations that indicate an inconsistency or an error that
// won't result in a departure of specifications, correctness, or expected behavior.
func (l *logger) Warn(args ...interface{}) {
	klog.WarningDepth(1, l.PrefixLabel(), fmt.Sprint(args...))
}

// Warnf logs to the WARNING and INFO logs.
// Arguments are handled like fmt.Printf(); a newline is appended if missing.
//
// See comments above for Warn() for guidelines on errors vs warnings.
func (l *logger) Warnf(inFormat string, args ...interface{}) {
	klog.WarningDepth(1, l.PrefixLabel(), fmt.Sprintf(inFormat, args...))
}

func (l *logger) Warnw(msg string, fields Fields) {
	klog.WarningDepth(1, l.PrefixLabel(), fmt.Sprintf(msg+" %v", fields))
}

// Error logs to the ERROR, WARNING, and INFO logs.
// Arguments are handled like fmt.Print(); a newline is appended if missing.
//
// Errors are reserved for situations that indicate an implementation deficiency, a
// corruption of data or resources, or an issue that if not addressed could spiral into deeper issues.
// Logging an error reflects that correctness or expected behavior is either broken or under threat.
func (l *logger) Error(args ...interface{}) {
	klog.ErrorDepth(1, l.PrefixLabel(), fmt.Sprint(args...))
}

// Errorf logs to the ERROR, WARNING, and INFO logs.
// Arguments are handled like fmt.Print; a newline is appended if missing.
//
// See comments above for Error() for guidelines on errors vs warnings.
func (l *logger) Errorf(inFormat string, args ...interface{}) {
	klog.ErrorDepth(1, l.PrefixLabel(), fmt.Sprintf(inFormat, args...))
}

func (l *logger) Errorw(msg string, fields Fields) {
	klog.ErrorDepth(1, l.PrefixLabel(), fmt.Sprintf(msg+" %v", fields))
}

// Fatalf logs to the FATAL, ERROR, WARNING, and INFO logs,
// Arguments are handled like fmt.Printf(); a newline is appended if missing.
func (l *logger) Fatalf(inFormat string, args ...interface{}) {
	klog.FatalDepth(1, l.PrefixLabel(), fmt.Sprintf(inFormat, args...))
}

func AwaitInterrupt() (
	first <-chan struct{},
	repeated <-chan struct{},
) {
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

			// Prevent un-terminated ^c character in terminal
			fmt.Println()

			klog.WarningDepth(1, "Received ", sig.String(), "\n")

			if onFirst != nil {
				firstTime = curTime
				close(onFirst)
				onFirst = nil
			} else if onRepeated != nil {
				if curTime > firstTime+3 && count >= 3 {
					klog.WarningDepth(1, "Received repeated interrupts\n")
					klog.Flush()
					close(onRepeated)
					onRepeated = nil
				}
			}
		}
	}()

	klog.InfoDepth(1, "To stop: \x1b[1m^C\x1b[0m  or  \x1b[1mkill -s SIGINT ", os.Getpid(), "\x1b[0m")
	return onFirst, onRepeated
}

/*

// Target of written (pushed) log entries.
// It is used to redirect log entries to a different destination, such as a file or a network socket.
type Target interface {
	Write(severity Severity, level int, buf []byte) error
}

func SetOutputBySeverity(target Target, severity ...Severity) {
	for _, si := range severity {
		redirect := &redirect{
			sev: si,
			dst: target,
		}
		klog.SetOutputBySeverity(SeverityNames[si], redirect)
	}
}


func RedirectTo(severity ...string) {
	for _, sev := range severity {
		if sev == "" {
			continue
		}
		si := klog.SeverityByName(sev)
		if si < 0 {
			klog.Fatalf("unknown severity %q", sev)
		}
		redirect := &redirect{
			sev: Severity(si),
			dst: klog.Stdout,
		}
		klog.SetOutputBySeverity(sev, redirect)
	}
}

type redirect struct {
	sev Severity
	dst Target
}

func (r *redirect) Write(p []byte) (n int, err error) {
	n = len(p)
	return n, r.dst.Write(r.sev, 0, p)
}
*/
