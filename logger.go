package traefikgeoip2

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

type logLevel int

const (
	levelError logLevel = iota
	levelWarn
	levelInfo
	levelDebug
)

func parseLogLevel(s string) (logLevel, error) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "", "ERROR":
		return levelError, nil
	case "WARN", "WARNING":
		return levelWarn, nil
	case "INFO":
		return levelInfo, nil
	case "DEBUG":
		return levelDebug, nil
	default:
		return levelError, fmt.Errorf("unknown logLevel %q (expected ERROR, WARN, INFO or DEBUG)", s)
	}
}

func (l logLevel) String() string {
	switch l {
	case levelError:
		return "error"
	case levelWarn:
		return "warn"
	case levelInfo:
		return "info"
	default:
		return "debug"
	}
}

// logger is a leveled logger owned by one middleware instance or one registry
// entry; there is no process-wide logging state. Error and warning lines go to
// stderr, which Traefik records at its error level; info and debug lines go to
// stdout, which Traefik records at its debug level.
type logger struct {
	level  logLevel
	name   string
	mu     sync.Mutex
	errOut io.Writer
	out    io.Writer
}

func newLogger(level logLevel, name string) *logger {
	return &logger{level: level, name: name, errOut: os.Stderr, out: os.Stdout}
}

func (l *logger) enabled(level logLevel) bool { return level <= l.level }

func (l *logger) logf(level logLevel, format string, args ...any) {
	if !l.enabled(level) {
		return
	}
	w := l.out
	if level <= levelWarn {
		w = l.errOut
	}
	msg := fmt.Sprintf(format, args...)
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = fmt.Fprintf(w, "geoip2 level=%s name=%q msg=%q\n", level, l.name, msg)
}

func (l *logger) errorf(format string, args ...any) { l.logf(levelError, format, args...) }
func (l *logger) infof(format string, args ...any)  { l.logf(levelInfo, format, args...) }
func (l *logger) debugf(format string, args ...any) { l.logf(levelDebug, format, args...) }
