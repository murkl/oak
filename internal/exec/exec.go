// Package exec is the only place this program starts a process. Everything the
// runtime does is shell, handed variables and read back as an exit code or
// stdout.
package exec

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"

	"github.com/murkl/oak/internal/logging"

	"github.com/charmbracelet/x/ansi"
	"golang.org/x/sys/unix"
)

// Env is the variable set handed to every script, as KEY=value entries.
type Env []string

// Runner starts scripts, each wrapped in the same failure-reporting trap. Shell
// is the product's shell put in front of every script, empty where there is
// none, and Module names the module in every failure.
type Runner struct {
	Shell  string
	Module string
}

// Script is one task's work: a file to source, or shell its yaml wrote
// outright, exactly one of the two set. They run differently, since a file's
// last status propagates out of `source` and is no failure.
type Script struct {
	File  string
	Shell string
}

// Step is one piece of a module's work as this layer takes it: the shell
// itself, what a failure calls it, and whether it is an action's rather than a
// task's - the one thing a failure report says differently about the two.
type Step struct {
	Name   string
	Action bool
	Script Script
}

// shell is either of them as one piece of shell, for the places that only run
// it and never report on where it broke.
func (s Script) shell() string {
	if s.File != "" {
		return "source " + quote(s.File)
	}
	return s.Shell
}

func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// The ERR trap of the wrappers below reports on file descriptor 3, so a script
// may print anything without it being taken for a report. -e stays off: it
// would also end the shell where `source` returns the status of a benign last
// line such as `[ "$X" = true ] && do_it`.
const strict = `set -Eo pipefail
`

// strictTrace is the same with the DEBUG trap carried into everything the
// script runs, which is what lets lastLine see past a function call.
const strictTrace = `set -Eo pipefail -T
`

// The product's shell, sourced in front of every script, so a script needs no
// preamble and reaches every function. It is loaded before the trap is set, so
// a fallback inside it is not blamed on the unit about to run, and only a shell
// that will not load fails here.
const preamble = `[ -z "$2" ] || source "$2" || exit $?
`

// The trap in its two shapes: a file's names the file and the line, shell a
// yaml wrote names the command. The empty BASH_SOURCE check drops the status a
// failing `source` hands back to the wrapper.
const (
	fileTrap = `trap 'c=$?; s=${BASH_SOURCE[0]}; [ -n "$s" ] && { printf "%d\t%s\t%d\t%s\n" "$c" "$s" "$LINENO" "$BASH_COMMAND" >&3; exit $c; }' ERR
`
	shellTrap = `trap 'c=$?; printf "%d\t\t%d\t%s\n" "$c" "$LINENO" "$BASH_COMMAND" >&3; exit $c' ERR
`
)

// lastLine remembers the script's own last line, for a script that says no
// without a failing command. `set -T` carries it into the functions the script
// calls, whose lines it skips.
const lastLine = `trap '[ "${BASH_SOURCE[0]}" = "$1" ] && { oak_line=$LINENO; oak_cmd=$BASH_COMMAND; }; :' DEBUG
`

// The two wrappers answer with the script's own exit status under the trap, so
// `exit 1`, `return 1` and a failing last line all fail a task, its test and an
// action alike. Where the trap saw nothing, the file wrapper reports the last
// line the script was on.
const (
	fileWrapper = strictTrace + preamble + fileTrap + lastLine + `source "$1"
c=$?
[ "$c" -eq 0 ] || printf "%d\t%s\t%d\t%s\n" "$c" "$1" "${oak_line:-0}" "$oak_cmd" >&3
exit $c`

	shellWrapper = strict + preamble + shellTrap + `eval "oak_task() {
$1
}"
oak_task
exit $?`
)

// wrap is the shell that runs one step, and what that shell is handed.
func wrap(step Step) (wrapper, payload string) {
	if step.Script.File != "" {
		return fileWrapper, step.Script.File
	}
	return shellWrapper, step.Script.Shell
}

// snippet runs what the yaml names - an option list, a prefill. No ERR trap:
// the caller wants the exit code or the output, and a non-zero status is an
// answer rather than a failure.
const snippet = preamble + `eval "$1"`

// guard runs a module's shell as a question with a yes or no for an answer. It
// is wrapped in a function as a task's inline shell is, so `return 0` says yes
// in both, and has no ERR trap since a no is an answer.
const guard = preamble + `eval "oak_guard() {
$1
}"
oak_guard
exit $?`

// handover runs a script that takes the terminal over, with no trap and no
// pipes: its output is the terminal's, and its exit code is all that comes
// back. See Handover.
const handover = preamble + `eval "$1"`

// args is how bash is handed a wrapper: the wrapper itself, then what it runs
// as $1 and the shell to load in front of it as $2.
func (r Runner) args(wrapper, payload string) []string {
	return []string{"-c", wrapper, "--", payload, r.Shell}
}

// bash is the one way this package starts a process. Descriptors above stderr
// are marked close-on-exec first, since bubbletea opens its epoll without the
// flag; a kernel before 5.11 leaves them as they are.
func bash(args []string) *exec.Cmd {
	_ = unix.CloseRange(3, math.MaxUint32, unix.CLOSE_RANGE_CLOEXEC)
	return exec.Command("bash", args...)
}

// Run executes a one-liner and returns its trimmed stdout. Used for the small
// reads: an option list, a suggested value.
func (r Runner) Run(s string, env Env) (string, error) {
	out, said, err := r.say(s, env)
	switch {
	case err == nil:
		return out, nil
	case said != "":
		return "", fmt.Errorf("%s: %s", err, said)
	}
	return "", err
}

// Reason runs a one-liner and answers with its last words on stderr, or its
// exit status where it left none. It is for a failure somebody reads as a
// sentence on the page they are on.
func (r Runner) Reason(s string, env Env) error {
	_, said, err := r.say(s, env)
	switch {
	case err == nil:
		return nil
	case said != "":
		return errors.New(said)
	}
	return err
}

// Guard runs a module's shell whose exit status is an answer: may this module
// open, does it have this action, may the work begin. What it says on stderr
// with a no is what somebody reads.
func (r Runner) Guard(s string, env Env) error {
	_, said, err := r.ask(guard, s, env)
	switch {
	case err == nil:
		return nil
	case said != "":
		return errors.New(said)
	}
	return err
}

// say runs a one-liner and keeps its two channels apart: what it printed, and
// what it said on the way out.
func (r Runner) say(s string, env Env) (out, said string, err error) {
	return r.ask(snippet, s, env)
}

// ask is that, under whichever wrapper the caller's shell is written to.
func (r Runner) ask(wrapper, s string, env Env) (out, said string, err error) {
	cmd := bash(r.args(wrapper, s))
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	return strings.TrimRight(stdout.String(), "\n"), strings.TrimSpace(stderr.String()), err
}

// Lines runs a command and returns its stdout one entry per line, blank lines
// dropped. Only the end of a line is trimmed: a leading tab follows an empty
// value, which is an answer too.
func (r Runner) Lines(s string, env Env) ([]string, error) {
	out, err := r.Run(s, env)
	if err != nil {
		return nil, err
	}
	return Lines(out), nil
}

// Lines is that rule on its own, for output that came back from somewhere else.
func Lines(out string) []string {
	var lines []string
	for l := range strings.SplitSeq(out, "\n") {
		l = strings.TrimRight(l, " \t\r")
		if strings.TrimSpace(l) == "" {
			continue
		}
		lines = append(lines, l)
	}
	return lines
}

// Session is a running script. Everything it prints goes to the log; what it
// said on stderr is kept as well, because that is what a failure report reads.
type Session struct {
	mu     sync.Mutex
	stderr []string
	latest string
	done   chan struct{}
	err    error
	cmd    *exec.Cmd
	run    Runner
	step   Step
}

// Start runs one script in the background, named in any failure the way step
// names it.
func (r Runner) Start(step Step, env Env) (*Session, error) {
	cmd, report, err := r.command(step, env)
	if err != nil {
		return nil, err
	}
	s := &Session{done: make(chan struct{}), cmd: cmd, run: r, step: step}

	// The log gets the raw bytes of both channels, the failure report sanitized
	// stderr. Each channel is followed for the line it last drew on a writer of
	// its own, since the two are copied on two goroutines.
	errw := &sessionWriter{sink: s.writeErr}
	cmd.Stdout = io.MultiWriter(logging.External(), &progressWriter{sink: s.setLatest})
	cmd.Stderr = io.MultiWriter(logging.External(), errw, &progressWriter{sink: s.setLatest})

	if err := cmd.Start(); err != nil {
		drain(cmd, report)
		return nil, err
	}
	go func() {
		raw := drain(cmd, report)
		err := cmd.Wait()
		errw.flush()
		s.err = s.failure(err, raw)
		close(s.done) // only after every line has been collected
	}()
	return s, nil
}

// command builds the invocation of a script, with the ERR trap's channel
// attached as fd 3. The returned reader yields the trap's report; the caller
// closes the write end right after starting (see drain).
func (r Runner) command(step Step, env Env) (*exec.Cmd, *os.File, error) {
	wrapper, payload := wrap(step)
	cmd := bash(r.args(wrapper, payload))
	cmd.Env = env
	// A process group of its own, so stopping a stage also stops the package
	// manager and the build it started rather than leaving them writing to the
	// target disk.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	rd, w, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	cmd.ExtraFiles = []*os.File{w}
	return cmd, rd, nil
}

// Terminal builds the invocation of a script that is handed the terminal, to
// run in place of the interface. It is no Session: nothing is captured or
// logged, and only the exit code comes back.
func (r Runner) Terminal(script Script, env Env) *Handover {
	cmd := bash(r.args(handover, script.shell()))
	cmd.Env = env
	return &Handover{cmd: cmd, tty: controllingTerminal}
}

// drain closes this side of the write end - without it, reading the report
// blocks until the child exits even though the trap already wrote - and returns
// what the trap reported.
func drain(cmd *exec.Cmd, r *os.File) string {
	for _, f := range cmd.ExtraFiles {
		f.Close()
	}
	defer r.Close()
	raw, _ := io.ReadAll(r)
	return string(raw)
}

// failure turns an exit status into the one error shape, filling in whatever
// the trap managed to report.
func (s *Session) failure(err error, report string) error {
	if err == nil {
		return nil
	}
	return s.run.failure(s.step, err, report, s.lastErr())
}

// Done closes once the script has exited and all its output is collected.
func (s *Session) Done() <-chan struct{} { return s.done }

// Err holds the result, valid once Done is closed.
func (s *Session) Err() error { return s.err }

// Kill stops the script and everything it started by signalling its whole
// process group. ctrl+c needs it during a stage: a package transaction left
// writing to an unwatched disk is worse than an interrupted one.
func (s *Session) Kill() {
	if s.cmd == nil || s.cmd.Process == nil {
		return
	}
	if pgid, err := syscall.Getpgid(s.cmd.Process.Pid); err == nil {
		syscall.Kill(-pgid, syscall.SIGKILL)
		return
	}
	s.cmd.Process.Kill()
}

// writeErr keeps the tail of stderr, which is where a failing tool says why.
func (s *Session) writeErr(line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if line = strings.TrimSpace(sanitize(line)); line == "" {
		return
	}
	s.stderr = append(s.stderr, line)
	if len(s.stderr) > stderrKeep {
		s.stderr = s.stderr[len(s.stderr)-stderrKeep:]
	}
}

// Latest is the line the script drew last on either channel, shown under a task
// that declared its output its progress. A carriage return ends a line too,
// since a progress bar redraws itself in place with one.
func (s *Session) Latest() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.latest
}

func (s *Session) setLatest(line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.latest = line
}

func (s *Session) lastErr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Join(s.stderr, "\n")
}

// sanitize strips what a line must not carry into the frame it is rendered in:
// raw colour and cursor sequences would corrupt the interface.
func sanitize(line string) string {
	line = ansi.Strip(line)
	return strings.Map(func(r rune) rune {
		if r == '\t' {
			return r
		}
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, line)
}

type sessionWriter struct {
	sink func(string)
	buf  []byte
}

func (w *sessionWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		w.sink(string(bytes.TrimRight(w.buf[:i], "\r")))
		w.buf = w.buf[i+1:]
	}
	return len(p), nil
}

// progressWriter hands on the last line a channel has drawn, the unfinished one
// included, sanitized and never empty. It keeps only what follows the last line
// ending, so output without one is not held.
type progressWriter struct {
	sink func(string)
	buf  []byte
}

// progressKeep is how much of an unfinished line is held on to.
const progressKeep = 4096

func (w *progressWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	line := ""
	for _, part := range bytes.FieldsFunc(w.buf, func(r rune) bool { return r == '\r' || r == '\n' }) {
		if drawn := strings.TrimSpace(sanitize(string(part))); drawn != "" {
			line = drawn
		}
	}
	if line != "" {
		w.sink(line)
	}
	if i := bytes.LastIndexAny(w.buf, "\r\n"); i >= 0 {
		w.buf = w.buf[i+1:]
	}
	if len(w.buf) > progressKeep {
		w.buf = w.buf[len(w.buf)-progressKeep:]
	}
	return len(p), nil
}

// flush emits a trailing line that never got its newline.
func (w *sessionWriter) flush() {
	if len(w.buf) > 0 {
		w.sink(string(w.buf))
		w.buf = nil
	}
}
