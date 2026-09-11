// Package exec is the only place this program starts a process.
//
// Everything the runtime actually does is shell: a stage's script, a variable's
// option list, the preflight check. The runtime feeds them variables and reads
// back an exit code or stdout — it never knows what any of them do.
package exec

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"

	"github.com/murkl/oak/internal/logging"

	"github.com/charmbracelet/x/ansi"
)

// Env is the variable set handed to every script, as KEY=value entries.
type Env []string

// Runner starts scripts, each wrapped in the same failure-reporting trap.
//
// Shell is what the module hands to everything it runs, in front of that
// script's own text — one place for what several of them share, and the
// functions its yaml calls by name. The runtime never reads it and has no idea
// what is in it; it only makes sure everything it starts gets the same one.
//
// Module is that module's name, and it is here rather than at every call site
// because every failure this package builds carries it: a run has one module in
// it, and which one is the first thing somebody reading a failure needs.
type Runner struct {
	Shell  string
	Module string
}

// Script is one task's work, in the shape its module wrote it: a file to
// source, or shell its yaml wrote outright. Exactly one of the two is set.
//
// Which it is travels with it because the two are not run the same way. A
// file's own last status propagates out of `source` and is not a failure;
// shell written outright runs at the shell's own level, where there is no file
// for a failure to point at and nothing for a status to propagate out of.
type Script struct {
	File  string
	Shell string
}

// Step is one piece of a module's work as this layer takes it: the shell
// itself, what a failure calls it, and the hook it belongs to where it is one.
//
// A hook is run step by step rather than as one piece of shell for exactly that
// reason — a mistake in one of them has to name the step it is in, the file and
// the line, the same way a task's does.
type Step struct {
	Name   string
	Hook   string
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

// The ERR trap of the two wrappers below reports on file descriptor 3 — the
// first entry of cmd.ExtraFiles. A dedicated descriptor keeps the report out of
// the script's own output, so a script may print anything at all without being
// mistaken for a failure report.
//
// The trap is the single detector, and -e is deliberately not set: -e would also
// kill the shell when `source` merely returns the status of a benign last line —
// `[ "$X" = true ] && do_it` leaves status 1 when the test is false.
//
// It reports on the failure's own terms: BASH_SOURCE and LINENO point into the
// script, BASH_COMMAND is the command that failed.
const strict = `set -Eo pipefail
`

// strictTrace is the same with the DEBUG trap carried into everything the
// script runs, which is what lets lastLine see past a function call.
const strictTrace = `set -Eo pipefail -T
`

// The two arguments every invocation is given: the script to run, and the
// module's own shell to put in front of it. Sourcing that here is what lets a
// script be plain shell with no preamble at all, and what puts the module's
// functions within reach of everything — its tasks, and the shell its yaml
// wrote.
//
// It is **loaded, not run**, and that is why the trap is installed after it
// rather than before. A lookup that tries one thing and falls back to another
// is ordinary shell, and under the trap every such fallback writes a report —
// which the unit about to run would then be blamed for, naming a line of
// somebody else's file.
//
// The one failure that is the module's own is a shell that will not load at
// all, and that is caught here, with whatever it said on the way out.
const preamble = `if [ -n "$2" ]; then source "$2" || exit $?; fi
`

// The trap, in its two shapes. A file names the file and the line it broke in;
// shell a yaml wrote outright has no file, so the report names the command.
//
// The empty-BASH_SOURCE check drops the status a failing `source` propagates
// back to the wrapper: everything inside the file names the file it is in, and
// this level names nothing.
const (
	fileTrap = `trap 'c=$?; s=${BASH_SOURCE[0]}; [ -n "$s" ] && { printf "%d\t%s\t%d\t%s\n" "$c" "$s" "$LINENO" "$BASH_COMMAND" >&3; exit $c; }' ERR
`
	shellTrap = `trap 'c=$?; printf "%d\t\t%d\t%s\n" "$c" "$LINENO" "$BASH_COMMAND" >&3; exit $c' ERR
`
)

// lastLine remembers where in the script the shell was, for the report a script
// that says no without anything having failed would otherwise not produce.
//
// It records the script's own lines and nothing else: a function it called out
// of the module's shell is where that function is, not where the script said
// no. `set -T` is what carries the trap into everything the script runs.
const lastLine = `trap '[ "${BASH_SOURCE[0]}" = "$1" ] && { oak_line=$LINENO; oak_cmd=$BASH_COMMAND; }; :' DEBUG
`

// The two wrappers: a script in a file, and shell a yaml wrote outright.
//
// Both answer with the script's own exit status, and both run under the trap.
// So a script fails on any command that fails, and again on whatever it hands
// back at the end — `exit 1`, `return 1`, or a last line that simply did not
// work. One rule, and the same one for a task, for its test and for every step
// of a hook.
//
// A script can say no without any command having failed: `return 1` and a guard
// that does not fire both look like that, and the trap sees neither. The file
// wrapper therefore reports the last line the script was on where the trap has
// nothing to report, so a failure always names a line.
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

// snippet runs a short piece of shell the yaml wrote inline — an option list, a
// prefill. No ERR trap: the caller wants the exit code or the output, and a
// non-zero status is an answer rather than a failure.
const snippet = preamble + `eval "$1"`

// handover runs a script that takes the terminal over. No trap and no pipes:
// what it does is a session somebody is sitting in front of, so its output is
// the terminal's and its exit code is the whole of what comes back.
const handover = preamble + `eval "$1"`

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

// Reason runs a one-liner and answers with what it said went wrong — the
// script's own last words on stderr, where it left any, and the exit status
// where it did not.
//
// For the shell whose failure is a sentence somebody reads on the page they are
// standing on rather than a report of where a task broke: "nothing is shared
// under that code" is the whole of what is worth saying, and an exit status in
// front of it only gets in the way.
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

// say runs a one-liner and keeps its two channels apart: what it printed, and
// what it said on the way out.
func (r Runner) say(s string, env Env) (out, said string, err error) {
	cmd := exec.Command("bash", "-c", snippet, "--", s, r.Shell)
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	return strings.TrimRight(stdout.String(), "\n"), strings.TrimSpace(stderr.String()), err
}

// Lines runs a command and returns its stdout, one entry per line, with blank
// lines dropped.
//
// Only the end of a line is trimmed. What is in front of the first character is
// the caller's business: a line may begin with a tab that separates a value
// from the text it is chosen by, and an empty value in front of that tab is a
// real answer — "no variant", "the default" — not an empty line.
func (r Runner) Lines(s string, env Env) ([]string, error) {
	out, err := r.Run(s, env)
	if err != nil {
		return nil, err
	}
	return Lines(out), nil
}

// Lines is that rule on its own, for output that came back from somewhere else
// — a hook answering with the networks in range.
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
	done   chan struct{}
	err    error
	cmd    *exec.Cmd
	run    Runner
	step   Step
}

// Hook runs the steps of one hook in order and hands back everything they
// printed, as one block.
//
// Unlike the one-liners above it runs under the ERR trap: a hook is a module's
// own code, and a mistake in it is an authoring bug that has to name the file
// and the line rather than an exit status nobody can place. Its output is the
// answer the runtime asked for — a device name, the networks in range — so it
// is captured rather than logged.
func (r Runner) Hook(steps []Step, env Env) (string, error) {
	var out []string
	for _, step := range steps {
		printed, said, report, err := r.trapped(step, env)
		if printed != "" {
			out = append(out, printed)
		}
		if err != nil {
			return "", r.failure(step, err, report, said)
		}
	}
	return strings.Join(out, "\n"), nil
}

// trapped runs one script under the ERR trap and keeps its three channels
// apart: what it printed, what it said on the way out, and where the trap says
// it broke.
func (r Runner) trapped(step Step, env Env) (printed, said, report string, err error) {
	cmd, rd, err := r.command(step, env)
	if err != nil {
		return "", "", "", err
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Start(); err != nil {
		drain(cmd, rd)
		return "", "", "", err
	}
	raw := drain(cmd, rd)
	err = cmd.Wait()
	return strings.TrimRight(stdout.String(), "\n"), lastWords(stderr.String()), raw, err
}

// Start runs one script in the background. unit names it in any failure, and
// hook the one it is a step of where it is one.
func (r Runner) Start(step Step, env Env) (*Session, error) {
	cmd, report, err := r.command(step, env)
	if err != nil {
		return nil, err
	}
	s := &Session{done: make(chan struct{}), cmd: cmd, run: r, step: step}

	// The log gets the raw bytes of both channels; the failure report gets
	// stderr, sanitized — see writeErr.
	errw := &sessionWriter{sink: s.writeErr}
	cmd.Stdout = logging.External()
	cmd.Stderr = io.MultiWriter(logging.External(), errw)

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
	cmd := exec.Command("bash", "-c", wrapper, "--", payload, r.Shell)
	cmd.Env = env
	// A process group of its own, so that stopping a stage stops everything it
	// started. A stage is one line of shell that runs a package manager that
	// runs a build; killing only the shell would leave all of that writing to
	// the target disk after the program itself is gone.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	rd, w, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	cmd.ExtraFiles = []*os.File{w}
	return cmd, rd, nil
}

// Terminal builds the invocation of a script that is handed the terminal, for
// the caller to run in place of the interface.
//
// It is deliberately not a Session: nothing is captured, nothing is logged, and
// there is no process group to kill — the user is at the keyboard, and what
// they see is what the script prints. All that comes back is the exit code.
func (r Runner) Terminal(script Script, env Env) *exec.Cmd {
	cmd := exec.Command("bash", "-c", handover, "--", script.shell(), r.Shell)
	cmd.Env = env
	return cmd
}

// drain closes this side of the write end — without it, reading the report
// blocks until the child exits even though the trap already wrote — and returns
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

// Kill stops the script and everything it started, by signalling the whole
// process group rather than the shell alone.
//
// It is the one thing ctrl+c has to do while a stage is running: leaving a
// package transaction writing to a disk that nobody is watching any more is
// worse than an interrupted one.
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

// flush emits a trailing line that never got its newline.
func (w *sessionWriter) flush() {
	if len(w.buf) > 0 {
		w.sink(string(w.buf))
		w.buf = nil
	}
}
