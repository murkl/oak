// Package logging is the single sink for everything a run records, which the
// interface never shows: the runtime's own lines at INFO, WARN and ERROR, a
// script's output at DEBUG (see External).
//
//	2026-08-26 14:08:01 | INFO | Prepare disk
//	2026-08-26 14:08:02 | DEBUG | :: Synchronizing package databases...
//	2026-08-26 14:08:09 | ERROR | Prepare disk failed
package logging

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

const stampLayout = "2006-01-02 15:04:05"

// externalLevel tags output that did not originate in the runtime. A distinct,
// low level keeps the noise filterable.
const externalLevel = "DEBUG"

var (
	mu   sync.Mutex
	file *os.File
	path string

	// The machine's time zone as the link names it, and what it was read as.
	// A run may set the zone, and Go reads it once at start.
	localtime = "/etc/localtime"
	linked    string
	zone      = time.Local
)

// Init opens path as the log file, keeping the previous run's log as
// "<path>.old". Without it every write is a no-op, so a failure here never
// stops a run - losing the log is worse than not having one, but not that much
// worse.
func Init(p string) error {
	mu.Lock()
	defer mu.Unlock()
	if file != nil {
		file.Close()
		file = nil
	}
	if _, err := os.Stat(p); err == nil {
		os.Rename(p, p+".old")
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	file, path = f, p
	return nil
}

// Path is the file being written, so the interface can tell someone where to
// look without being told twice.
func Path() string {
	mu.Lock()
	defer mu.Unlock()
	return path
}

// There is no Close: the log stays open for the life of the process, so a
// failure reported on the way out still lands in it. Every line is written
// straight to the file, so nothing is buffered and nothing is lost at exit.

// Info, Warn and Error record one line for messages the runtime itself emits.
func Info(format string, a ...any)  { write("INFO", fmt.Sprintf(format, a...)) }
func Warn(format string, a ...any)  { write("WARN", fmt.Sprintf(format, a...)) }
func Error(format string, a ...any) { write("ERROR", fmt.Sprintf(format, a...)) }

func write(level, msg string) {
	mu.Lock()
	defer mu.Unlock()
	if file == nil {
		return
	}
	fmt.Fprintf(file, "%s | %s | %s\n", time.Now().In(local()).Format(stampLayout), level, msg)
}

// local is the zone /etc/localtime names now, read again only where the link
// changed. TZ in the environment wins, as it does for Go itself.
func local() *time.Location {
	if _, set := os.LookupEnv("TZ"); set {
		return time.Local
	}
	target, err := os.Readlink(localtime)
	if err != nil || target == linked {
		return zone
	}
	name := target
	if i := strings.LastIndex(target, "zoneinfo/"); i >= 0 {
		name = target[i+len("zoneinfo/"):]
	}
	if loc, err := time.LoadLocation(name); err == nil {
		linked, zone = target, loc
	}
	return zone
}

// External returns an io.Writer that logs a script's output, one log line per
// text line, at externalLevel.
func External() io.Writer { return &lineWriter{level: externalLevel} }

type lineWriter struct {
	level string
	buf   []byte
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		if line := bytes.TrimRight(w.buf[:i], "\r"); len(line) > 0 {
			write(w.level, string(line))
		}
		w.buf = w.buf[i+1:]
	}
	return len(p), nil
}
