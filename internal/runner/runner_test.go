package runner

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/murkl/oak/internal/exec"
	"github.com/murkl/oak/internal/i18n"
	"github.com/murkl/oak/internal/spec"
	"github.com/murkl/oak/internal/store"
)

// setup writes a module: its declaration from the given variables, and its
// tasks in the one stage there is.
func setup(t *testing.T, variables string, tasks map[string]string) (*spec.Module, *store.Store, *Runner) {
	t.Helper()
	return setupWith(t, variables, "", tasks)
}

// setupWith is setup with lib as the product's oak.sh, which holds what the
// declaration calls.
func setupWith(t *testing.T, variables, lib string, tasks map[string]string) (*spec.Module, *store.Store, *Runner) {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{spec.FileModule: "title: T\nstages: [go]\n" + variables}
	if len(tasks) == 0 {
		tasks = oneTask
	}
	for id, yaml := range tasks {
		files["tasks/@go/"+id+"/task.yaml"] = yaml
		files["tasks/@go/"+id+"/task.sh"] = "echo ran\n"
	}
	for name, body := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sp := load(t, dir, lib)
	st := store.New(sp, filepath.Join(t.TempDir(), "installer.conf"), false)
	return sp, st, New(sp, st)
}

// load reads the module in dir, with lib written out as the product's oak.sh
// where it is not empty.
func load(t *testing.T, dir, lib string) *spec.Module {
	t.Helper()
	sp, err := spec.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if lib != "" {
		sp.Shell = filepath.Join(t.TempDir(), spec.FileRuntimeShell)
		if err := os.WriteFile(sp.Shell, []byte(lib), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return sp
}

var oneTask = map[string]string{"go": "title: Go\n"}

// A bool answers itself, in the interface's own language, so a folder never has
// to spell out what true and false are called.
func TestABoolOffersItsTwoAnswersInWords(t *testing.T) {
	i18n.Use("de", &i18n.Catalog{Messages: map[string]string{"Yes": "Ja", "No": "Nein"}})
	defer i18n.Use(i18n.SourceLang)

	sp, _, r := setup(t, "variables:\n  - name: X\n    title: X\n    type: bool\n", nil)
	got, err := r.Options(sp.Var("X"))
	if err != nil {
		t.Fatal(err)
	}
	want := []Option{{Value: "true", Label: "Ja"}, {Value: "false", Label: "Nein"}}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("options = %+v, want %+v", got, want)
	}
}

func TestAWrittenOutSetIsItsOwnLabel(t *testing.T) {
	sp, _, r := setup(t, "variables:\n  - name: FS\n    type: list\n    title: FS\n    options: [btrfs, ext4]\n", nil)
	got, err := r.Options(sp.Var("FS"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Value != "btrfs" || got[0].Label != "btrfs" {
		t.Errorf("options = %+v", got)
	}
}

// The one rule that lets a disk be stored as /dev/sda and chosen by its size.
func TestATabSeparatesTheValueStoredFromTheTextRead(t *testing.T) {
	sp, _, r := setupWith(t, `
variables:
  - name: DISK
    type: list
    title: Disk
    options-from: disks()
`, `disks() { printf '/dev/sda\t/dev/sda  1TB Samsung\n\tNone of these\nplain\n'; }`, nil)
	got, err := r.Options(sp.Var("DISK"))
	if err != nil {
		t.Fatal(err)
	}
	want := []Option{
		{Value: "/dev/sda", Label: "/dev/sda  1TB Samsung"},
		// An empty value in front of the tab is a real answer, not a blank line.
		{Value: "", Label: "None of these"},
		// No tab at all: the value is its own label.
		{Value: "plain", Label: "plain"},
	}
	if len(got) != len(want) {
		t.Fatalf("options = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("option %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// A suggestion is never worth stopping for.
func TestAPrefillThatFailsIsSimplyNoSuggestion(t *testing.T) {
	sp, _, r := setupWith(t, `
variables:
  - name: A
    type: text
    title: A
    prefill: zone()
  - name: B
    type: text
    title: B
    prefill: nothing()
`, "zone() { echo Europe/Berlin; }\nnothing() { return 1; }\n", nil)
	if got := r.Prefill(sp.Var("A")); got != "Europe/Berlin" {
		t.Errorf("prefill = %q", got)
	}
	if got := r.Prefill(sp.Var("B")); got != "" {
		t.Errorf("failed prefill = %q, want nothing", got)
	}
}

// The list is a promise of what is about to happen, so a task that has
// ruled itself out is not in it.
func TestTasksAreOnlyTheOnesThatWillRun(t *testing.T) {
	_, st, r := setup(t, "variables:\n  - name: DESKTOP\n    title: D\n    type: bool\n", map[string]string{
		"always":  "title: Always\n",
		"desktop": "title: Desktop\nconditions: DESKTOP == true\n",
	})
	if got := names(r.Tasks()); strings.Join(got, ",") != "Always" {
		t.Errorf("tasks = %v", got)
	}
	st.Set("DESKTOP", "true")
	if got := names(r.Tasks()); strings.Join(got, ",") != "Always,Desktop" {
		t.Errorf("tasks = %v", got)
	}
}

// acting is a module that names its actions the way head says, each action
// declared by its yaml and doing what its script does, loaded for a run started
// with or without --debug.
func acting(t *testing.T, head, lib string, actions map[string][2]string, debug bool) (*spec.Module, *store.Store, *Runner) {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		spec.FileModule:           "title: T\nstages: [go]\n" + head,
		"tasks/@go/run/task.yaml": "title: Go\n",
		"tasks/@go/run/task.sh":   "true\n",
	}
	for name, a := range actions {
		files["actions/"+name+"/action.yaml"] = a[0]
		files["actions/"+name+"/action.sh"] = a[1]
	}
	for name, body := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sp := load(t, dir, lib)
	st := store.New(sp, filepath.Join(t.TempDir(), "c"), debug)
	return sp, st, New(sp, st)
}

// opened runs an action to the end and answers with how it went.
func opened(t *testing.T, r *Runner, a *spec.Action) error {
	t.Helper()
	session, err := r.Open(a)
	if err != nil || session == nil {
		return err
	}
	<-session.Done()
	return session.Err()
}

// An action the work requires answers with its exit status alone: what it
// means is its fail, which the page standing in front of the work reads.
func TestARequiredActionAnswersWithItsExitStatus(t *testing.T) {
	says := func(script string) bool {
		t.Helper()
		sp, _, r := acting(t, "rules:\n  start-if: [uefi]\n", "", map[string][2]string{
			"uefi": {"title: UEFI\nerror: Set the boot mode to UEFI.\n", script},
		}, false)
		return r.Says(sp.Action("uefi"))()
	}
	if says("echo bios >&2\nexit 1\n") {
		t.Error("said yes, although the script said no")
	}
	if !says("return 0\n") {
		t.Error("said no, although the script said yes")
	}
}

// An action exists on a machine where what it requires says yes, and on every
// machine where it requires nothing.
func TestAnActionIsOfferedWhereWhatItRequiresSaysYes(t *testing.T) {
	offered := func(card string) bool {
		t.Helper()
		sp, _, r := acting(t, "rules:\n  on-settings: [wlan]\n", "", map[string][2]string{
			"wlan": {"title: Wireless\nrules:\n  offer-if: [card]\n", "true\n"},
			"card": {"title: Card\n", card},
		}, false)
		return r.Offered(sp.Action("wlan"))()
	}
	if offered("exit 1\n") {
		t.Error("offered, although what it requires said no")
	}
	if !offered("return 0\n") {
		t.Error("not offered, although what it requires said yes")
	}
}

// What the page was answered with is what the script is handed, under the
// name the page declared.
func TestAnActionIsHandedItsPage(t *testing.T) {
	out := filepath.Join(t.TempDir(), "said")
	sp, st, r := acting(t, "rules:\n  on-settings: [greet]\n", "", map[string][2]string{
		"greet": {"title: Greet\nvariables:\n  - name: GREETING\n    type: text\n    title: Greeting\n", "printf '%s' \"$GREETING\" > '" + out + "'\n"},
	}, false)
	st.Set("GREETING", "hello")
	if err := opened(t, r, sp.Action("greet")); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(out); string(got) != "hello" {
		t.Errorf("the script was handed %q, want the page's answer", got)
	}
}

// A script that breaks is reported the way a task that breaks is, and says it
// was an action.
func TestAFailingActionIsReportedAsOne(t *testing.T) {
	sp, _, r := acting(t, "rules:\n  on-settings: [greet]\n", "", map[string][2]string{
		"greet": {"title: Greet\n", "echo no network >&2\nexit 1\n"},
	}, false)
	err := opened(t, r, sp.Action("greet"))
	var f *exec.Failure
	if !errors.As(err, &f) {
		t.Fatalf("err = %v, want a failure report", err)
	}
	if !f.Action || f.Unit != "Greet" || !strings.Contains(f.Stderr, "no network") {
		t.Errorf("failure = %+v, want the action, its title and what it said", f)
	}
}

// A simulated run is read on somebody's own machine, which is neither the one
// the checks are about nor one to switch off: nothing required is asked,
// everything is offered, and nothing runs that did not say it simulates itself.
func TestASimulatedRunNeitherAsksNorRunsAnAction(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "ran")
	script := "touch '" + marker + "'\n"
	sp, _, r := acting(t, "rules:\n  start-if: [check]\n  on-settings: [restart]\n", "", map[string][2]string{
		"check":   {"title: Check\nerror: No.\n", "exit 1\n"},
		"restart": {"title: Restart\nrules:\n  offer-if: [check]\n", script},
	}, true)
	if !r.Offered(sp.Action("restart"))() || !r.Says(sp.Action("check"))() {
		t.Error("a simulated run was held to this machine")
	}
	if err := opened(t, r, sp.Action("restart")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("the action ran in a simulated run")
	}

	sp, _, r = acting(t, "rules:\n  on-settings: [restart]\n", "", map[string][2]string{
		"restart": {"title: Restart\nsimulates: true\n", script},
	}, true)
	if err := opened(t, r, sp.Action("restart")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Error("an action that simulates itself did not run")
	}
}

func TestStartRunsAnTaskAndReportsIt(t *testing.T) {
	sp, _, r := setup(t, "variables: []\n", map[string]string{
		"good": "title: Good\n",
		"bad":  "title: Bad\nneeds: [good]\n",
	})
	// The failing one is written over the script setup laid down for it.
	if err := os.WriteFile(string(sp.Tasks[1].Work()), []byte("echo why not >&2\nexit 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	good, err := r.Start(sp.Tasks[0])
	if err != nil {
		t.Fatal(err)
	}
	<-good.Done()
	if good.Err() != nil {
		t.Errorf("err = %v", good.Err())
	}

	bad, err := r.Start(sp.Tasks[1])
	if err != nil {
		t.Fatal(err)
	}
	<-bad.Done()
	if bad.Err() == nil {
		t.Fatal("a failing task was not reported")
	}
	if !strings.Contains(bad.Err().Error(), "why not") {
		t.Errorf("err = %q", bad.Err())
	}
}

func names(units []*spec.Task) []string {
	out := make([]string, len(units))
	for i, e := range units {
		out[i] = e.Title
	}
	return out
}

// An answer that changes the machine the installer is running on has to be put
// in force rather than only stored: a console keyboard nobody loaded is a
// layout nobody is typing on.
func TestApplyPutsAnAnswerInForce(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DIR", dir)
	sp, st, r := setupWith(t, `
variables:
  - name: KEYMAP
    type: text
    title: Keymap
    apply: load_keymap()
  - name: PLAIN
    type: text
    title: Plain
`, `load_keymap() { echo "$KEYMAP" > "${DIR}/loaded"; }`, nil)
	st.Set("KEYMAP", "de-latin1")
	r.Apply(sp.Var("KEYMAP"))
	loaded, err := os.ReadFile(filepath.Join(dir, "loaded"))
	if err != nil {
		t.Fatalf("nothing was applied: %v", err)
	}
	if strings.TrimSpace(string(loaded)) != "de-latin1" {
		t.Errorf("applied %q, want the answer that was just given", loaded)
	}
	// A variable that declares none is simply not applied, and saying so is
	// cheaper than a nil check at every call site.
	r.Apply(sp.Var("PLAIN"))
}

// An answer just given stands whether or not it could be put in force: an
// installer that stops because a keymap would not load is worse than one
// carrying on.
func TestAnApplyThatFailsIsOnlyAWarning(t *testing.T) {
	sp, st, r := setupWith(t, "variables:\n  - name: X\n    type: text\n    title: X\n    apply: refuse()\n", "refuse() { return 1; }\n", nil)
	st.Set("X", "value")
	if err := r.Apply(sp.Var("X")); err == nil {
		t.Error("an apply that failed was reported as done")
	}
	if got := st.Get("X"); got != "value" {
		t.Errorf("X = %q, want the answer to stand", got)
	}
}

// One the run started with is asked again instead: nobody watched it being put
// in force, and a password typed next on a keymap that never loaded is refused
// without a word about why.
func TestSettleAsksAgainForAnAnswerItCannotPutInForce(t *testing.T) {
	_, st, r := setupWith(t, `
variables:
  - name: KEYMAP
    type: text
    title: Keymap
    required: true
    apply: refuse()
  - name: FONT
    type: text
    title: Font
    default: auto
    apply: only_auto()
`, "refuse() { return 1; }\nonly_auto() { [ \"$FONT\" = auto ]; }\n", nil)
	st.Set("KEYMAP", "de-latin1")
	st.Set("FONT", "ter-v32n")
	r.Settle()

	if got := st.Get("KEYMAP"); got != "" {
		t.Errorf("KEYMAP = %q, want it unanswered again", got)
	}
	if got := st.Get("FONT"); got != "auto" {
		t.Errorf("FONT = %q, want it back on its default", got)
	}
}

// Settle is what makes a second start on the same machine stand where the first
// one left off: the answers survived in the answer file and their effect on the
// live system did not.
func TestSettleAppliesOnlyTheAnswersThatWereGiven(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DIR", dir)
	_, st, r := setupWith(t, `
variables:
  - name: GIVEN
    type: text
    title: Given
    apply: give()
  - name: OPEN
    type: text
    title: Open
    apply: open_up()
`, "give() { touch \"${DIR}/given\"; }\nopen_up() { touch \"${DIR}/open\"; }\n", nil)
	st.Set("GIVEN", "de-latin1")
	r.Settle()
	if _, err := os.Stat(filepath.Join(dir, "given")); err != nil {
		t.Errorf("an answer this run started with was not put in force: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "open")); err == nil {
		t.Error("a question nobody has answered was applied")
	}
}

// true and false are the runtime's own two words wherever they turn up, so a
// list that offers a third answer beside them still reads as Yes and No.
func TestAListHoldingTrueAndFalseStillReadsInWords(t *testing.T) {
	i18n.Use("de", &i18n.Catalog{Messages: map[string]string{"Yes": "Ja", "No": "Nein"}})
	defer i18n.Use(i18n.SourceLang)

	sp, _, r := setup(t, "variables:\n  - name: X\n    type: list\n    title: X\n    options: [auto, true, false]\n", nil)
	got, err := r.Options(sp.Var("X"))
	if err != nil {
		t.Fatal(err)
	}
	want := []Option{{Value: "auto", Label: "auto"}, {Value: "true", Label: "Ja"}, {Value: "false", Label: "Nein"}}
	if len(got) != len(want) {
		t.Fatalf("options = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("option %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// The starting point that fetches its answers: an action's script writes them
// into the answer file, and the runner reads them back and puts them into
// force. This is the whole of how a shared configuration becomes an
// installation.
func TestAFetchedConfigurationBecomesTheAnswers(t *testing.T) {
	applied := filepath.Join(t.TempDir(), "applied")
	t.Setenv("APPLIED", applied)
	sp, st, r := acting(t, "variables:\n"+
		"  - name: DISK\n    type: text\n    title: Disk\n"+
		"  - name: KEYMAP\n    type: text\n    title: Keymap\n    apply: mark_applied()\n"+
		"presets:\n  - title: Online\n    action: fetch\n",
		"mark_applied() { touch \"$APPLIED\"; }\n",
		map[string][2]string{
			"fetch": {"title: Fetch\n", "printf \"DISK='/dev/sdz'\\nKEYMAP='de'\\n\" >>\"$MODULE_CONF\"\n"},
		}, false)

	if err := opened(t, r, sp.Action("fetch")); err != nil {
		t.Fatal(err)
	}
	if err := r.Imported(); err != nil {
		t.Fatal(err)
	}

	if got := st.Get("DISK"); got != "/dev/sdz" {
		t.Errorf("DISK = %q, want /dev/sdz", got)
	}
	if _, err := os.Stat(applied); err != nil {
		t.Error("the imported keymap was never applied to the live system")
	}
}

// A question a machine can see the answer to is not asked but worked out, and
// worked out again whenever the answer it follows from changes.
func TestADerivedAnswerIsReadOffTheMachine(t *testing.T) {
	_, st, r := setupWith(t, "variables:\n"+
		"  - name: DISK\n    type: text\n    title: Disk\n"+
		"  - name: ENCRYPTED\n    title: Encrypted\n    type: bool\n    value-from: encrypted()\n",
		"encrypted() { if [ \"$DISK\" = /dev/sdz ]; then echo true; else echo false; fi; }\n", nil)

	r.Resolve()
	if got := st.Get("ENCRYPTED"); got != "false" {
		t.Errorf("ENCRYPTED = %q before the disk is answered, want false", got)
	}

	st.Set("DISK", "/dev/sdz")
	r.Resolve()
	if got := st.Get("ENCRYPTED"); got != "true" {
		t.Errorf("ENCRYPTED = %q after the disk changed, want true", got)
	}
}

// A script that will not run leaves the value empty rather than whatever it
// held before: there is no question to fall back on, and a guard on an empty
// name is simply false.
func TestADerivedAnswerThatCannotBeReadIsEmpty(t *testing.T) {
	_, st, r := setupWith(t, "variables:\n  - name: X\n    type: text\n    title: X\n    value-from: broken()\n", "broken() { return 7; }\n", nil)
	st.Set("X", "stale")

	r.Resolve()
	if got := st.Get("X"); got != "" {
		t.Errorf("X = %q after an answer that would not run, want empty", got)
	}
}

// Read again right before the work, a list names every answer it no longer
// prints - and only those: an answer its own list still offers, one typed into
// the box a list carries for exactly that, one whose list cannot be read and
// one whose question does not apply are all let be.
func TestUnofferedNamesTheAnswersTheirListsNoLongerPrint(t *testing.T) {
	_, st, r := setupWith(t, `variables:
  - name: DISK
    type: list
    title: Disk
    options-from: disks()
  - name: GONE
    type: list
    title: Gone
    options-from: numbers()
  - name: FONT
    type: open-list
    title: Font
    options-from: fonts()
  - name: BROKEN
    type: list
    title: Broken
    options-from: broken()
  - name: OFF
    type: list
    title: Off
    options-from: switches()
    conditions: DISK == nothing
`, `disks() { printf '/dev/sda\t/dev/sda  1TB\n'; }
numbers() { printf 'one\ntwo\n'; }
fonts() { echo ter-v16n; }
broken() { return 1; }
switches() { echo on; }
`, nil)
	st.Set("DISK", "/dev/sda")
	st.Set("GONE", "three")
	st.Set("FONT", "my-own-font")
	st.Set("BROKEN", "anything")
	st.Set("OFF", "whatever")

	if got := r.Unoffered()(); strings.Join(got, ",") != "GONE" {
		t.Errorf("unoffered = %v, want only GONE", got)
	}
}

// The environment is taken when the check is made, on the side that owns the
// answers, and the lists are read against it wherever the check then runs.
func TestUnofferedReadsTheListsAgainstTheAnswersWhenItWasMade(t *testing.T) {
	_, st, r := setupWith(t, `variables:
  - name: LAYOUT
    type: list
    title: Layout
    options-from: layouts()
  - name: VARIANT
    type: list
    title: Variant
    options-from: variants()
`, `layouts() { echo de; }
variants() { if [ "$LAYOUT" = de ]; then echo nodeadkeys; else echo intl; fi; }
`, nil)
	st.Set("LAYOUT", "de")
	st.Set("VARIANT", "nodeadkeys")
	check := r.Unoffered()
	st.Set("LAYOUT", "us")
	if got := check(); len(got) != 0 {
		t.Errorf("unoffered = %v, want none: the lists were read against the answers of the moment", got)
	}
}
