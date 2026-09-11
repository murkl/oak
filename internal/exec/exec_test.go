package exec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var sh = Runner{Module: "Test Module"}

// script writes a task's file and answers with the shell that runs it.
func script(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stage.sh")
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// sourced is that file as one task's work.
func sourced(t *testing.T, body string) Script { return Script{File: script(t, body)} }

// run is a script in a file, the way a task with a task.sh beside it runs.
func run(t *testing.T, body string) *Session {
	t.Helper()
	return start(t, sourced(t, body))
}

// start is one task's work, whichever of the two shapes it was written in.
func start(t *testing.T, sc Script) *Session {
	t.Helper()
	s, err := sh.Start(Step{Name: "Test stage", Script: sc}, Env(os.Environ()))
	if err != nil {
		t.Fatal(err)
	}
	<-s.Done()
	return s
}

func TestAScriptThatWorksReportsNothing(t *testing.T) {
	if err := run(t, "echo working\n").Err(); err != nil {
		t.Fatalf("err = %v", err)
	}
}

// The whole point of the ERR trap: a failure names the file, the line, the
// command and the exit code, without the runtime knowing anything about what
// the script was doing.
func TestAFailureSaysExactlyWhereItBroke(t *testing.T) {
	s := run(t, "echo first\nls /definitely/not/here\necho never\n")
	f, ok := s.Err().(*Failure)
	if !ok {
		t.Fatalf("err = %v (%T), want a *Failure", s.Err(), s.Err())
	}
	if f.Line != 2 {
		t.Errorf("line = %d, want 2", f.Line)
	}
	if !strings.HasSuffix(f.Script, "stage.sh") {
		t.Errorf("script = %q", f.Script)
	}
	if f.Command != "ls /definitely/not/here" {
		t.Errorf("command = %q", f.Command)
	}
	if f.Code == 0 {
		t.Errorf("code = 0, want the command's own")
	}
	if !strings.Contains(f.Stderr, "not/here") {
		t.Errorf("stderr = %q, want what the tool said", f.Stderr)
	}
	if f.Unit != "Test stage" {
		t.Errorf("unit = %q", f.Unit)
	}
}

// Shell a yaml wrote outright has no file to point at, so the report is what
// broke and what it returned — and that much still arrives.
func TestShellWithNoFileStillNamesWhatBroke(t *testing.T) {
	s := start(t, Script{Shell: "echo first\nls /definitely/not/here\necho never\n"})
	f, ok := s.Err().(*Failure)
	if !ok {
		t.Fatalf("err = %v (%T), want a *Failure", s.Err(), s.Err())
	}
	if f.Script != "" {
		t.Errorf("script = %q, want none — there is no file", f.Script)
	}
	if f.Command != "ls /definitely/not/here" {
		t.Errorf("command = %q", f.Command)
	}
	if f.Code == 0 {
		t.Errorf("code = 0, want the command's own")
	}
	if err := start(t, Script{Shell: "ls /definitely/not/here\n"}).Err(); err == nil {
		t.Error("a one-line script that failed was not reported")
	}
}

// A bare exit does not fire ERR, so there is no report to read — and the
// failure still has to name the step and the code.
func TestABareExitIsStillAFailure(t *testing.T) {
	s := run(t, "echo giving up >&2\nexit 3\n")
	f, ok := s.Err().(*Failure)
	if !ok {
		t.Fatalf("err = %v, want a *Failure", s.Err())
	}
	if f.Code != 3 {
		t.Errorf("code = %d, want 3", f.Code)
	}
	if !strings.Contains(f.Stderr, "giving up") {
		t.Errorf("stderr = %q", f.Stderr)
	}
}

// A guard is a guard wherever it is not the last thing a script does: what it
// leaves behind is only ever read at the end.
func TestAGuardInTheMiddleOfAScriptIsNotAFailure(t *testing.T) {
	if err := run(t, "X=false\n[ \"$X\" = true ] && echo yes\necho after\n").Err(); err != nil {
		t.Fatalf("mid-script guard reported: %v", err)
	}
	if err := run(t, "if false; then echo no; fi\n").Err(); err != nil {
		t.Fatalf("if-block reported: %v", err)
	}
}

// On the last line it is the script's answer, like any other. A script that
// means to end there ends on the work, on an `if` block or on an `echo` — the
// alternative is a status nobody meant, read as though somebody had.
func TestAGuardOnTheLastLineIsTheScriptsAnswer(t *testing.T) {
	f, ok := run(t, "X=false\n[ \"$X\" = true ] && echo yes\n").Err().(*Failure)
	if !ok {
		t.Fatal("a trailing guard that did not fire was taken as a pass")
	}
	if f.Line != 2 || f.Command != `[ "$X" = true ]` {
		t.Errorf("got %s:%d running %q, want the guard's own line", f.Script, f.Line, f.Command)
	}
}

// Every script is judged by its status: a command that failed, or whatever it
// handed back at the end. One rule, and the same one for a task, for its test
// and for every step of a hook.
func TestAScriptIsJudgedByItsStatus(t *testing.T) {
	answers := func(sc Script) error {
		t.Helper()
		s, err := sh.Start(Step{Name: "Test", Script: sc}, Env(os.Environ()))
		if err != nil {
			t.Fatal(err)
		}
		<-s.Done()
		return s.Err()
	}
	if err := answers(sourced(t, "return 1\n")); err == nil {
		t.Error("a file answering 1 was taken as a pass")
	}
	if err := answers(Script{Shell: "return 1"}); err == nil {
		t.Error("shell answering 1 was taken as a pass")
	}
	if err := answers(sourced(t, "return 0\n")); err != nil {
		t.Errorf("a file answering 0 reported %v", err)
	}
	if err := answers(Script{Shell: `[ 1 = 2 ] && echo no`}); err == nil {
		t.Error("a guard that did not fire was taken as a pass")
	}
}

// And it still reports where it broke.
func TestAScriptStillNamesWhereItBroke(t *testing.T) {
	s, err := sh.Start(Step{
		Name:   "Test",
		Script: sourced(t, "echo looking\nls /definitely/not/here\n"),
	}, Env(os.Environ()))
	if err != nil {
		t.Fatal(err)
	}
	<-s.Done()
	f, ok := s.Err().(*Failure)
	if !ok {
		t.Fatalf("err = %v (%T), want a *Failure", s.Err(), s.Err())
	}
	if f.Line != 2 {
		t.Errorf("line = %d, want 2", f.Line)
	}
}

// The module's shell is loaded, not run. A lookup in it that tries one thing
// and falls back to another is ordinary shell — and under the trap every such
// fallback wrote a report, which the unit about to run was then blamed for,
// naming a line of somebody else's file.
func TestWhatTheModuleShellRecoversFromIsNotTheUnitsFailure(t *testing.T) {
	shell := script(t, "lib_recovered=$(grep nothing /dev/null || true)\n")
	// The same shape the real one had: a pipeline whose first command finds
	// nothing, under pipefail, inside a substitution the shell goes on from.
	noisy := Runner{Module: "Test Module", Shell: script(t,
		`: "${found:=$(grep nothing /dev/null | tail -n1)}"`+"\n")}

	for _, r := range []Runner{{Module: "Test Module", Shell: shell}, noisy} {
		s, err := r.Start(Step{Name: "Test", Script: sourced(t, "return 1\n")}, Env(os.Environ()))
		if err != nil {
			t.Fatal(err)
		}
		<-s.Done()
		f, ok := s.Err().(*Failure)
		if !ok {
			t.Fatalf("err = %v (%T), want a *Failure", s.Err(), s.Err())
		}
		// The test's own line, not a line of the shell it was given.
		if f.Line != 1 || f.Command != "return 1" {
			t.Errorf("blamed on %s:%d running %q, want the test's own line", f.Script, f.Line, f.Command)
		}
		if strings.Contains(f.Script, "lib") {
			t.Errorf("script = %q, want the file the test is in", f.Script)
		}
	}
}

// And a shell that will not load at all is still the unit's failure, with
// whatever it said on the way out.
func TestAModuleShellThatWillNotLoadFailsTheUnit(t *testing.T) {
	r := Runner{Module: "Test Module", Shell: script(t, "echo broken >&2\nexit 3\n")}
	s, err := r.Start(Step{Name: "Test", Script: sourced(t, "echo never\n")}, Env(os.Environ()))
	if err != nil {
		t.Fatal(err)
	}
	<-s.Done()
	f, ok := s.Err().(*Failure)
	if !ok {
		t.Fatalf("err = %v (%T), want a *Failure", s.Err(), s.Err())
	}
	if f.Code != 3 || !strings.Contains(f.Stderr, "broken") {
		t.Errorf("got code %d saying %q, want 3 and what the shell said", f.Code, f.Stderr)
	}
}

// A script can say no without any command having failed, and the trap sees
// nothing then. `return 1` is one way; a guard that does not fire is the other,
// and both are how an assertion is actually written. The line has to come back
// either way — it is the one thing somebody reading the report came for.
func TestAScriptThatSaysNoWithoutFailingStillNamesTheLine(t *testing.T) {
	says := func(body string) *Failure {
		t.Helper()
		s, err := sh.Start(Step{Name: "Test", Script: sourced(t, body)}, Env(os.Environ()))
		if err != nil {
			t.Fatal(err)
		}
		<-s.Done()
		f, ok := s.Err().(*Failure)
		if !ok {
			t.Fatalf("err = %v (%T), want a *Failure", s.Err(), s.Err())
		}
		return f
	}
	for _, tc := range []struct{ body, command string }{
		{"# a comment\n\nreturn 1\n", "return 1"},
		{"# a comment\n\n[ -f /definitely/not/here ] && grep -q x /etc/hostname\n", "[ -f /definitely/not/here ]"},
	} {
		f := says(tc.body)
		if f.Line != 3 || f.Command != tc.command {
			t.Errorf("got %s:%d running %q, want line 3 running %q", f.Script, f.Line, f.Command, tc.command)
		}
	}
}

// The fallback is only for a script that said no without anything failing. A
// command that really did fail is reported where that command is — inside the
// module's shell, where a function it called lives — and naming the call site
// instead would be pointing away from the line somebody has to open.
func TestAFailureInsideTheModuleShellIsReportedThere(t *testing.T) {
	r := Runner{Module: "Test Module", Shell: script(t, "no_thanks() { ls /definitely/not/here; }\n")}
	own := sourced(t, "# a comment\nno_thanks\n")
	s, err := r.Start(Step{Name: "Test", Script: own}, Env(os.Environ()))
	if err != nil {
		t.Fatal(err)
	}
	<-s.Done()
	f, ok := s.Err().(*Failure)
	if !ok {
		t.Fatalf("err = %v (%T), want a *Failure", s.Err(), s.Err())
	}
	if f.Command != "ls /definitely/not/here" || strings.HasSuffix(own.File, f.Script) {
		t.Errorf("got %s:%d running %q, want the line the command is on", f.Script, f.Line, f.Command)
	}
}

func TestAFailingPipelineIsAFailure(t *testing.T) {
	if err := run(t, "ls /definitely/not/here | cat\n").Err(); err == nil {
		t.Fatal("a failing pipeline was not reported")
	}
}

// What a script said is read inside the frame, so it must never arrive with
// escape sequences in it: a package manager's progress bar would tear the
// interface apart.
func TestOutputIsStrippedOfControlSequences(t *testing.T) {
	s := run(t, "printf '\\033[31mred\\033[0m and \\007plain\\n' >&2\nexit 1\n")
	f, ok := s.Err().(*Failure)
	if !ok {
		t.Fatalf("err = %v, want a *Failure", s.Err())
	}
	if f.Stderr != "red and plain" {
		t.Errorf("stderr = %q, want it sanitized", f.Stderr)
	}
}

func TestLinesKeepsTheTabThatSeparatesAValueFromItsLabel(t *testing.T) {
	got, err := sh.Lines("printf '/dev/sda\\t/dev/sda 1TB\\n\\n\\tStandard\\n  \\n'", nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/dev/sda\t/dev/sda 1TB", "\tStandard"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("lines = %q, want %q", got, want)
	}
}

func TestRunReturnsWhatACommandPrinted(t *testing.T) {
	got, err := sh.Run("echo Europe/Berlin", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Europe/Berlin" {
		t.Errorf("out = %q", got)
	}
	if _, err := sh.Run("echo nope >&2; exit 1", nil); err == nil {
		t.Error("a failing command was not reported")
	}
}

// The environment a script sees is the environment it is handed, and nothing
// else — this is how every answer reaches every task.
func TestAScriptSeesTheEnvironmentItWasGiven(t *testing.T) {
	seen := filepath.Join(t.TempDir(), "seen")
	s, err := sh.Start(Step{Name: "Test", Script: sourced(t, "echo \"disk=$DISK\" >"+seen+"\n")}, Env{"DISK=/dev/sda"})
	if err != nil {
		t.Fatal(err)
	}
	<-s.Done()
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(seen)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(got)) != "disk=/dev/sda" {
		t.Errorf("the script saw %q", got)
	}
}

// A stage is one line of shell that runs a package manager that runs a build.
// Killing only the shell would leave all of that writing to the target disk
// after the program itself is gone.
func TestKillingAStageTakesEverythingItStartedWithIt(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "still-running")
	// A grandchild that outlives its parent unless the whole group is signalled:
	// the shell exits at once, the sleep would not.
	body := "(sleep 5; touch " + marker + ") &\nsleep 5\n"

	s, err := sh.Start(Step{Name: "Test", Script: sourced(t, body)}, Env(os.Environ()))
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	s.Kill()

	select {
	case <-s.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("the session never finished after being killed")
	}
	time.Sleep(600 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("something the stage started outlived it")
	}
}

// Reason is for the shell whose failure is a sentence somebody reads on the page
// they are standing on — a configuration that could not be fetched — rather than
// a report of where a task broke. What the script said is the whole of it.
func TestReasonAnswersWithWhatTheScriptSaid(t *testing.T) {
	err := sh.Reason(`echo "Nothing is shared under that code" >&2; exit 1`, Env(os.Environ()))
	if err == nil {
		t.Fatal("a script that failed reported nothing")
	}
	if err.Error() != "Nothing is shared under that code" {
		t.Errorf("err = %q, want the script's own last words and nothing else", err)
	}
}

// A script that fails without saying anything still has to be reported, and
// then the exit status is all there is.
func TestReasonFallsBackToTheExitStatusWhenNothingWasSaid(t *testing.T) {
	err := sh.Reason("exit 3", Env(os.Environ()))
	if err == nil {
		t.Fatal("a script that failed reported nothing")
	}
	if !strings.Contains(err.Error(), "3") {
		t.Errorf("err = %q, want the exit status in it", err)
	}
}

func TestReasonIsSilentWhenTheScriptWorks(t *testing.T) {
	if err := sh.Reason("echo fine", Env(os.Environ())); err != nil {
		t.Errorf("err = %v, want nothing", err)
	}
}
