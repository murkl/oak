package exec

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// A failure report is the most important text this program ever puts on screen,
// and the frame lays it out as a table — so what it holds is a contract. The
// first two rows are what somebody reading it has to have before anything else:
// which module, and which step of it.
func TestAFailureIsReportedAsLabelledFields(t *testing.T) {
	f := &Failure{
		Module: "Tux Setup", Unit: "Set up the disk",
		Script: "/module/tasks/disk/task.sh", Line: 12,
		Code: 2, Command: "mkfs.btrfs -f /dev/sda2", Stderr: "no such device",
	}
	got := f.Fields()
	want := [][2]string{
		{"Module", "Tux Setup"},
		{"Task", "Set up the disk"},
		{"Script", "/module/tasks/disk/task.sh:12"},
		{"Command", "mkfs.btrfs -f /dev/sda2"},
		{"Exit code", "2"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("field %d = %v, want %v", i, got[i], want[i])
		}
	}

	msg := f.Error()
	for _, fragment := range []string{"Set up the disk failed", "task.sh:12", "mkfs.btrfs", "no such device"} {
		if !strings.Contains(msg, fragment) {
			t.Errorf("the message does not mention %q:\n%s", fragment, msg)
		}
	}
}

// A hook is a module's own code run at a moment the runtime chose, so a mistake
// in one is an authoring bug like any other — and the report says which hook,
// because that is the folder to go and open.
func TestAFailureInAHookNamesTheHook(t *testing.T) {
	f := &Failure{
		Module: "Tux Setup", Hook: "@wlan-device", Unit: "Find the wireless device",
		Script: "/module/tasks/@wlan-device/station/task.sh", Line: 3, Code: 127,
	}
	want := [][2]string{
		{"Module", "Tux Setup"},
		{"Hook", "@wlan-device"},
		{"Step", "Find the wireless device"},
		{"Script", "/module/tasks/@wlan-device/station/task.sh:3"},
		{"Exit code", "127"},
	}
	got := f.Fields()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("field %d = %v, want %v", i, got[i], want[i])
		}
	}
}

// A script that died without the trap having anything to say still reports the
// step and the exit code, and says nothing it does not know.
func TestAFailureWithNoTrapReportSaysOnlyWhatItKnows(t *testing.T) {
	f := &Failure{Unit: "Restart", Code: 1}
	got := f.Fields()
	if len(got) != 2 || got[0][0] != "Task" || got[1][0] != "Exit code" {
		t.Fatalf("got %v, want the step and the exit code alone", got)
	}
}

// Fail is for a script that was handed the terminal: what went wrong was on
// screen, so there is no trap report to fill in — only the code it left.
func TestFailCarriesTheExitCodeOfAScriptThatOwnedTheTerminal(t *testing.T) {
	step := Step{Name: "Enter the new system"}
	if err := sh.Fail(step, nil); err != nil {
		t.Errorf("a script that worked reported %v", err)
	}

	cmd := exec.Command("sh", "-c", "exit 7")
	err := sh.Fail(step, cmd.Run())
	var f *Failure
	if !errors.As(err, &f) {
		t.Fatalf("got %T, want a *Failure", err)
	}
	if f.Unit != "Enter the new system" || f.Code != 7 || f.Module != sh.Module {
		t.Errorf("got %q in %q with code %d", f.Unit, f.Module, f.Code)
	}
}

// A hook runs step by step rather than as one piece of shell, so that a mistake
// in one of them names the step, the file and the line — and so that what the
// steps printed still comes back as one answer.
func TestAHookRunsItsStepsInOrderAndReportsWhichOneBroke(t *testing.T) {
	out, err := sh.Hook([]Step{
		{Name: "First", Hook: "@online", Script: sourced(t, "echo one\n")},
		{Name: "Second", Hook: "@online", Script: sourced(t, "echo two\n")},
	}, Env(os.Environ()))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if out != "one\ntwo" {
		t.Errorf("out = %q, want both lines", out)
	}

	_, err = sh.Hook([]Step{
		{Name: "Broken", Hook: "@online", Script: sourced(t, "echo fine\nls /definitely/not/here\n")},
	}, Env(os.Environ()))
	var f *Failure
	if !errors.As(err, &f) {
		t.Fatalf("got %T, want a *Failure", err)
	}
	if f.Hook != "@online" || f.Unit != "Broken" || f.Line != 2 {
		t.Errorf("got %q in %q at line %d, want the step and its line", f.Unit, f.Hook, f.Line)
	}
	// What the tool said, not how it said it: the wording is the machine's
	// locale and the path is the only part of it this test owns.
	if !strings.Contains(f.Stderr, "/definitely/not/here") {
		t.Errorf("stderr = %q, want what the tool said", f.Stderr)
	}
}
