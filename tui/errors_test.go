package tui

import (
	"path/filepath"
	"testing"
)

// A module whose second task stops itself with a status, and a row for a run
// that failed: what a failure offers, which words of the module's never do.
func refusedRun(t *testing.T, script, errors string) *harness {
	t.Helper()
	h := newHarness(t, map[string]string{
		treeFile:                        testInstaller + "rules:\n  on-failure: [share-log]\nerrors:\n  4: |\n    The disk is gone\n\n    Plug {{DISK}} back in and choose Start again.\n",
		"actions/share-log/action.yaml": "title: Share the log\n",
		"actions/share-log/action.sh":   "true\n",
		"tasks/@go/b-second/task.yaml":  "title: Second\n" + errors,
		"tasks/@go/b-second/task.sh":    script,
	})
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter().typeIn("x").enter().typeIn("x").enter().yes()
	return h.ran()
}

// A status the task has words for stops the run on them, and on nothing else:
// no table of where it broke, no row to share the log, no question on the way
// back to the answers.
func TestAStatusATaskHasWordsForStopsTheRunOnThem(t *testing.T) {
	h := refusedRun(t, "echo 'curl: (6) Could not resolve host' >&2\nexit 3\n",
		"errors:\n  3: |\n    No internet connection\n\n    Connect this machine and choose Start again.\n")
	h.wants("No internet connection", "Connect this machine and choose Start again.").
		refuses("Failed", "Script", "Exit code", "Could not resolve")

	h.enter().wants("Setup").refuses("Share the log", "Back to the menu")
}

// The module's words cover a status the task has none for, filled in from the
// answers.
func TestTheModulesWordsCoverAStatusTheTaskHasNoneFor(t *testing.T) {
	h := refusedRun(t, "exit 4\n", "")
	h.wants("The disk is gone", "Plug /dev/sda back in").refuses("Failed", "Exit code")
}

// A status nobody has words for is a failure like any other: whoever changes
// the script is the one who can do something about it.
func TestAStatusNobodyHasWordsForIsAFailure(t *testing.T) {
	h := refusedRun(t, "exit 5\n", "errors:\n  3: No internet connection\n")
	h.wants("Failed", "It stopped at Second")
	h.enter().wants("Exit code", "5", "Share the log")
}

// A command that failed is where the script broke, even with the status of one
// that has words: the script did not choose it.
func TestACommandThatFailedIsAFailureWhateverItsStatus(t *testing.T) {
	h := refusedRun(t, "ls /definitely/not/here\n", "errors:\n  2: No such folder\n")
	h.wants("Failed").refuses("No such folder")
	h.enter().wants("Command", "not/here", "Share the log")
}

// An optional task stopped on purpose reads the same behind the test results.
func TestAnOptionalTaskStoppedOnPurposeReadsAsItsWords(t *testing.T) {
	h := installed(t, map[string]string{
		"tasks/@go/a-first/task.yaml": "title: First\nallow-failure: true\nerrors:\n  3: The AUR is out of reach\n",
		"tasks/@go/a-first/task.sh":   "exit 3\n",
	})
	h.up().enter().wants("1 of 1 optional tasks failed", "First")
	h.enter().wants("The AUR is out of reach").refuses("Exit code", "Script")
}

// An action that stops itself with a status it has words for says them, and
// opens nothing: what it opens on failure puts right a no, not this.
func TestAnActionStoppedOnPurposeSaysSoAndOpensNothing(t *testing.T) {
	tree := wireless(filepath.Join(t.TempDir(), "online"), "exit 0", false)
	tree["actions/wlan/action.yaml"] += "errors:\n  3: |\n    The wireless card is switched off\n\n    Switch it on and try again.\n"
	tree["actions/wlan/action.sh"] = "exit 3\n"
	h := newHarness(t, tree)
	intoHub(h)
	toAction(h, "Wireless network").enter().enter()
	h.wants("The wireless card is switched off", "Switch it on and try again.").
		refuses("Passphrase", "Exit code", "action.sh")

	h.enter().wants("Network")
}
