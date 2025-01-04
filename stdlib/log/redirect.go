package log

import (
	"github.com/brynbellomy/klog"
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

}

type redirect struct {
	sev Severity
	dst Target
}

func (r *redirect) Write(p []byte) (n int, err error) {
	n = len(p)
	return n, r.dst.Write(r.sev, 0, p)
}
