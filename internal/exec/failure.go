package exec

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/murkl/oak/internal/i18n"
)

// stderrKeep is how many of the last stderr lines a Failure carries. The real
// message is usually the last thing a tool says before it gives up; more than
// that turns an error into a log dump.
const stderrKeep = 8

// Failure is what a script reports back when it dies, in one shape for every
// script there is or will be: the runtime never knows what a script does, so it
// asks bash where it broke rather than guessing from the output.
//
// What it carries is what somebody reading it has to have to go and find the
// line: which module, which step of it, which file and line, and what the tool
// itself said on the way out.
type Failure struct {
	Module  string // the module the step belongs to
	Unit    string // the step's name, filled in by the caller
	Hook    string // the hook it is a step of, empty for an ordinary task
	Script  string // the file the ERR trap fired in
	Line    int
	Code    int
	Command string // the command that failed
	Stderr  string
}

// Field is one row of a failure report: what it is, and what it says.
//
// Path marks the value whose end carries more than its start — the file and the
// line it broke on. A frame too narrow for it cuts the front rather than the
// back, because the leading folders are the one part somebody chasing it can
// work out for themselves and the line number is not.
type Field struct {
	Label string
	Value string
	Path  bool
}

// Fields renders the failure as those rows, so the frame can lay them out as a
// table rather than parse a sentence back apart. The labels are translated here
// because this is the one place that knows what each value is.
//
// A hook is named as one. It is a module's own code run at a moment the runtime
// chose, so a mistake in it is as much an authoring bug as one in a task — and
// the row that says which hook is the difference between a puzzle and a file to
// open.
func (f *Failure) Fields() []Field {
	var out []Field
	add := func(label, value string) {
		if value != "" {
			out = append(out, Field{Label: label, Value: value})
		}
	}
	// TRANSLATORS: the labels below head the table shown when a step failed:
	// the module it belongs to, the step itself, the script and line it was in,
	// what ran, and what it returned. Each is one column of a narrow table, so
	// short wins over exact.
	add(i18n.T("Module"), f.Module)
	if f.Hook != "" {
		add(i18n.T("Hook"), f.Hook)
		add(i18n.T("Step"), f.Unit)
	} else {
		add(i18n.T("Task"), f.Unit)
	}
	if f.Script != "" {
		// A script that simply returned non-zero broke on no line in
		// particular: there is a file to open and nothing to point at in it.
		where := f.Script
		if f.Line > 0 {
			where = fmt.Sprintf("%s:%d", f.Script, f.Line)
		}
		out = append(out, Field{Label: i18n.T("Script"), Value: where, Path: true})
	}
	add(i18n.T("Command"), f.Command)
	if f.Code != 0 {
		add(i18n.T("Exit code"), strconv.Itoa(f.Code))
	}
	return out
}

// Error renders the one format every failure is reported in.
func (f *Failure) Error() string {
	var b strings.Builder
	b.WriteString(i18n.T("%s failed", f.Unit))
	for _, kv := range f.Fields() {
		fmt.Fprintf(&b, "\n%-11s %s", kv.Label+":", kv.Value)
	}
	if f.Stderr != "" {
		fmt.Fprintf(&b, "\n%-11s %s", i18n.T("Error")+":", f.Stderr)
	}
	return b.String()
}

// parseReport reads the ERR trap's first line: code, source, line, command.
//
// A script can also die without the trap — a bare `exit 1` does not fire ERR —
// so a missing or unreadable report is normal and simply leaves the fields
// empty. The exit code alone still names the step that failed.
//
// Only the first line counts. A command substitution fails twice over — once in
// the subshell it ran in and once in the line that took its output — and the
// inner one is where the command actually is.
func parseReport(s string) *Failure {
	first, _, _ := strings.Cut(strings.TrimRight(s, "\n"), "\n")
	fields := strings.SplitN(first, "\t", 4)
	if len(fields) != 4 {
		return nil
	}
	code, err := strconv.Atoi(fields[0])
	if err != nil {
		return nil
	}
	line, err := strconv.Atoi(fields[2])
	if err != nil {
		return nil
	}
	return &Failure{Code: code, Script: short(fields[1]), Line: line, Command: fields[3]}
}

// short renders a script path the way whoever wrote it knows it — relative to
// the working dir. Absolute is right only when the path is outside it.
func short(p string) string {
	wd, err := os.Getwd()
	if err != nil {
		return p
	}
	rel, err := filepath.Rel(wd, p)
	if err != nil || strings.HasPrefix(rel, "..") {
		return p
	}
	return rel
}

// exitCode digs the status out of whatever error exec returned.
func exitCode(err error) int {
	type coder interface{ ExitCode() int }
	if e, ok := err.(coder); ok {
		return e.ExitCode()
	}
	return 1
}

// Fail wraps whatever comes back from a script that ran outside a Session — one
// that was handed the terminal — in the one shape failures are reported in.
// There is no trap report to fill in: what went wrong was on screen.
func (r Runner) Fail(step Step, err error) error {
	if err == nil {
		return nil
	}
	return &Failure{
		Module: r.Module, Unit: step.Name, Hook: step.Hook,
		Script: short(step.Script.File), Code: exitCode(err),
	}
}

// failure turns an exit status into that one shape, filling in whatever the
// trap managed to report and whatever the script said on its way out.
func (r Runner) failure(step Step, err error, report, said string) error {
	f := parseReport(report)
	if f == nil {
		// Nothing tripped the trap, so the script answered with a status of its
		// own. There is no line to name, but there is still the file it is in.
		f = &Failure{Code: exitCode(err), Script: short(step.Script.File)}
	}
	f.Module, f.Unit, f.Hook, f.Stderr = r.Module, step.Name, step.Hook, said
	return f
}

// lastWords is the tail of what a script said on stderr, which is where a
// failing tool says why. Everything above it is the tool working.
func lastWords(s string) string {
	var kept []string
	for line := range strings.SplitSeq(s, "\n") {
		if line = strings.TrimSpace(sanitize(line)); line != "" {
			kept = append(kept, line)
		}
	}
	if len(kept) > stderrKeep {
		kept = kept[len(kept)-stderrKeep:]
	}
	return strings.Join(kept, "\n")
}
