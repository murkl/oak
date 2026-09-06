package exec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var sh = Runner{}

func script(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stage.sh")
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func run(t *testing.T, body string) *Session {
	t.Helper()
	s, err := sh.Start("Test stage", script(t, body), Env(os.Environ()))
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

// The one shell idiom every stage script is written in. It leaves a non-zero
// status behind when the test is false, and it must not be mistaken for a
// failure — including on the very last line of a script, where the status
// propagates out of `source`.
func TestAGuardThatDoesNotFireIsNotAFailure(t *testing.T) {
	if err := run(t, "X=false\n[ \"$X\" = true ] && echo yes\necho after\n").Err(); err != nil {
		t.Fatalf("mid-script guard reported: %v", err)
	}
	if err := run(t, "X=false\n[ \"$X\" = true ] && echo yes\n").Err(); err != nil {
		t.Fatalf("trailing guard reported: %v", err)
	}
	if err := run(t, "if false; then echo no; fi\n").Err(); err != nil {
		t.Fatalf("if-block reported: %v", err)
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
	s, err := sh.Start("Test", script(t, "echo \"disk=$DISK\" >"+seen+"\n"), Env{"DISK=/dev/sda"})
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

	s, err := sh.Start("Test", script(t, body), Env(os.Environ()))
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
