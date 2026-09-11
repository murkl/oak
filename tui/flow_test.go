package tui

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/murkl/oak/internal/i18n"
	"github.com/murkl/oak/internal/runner"
	"github.com/murkl/oak/internal/spec"
	"github.com/murkl/oak/internal/store"

	"github.com/charmbracelet/bubbles/cursor"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// What a test module's declaration is called: the runtime takes whichever yaml
// it finds in the folder, and these tests use the name the real modules use.
const treeFile = spec.FileModule

// A whole program, driven by keystrokes, with a folder written for the test.
//
// The interface is tested the way it is used: press keys, read the screen. What
// is asserted is what a person would see, so a refactor that keeps the
// behaviour keeps the tests.

type harness struct {
	t    *testing.T
	m    *Model
	a    *app
	msgs chan tea.Msg

	// How many commands are out and have not answered yet. Without it the loop
	// below could only tell "settled" from "still working" by waiting, and a
	// keystroke that starts nothing would cost the same as an installation.
	inflight atomic.Int64
}

// The module every flow test starts from: one page of presets, a handful of
// questions, three tasks, one of them conditional.
const testInstaller = `
title: Test Installer
confirm: Erasing {{DISK}}.
stages: [go, finish]
presets:
  - title: Setup
    description: Choose what kind of system to install.
    options:
      - title: Full
        description: Everything at once.
        values:
          EXTRAS: "true"
      - title: Bare
        description: Nothing at all.
        values:
          EXTRAS: "false"
variables:
  - name: USER
    title: User name
    description: The account you log in with.
    group: Identity
    required: true
    pattern: '^[a-z]+$'
    error: Lower case letters only.
  - name: PW
    title: Password
    type: secret
    required: true
  - name: DISK
    title: Disk
    group: Storage
    required: true
    command: printf '/dev/sda\t/dev/sda  1TB\n/dev/sdb\t/dev/sdb  2TB\n'
  - name: EXTRAS
    title: Extras
    group: Storage
    type: bool
  - name: DRIVER
    title: Driver
    values: [mesa, nvidia]
    required: true
    conditions: EXTRAS == true
`

// The three tasks, as the files they are made of.
var testTasks = map[string]string{
	"tasks/@go/a-first/task.yaml":  "title: First\n",
	"tasks/@go/a-first/task.sh":    "echo ran\n",
	"tasks/@go/b-second/task.yaml": "title: Second\n",
	"tasks/@go/b-second/task.sh":   "echo ran\n",
	"tasks/@go/c-extras/task.yaml": "title: Only with extras\nconditions: EXTRAS == true\n",
	"tasks/@go/c-extras/task.sh":   "echo ran\n",
}

// writeModule puts one module on disk — the standard one, with whatever a test
// changed about it — and answers with the folder it went into.
func writeModule(t *testing.T, dir string, files map[string]string) string {
	t.Helper()
	base := map[string]string{treeFile: testInstaller}
	for name, body := range testTasks {
		base[name] = body
	}
	for name, body := range files {
		base[name] = body
	}
	for name, body := range base {
		// An empty body is how a test leaves one of the standard files out.
		if body == "" {
			continue
		}
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func loadModule(t *testing.T, dir string) *spec.Module {
	t.Helper()
	mod, err := spec.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return mod
}

// openModule is the interface's Open, as the program supplies it: where this
// module's answers go, the runner that joins the two, and the catalogs it
// brought.
func openModule(t *testing.T) Open { return openModuleIn(t, false) }

// openModuleIn is the same with the run's own DEBUG settled, for the tests that
// are about what a script is handed rather than about a page.
func openModuleIn(t *testing.T, debug bool) Open {
	t.Helper()
	answers := t.TempDir()
	return func(mod *spec.Module) (*Program, error) {
		st := store.New(mod, filepath.Join(answers, mod.ID()+".conf"), debug)
		// The catalogs the module brought, discovered the way the program
		// discovers them — so a test that writes one is testing what ships.
		var sources []fs.FS
		if mod.Locales != "" {
			sources = append(sources, os.DirFS(mod.Locales))
		}
		return &Program{
			Module: mod, Store: st, Runner: runner.New(mod, st),
			Langs: i18n.Discover(sources...), Sources: sources,
		}, nil
	}
}

// The runtime the flow tests run inside: a name over the modules, and nothing
// else it needs to say for a test with no terminal to dress.
var testRuntime = &spec.Runtime{Title: "Test OS", Modules: []string{"installer", "recovery"}}

// start brings the interface up around one or more modules, exactly as Run
// does.
func start(t *testing.T, mods ...*spec.Module) *harness {
	t.Helper()
	return startIn(t, "", mods...)
}

// startIn is the same with catalogs of the runtime's own, for the pages that
// are drawn before any module has been opened.
func startIn(t *testing.T, locales string, mods ...*spec.Module) *harness {
	t.Helper()
	return startWith(t, openModule(t), locales, mods...)
}

// startWith is the same again for a run opened some other way — with --debug,
// for the tests that are about what a script is handed.
func startWith(t *testing.T, open Open, locales string, mods ...*spec.Module) *harness {
	t.Helper()
	i18n.Use(i18n.SourceLang)
	a := &app{
		runtime: testRuntime, modules: mods, open: open, version: "test",
		prefs: store.NewPreferences(filepath.Join(t.TempDir(), "runtime.conf")),
	}
	if locales != "" {
		a.sources = []fs.FS{os.DirFS(locales)}
		a.langs = i18n.Discover(a.sources...)
	}
	if len(mods) == 1 {
		if err := a.enter(mods[0]); err != nil {
			t.Fatal(err)
		}
	}
	h := &harness{t: t, a: a, msgs: make(chan tea.Msg, 64)}
	h.m = newModel(a, "")
	h.run(h.m.Init())
	h.drain()
	return h
}

func newHarness(t *testing.T, files map[string]string) *harness {
	t.Helper()
	return start(t, loadModule(t, writeModule(t, t.TempDir(), files)))
}

// newSimulated is the same for a run started with --debug: every script is
// handed DEBUG=true, which is the one thing no page can stand in for.
func newSimulated(t *testing.T, files map[string]string) *harness {
	t.Helper()
	return startWith(t, openModuleIn(t, true), "", loadModule(t, writeModule(t, t.TempDir(), files)))
}

// The loop, as the real program runs it: a command goes off on its own and
// whatever it produces comes back as a message. Nothing here waits on a
// command, because some of them are clocks that fire in half a second and one
// of them is an installation.
func (h *harness) run(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	h.inflight.Add(1)
	go func() {
		// Counted down only after the message is queued, so a loop that sees
		// nothing out there has already been handed everything there was.
		defer h.inflight.Add(-1)
		if msg := cmd(); msg != nil {
			h.msgs <- msg
		}
	}()
}

// The interface's clocks, fired the moment a page sets one. What a page does
// when its clock goes off is what these tests are about; the stretch of time in
// front of it is not, and waiting those out was what the suite spent its run on.
func init() {
	after = func(_ time.Duration, fire func(time.Time) tea.Msg) tea.Cmd {
		return func() tea.Msg { return fire(time.Now()) }
	}
	// The one clock bubbles owns: a blinking cursor is a command that sits
	// there for half a second and answers with a redraw nothing asserts on.
	cursorMode = cursor.CursorStatic
}

// quiet is how long the loop waits on a command that is still out there before
// it takes the interface to have settled anyway.
//
// With the clocks stopped this is only ever the installation: a page reacting
// answers in microseconds, and what is left taking real time is a task, which
// is a real process. A test watching a run needs the screen while that is still
// going, so this is the one thing here deliberately not waited out.
const quiet = 60 * time.Millisecond

// drain handles everything waiting, and everything that arrives while it is
// handling it, until nothing is left and nothing is still coming.
func (h *harness) drain() {
	for {
		var msg tea.Msg
		select {
		case msg = <-h.msgs:
		default:
			// Nothing queued. With nothing out there either, this is as
			// settled as it is going to get.
			if h.inflight.Load() == 0 {
				return
			}
			select {
			case msg = <-h.msgs:
			case <-time.After(quiet):
				return
			}
		}
		h.handle(msg)
	}
}

// handle is one message put through the program, exactly as bubbletea would.
func (h *harness) handle(msg tea.Msg) {
	switch msg := msg.(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			h.run(c)
		}
	case tickMsg, spinMsg, animMsg:
		// A clock. It carries nothing and re-arms itself, so handling one
		// would be a test that never ends.
	default:
		if blink(msg) {
			return
		}
		m, cmd := h.m.Update(msg)
		h.m = m.(*Model)
		h.run(cmd)
	}
}

// blink reports whether a message is a text cursor asking to be redrawn — the
// other clock in the program, and the only one this file cannot name outright
// because the type behind it is not exported.
func blink(msg tea.Msg) bool {
	return strings.HasPrefix(fmt.Sprintf("%T", msg), "cursor.")
}

func (h *harness) send(msg tea.Msg) *harness {
	h.t.Helper()
	m, cmd := h.m.Update(msg)
	h.m = m.(*Model)
	h.run(cmd)
	h.drain()
	return h
}

// restart is this machine started a second time: the answers are where the last
// run left them, and the program is asked again what there is to show.
func (h *harness) restart() *harness {
	h.t.Helper()
	h.a.first = false
	h.m = newModel(h.a, "")
	h.run(h.m.Init())
	h.drain()
	return h
}

func (h *harness) key(k tea.KeyType) *harness {
	h.send(tea.KeyMsg{Type: k})
	return h
}

func (h *harness) enter() *harness { return h.key(tea.KeyEnter) }
func (h *harness) down() *harness  { return h.key(tea.KeyDown) }
func (h *harness) esc() *harness   { return h.key(tea.KeyEsc) }
func (h *harness) ctrlC() *harness { return h.key(tea.KeyCtrlC) }

// erase is the other way to say back, and the delete key in front of a text
// box. Which of the two it is is the whole point of testing it.
func (h *harness) erase() *harness { return h.key(tea.KeyBackspace) }

func (h *harness) typeIn(s string) *harness {
	h.t.Helper()
	for _, r := range s {
		h.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return h
}

// screen is what is on it, as plain text.
func (h *harness) screen() string { return h.m.View() }

func (h *harness) wants(fragments ...string) *harness {
	h.t.Helper()
	view := h.screen()
	for _, want := range fragments {
		if !strings.Contains(view, want) {
			h.t.Fatalf("the screen does not show %q:\n%s", want, view)
		}
	}
	return h
}

func (h *harness) refuses(fragments ...string) *harness {
	h.t.Helper()
	view := h.screen()
	for _, unwanted := range fragments {
		if strings.Contains(view, unwanted) {
			h.t.Fatalf("the screen shows %q and should not:\n%s", unwanted, view)
		}
	}
	return h
}

// ran waits for a run to finish and for its result to become answerable. It is
// the one thing here that is genuinely worth waiting for: the stages are real
// processes, and the pause afterwards is what stops a keystroke meant for
// something else from dismissing the result.
func (h *harness) ran() *harness {
	h.t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if r, ok := h.m.top().(*runScreen); ok && r.settled {
			return h
		}
		h.drain()
	}
	h.t.Fatalf("the run never settled; the page on top is %T", h.m.top())
	return h
}

// asked waits for the run to stop at a task that asks first, and for its
// question to become answerable.
func (h *harness) asked() *harness {
	h.t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if r, ok := h.m.top().(*runScreen); ok && r.asking != nil && r.settled {
			return h
		}
		h.drain()
	}
	h.t.Fatalf("no question was ever asked; the page on top is %T", h.m.top())
	return h
}

// askedFor waits for the run to stop at a task that needs a value, with its
// answers fetched and its question answerable.
func (h *harness) askedFor() *harness {
	h.t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if r, ok := h.m.top().(*runScreen); ok && r.ask != nil && !r.ask.loading && r.settled {
			return h
		}
		h.drain()
	}
	h.t.Fatalf("nothing was ever asked for; the page on top is %T", h.m.top())
	return h
}

// reported waits for the run to stop on something it has to say, and for that
// page to become answerable.
func (h *harness) reported() *harness {
	h.t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if r, ok := h.m.top().(*runScreen); ok && r.told != nil && r.settled {
			return h
		}
		h.drain()
	}
	h.t.Fatalf("nothing was ever reported; the page on top is %T", h.m.top())
	return h
}

// answered waits for a question with shell hanging on it to have run that shell
// and come back — either onto the next page, or onto the same one with the
// reason it would not work.
func (h *harness) answered() *harness {
	h.t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if f, ok := h.m.top().(*fieldScreen); !ok || !f.busy {
			return h
		}
		h.drain()
	}
	h.t.Fatalf("the answer was never made good on")
	return h
}

// ─── What comes first ────────────────────────────────────────────────────────

// The one thing that cannot wait for the opening run of questions: a question
// marked `first` is asked before the network screen, because the passphrase
// typed into that screen is already typed on the keyboard this answer settles.
func TestAFirstQuestionIsAskedBeforeTheNetwork(t *testing.T) {
	h := newHarness(t, map[string]string{
		treeFile: testInstaller +
			"  - name: LOCALE\n    title: Language and formats\n    required: true\n    first: true\n    values: [de, en]\n",
		"hooks/@online/check/hook.yaml": "title: Online\nscript: exit 1\n",
	})
	h.wants("Language and formats", "de", "en").refuses("Wireless network", "internet connection")

	// Answered, and only now is there a network to look for.
	h.enter()
	h.wants("There is no internet connection.")
	if got := h.a.store.Get("LOCALE"); got != "de" {
		t.Errorf("LOCALE = %q, want the answer given before the network", got)
	}
}

// And the point of asking it first: answering it puts it in force there and
// then, before the page after it is on screen to be typed into.
func TestAnsweringAFirstQuestionPutsItInForce(t *testing.T) {
	loaded := filepath.Join(t.TempDir(), "loaded")
	h := newHarness(t, map[string]string{
		treeFile: testInstaller +
			"  - name: LOCALE\n    title: Language and formats\n    required: true\n    first: true\n" +
			"    values: [de, en]\n    apply: echo \"$LOCALE\" > " + loaded + "\n",
	})
	h.wants("Language and formats")
	if _, err := os.Stat(loaded); err == nil {
		t.Fatal("the answer was applied before it was given")
	}

	h.down().enter() // en
	got, err := os.ReadFile(loaded)
	if err != nil {
		t.Fatalf("the answer was never put in force: %v", err)
	}
	if strings.TrimSpace(string(got)) != "en" {
		t.Errorf("applied %q, want the answer that was just given", got)
	}
}

// A `first` question that is already answered is not asked again on the way in:
// a second start goes straight to the network, exactly as it would if nothing
// had ever been marked.
func TestAnAnsweredFirstQuestionIsNotAskedAgain(t *testing.T) {
	h := newHarness(t, map[string]string{
		treeFile: testInstaller +
			"  - name: LOCALE\n    title: Language and formats\n    required: true\n    first: true\n    default: de\n    values: [de, en]\n",
		"hooks/@online/check/hook.yaml": "title: Online\nscript: exit 1\n",
	})
	h.wants("There is no internet connection.").refuses("Language and formats")
}

// A question asked first is asked before loadkeys has run, so even the key that
// would normally open the filter is typed on a layout nobody has chosen yet.
// Its box is up from the first frame, and typing narrows straight away — no /
// needed first, and nothing in the yaml to say so.
func TestAQuestionAskedFirstOpensItsFilterFromTheStart(t *testing.T) {
	h := newHarness(t, map[string]string{
		treeFile: testInstaller +
			"  - name: KEYMAP\n    title: Console keyboard\n    required: true\n    first: true\n    values: [us, de]\n",
	})
	h.wants("Console keyboard", "Filter …")
	h.typeIn("de")
	h.wants("de").refuses("us")
}

// ─── Which program ───────────────────────────────────────────────────────────

// A runtime offering more than one module asks which to open before anything
// follows from it — and what it asks with is entirely each module's own words.
// What is chosen then decides which questions there are, where the answers go
// and what the run is called from there on.
const testRecovery = `
title: Test Recovery
description: Open a system already on a disk.
confirm: Opening {{DISK}}.
stages: [open]
variables:
  - name: DISK
    title: Disk
    required: true
    values: [/dev/sda]
  - name: SNAPSHOT
    title: Snapshot
    required: true
    values: [one, two]
`

// both is a folder holding two modules, the way a build leaves one: the
// standard installer, and a recovery beside it.
func both(t *testing.T) []*spec.Module {
	t.Helper()
	dir := t.TempDir()
	installer := writeModule(t, filepath.Join(dir, "installer"), map[string]string{
		treeFile: strings.Replace(testInstaller, "title: Test Installer",
			"title: Test Installer\ndescription: Put a system on this machine.", 1),
	})
	recovery := writeModule(t, filepath.Join(dir, "recovery"), map[string]string{
		treeFile:                       testRecovery,
		"tasks/@go/a-first/task.yaml":  "",
		"tasks/@go/a-first/task.sh":    "",
		"tasks/@go/b-second/task.yaml": "",
		"tasks/@go/b-second/task.sh":   "",
		"tasks/@go/c-extras/task.yaml": "",
		"tasks/@go/c-extras/task.sh":   "",
		"tasks/@open/d-open/task.yaml": "title: Open the disk\n",
		"tasks/@open/d-open/task.sh":   "echo opened\n",
	})
	return []*spec.Module{loadModule(t, installer), loadModule(t, recovery)}
}

func TestChoosingAProgramSettlesTheQuestionsTheWarningAndTheRun(t *testing.T) {
	h := start(t, both(t)...)
	h.wants("What to do", "Test Installer", "Test Recovery", "Put a system on this machine.")

	h.down().enter() // the recovery
	h.wants("Disk").enter()
	h.wants("Snapshot", "one", "two").enter()

	// The hub, the warning and the run are all read in that module's own name for
	// a run of it, and only its own tasks run.
	h.wants("Test Recovery", "Open a system already on a disk.").enter()
	h.wants("Ready to start", "Opening /dev/sda.", "Start Test Recovery").enter()
	h.ran()
	h.wants("Test Recovery complete in", "Open the disk")
	h.refuses("First")
}

// The other module's questions are not this one's: they are declared in a folder
// this run never opened.
func TestTheOtherProgramsQuestionsAreNotAsked(t *testing.T) {
	h := start(t, both(t)...)
	h.enter() // the installer, the row the page opens on
	h.wants("Setup", "Choose what kind of system to install.")
	h.refuses("Snapshot")
}

// The page in front of all of them is headed by none of them: naming it after
// one would answer its own question, so it is read under the name the runtime
// gave itself.
func TestTheQuestionOfWhichModuleIsHeadedByTheRuntime(t *testing.T) {
	h := start(t, both(t)...)
	h.wants(testRuntime.Title, "What to do")
}

// And the landing page comes in front of that: the words the rest is read in
// belong to the runtime rather than to any module, and the question of which
// module to open is itself read in them.
func TestTheLandingPageComesBeforeTheQuestionOfWhichModule(t *testing.T) {
	mods := both(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "de.po"), []byte("msgid \"What to do\"\nmsgstr \"Was tun\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := startIn(t, dir, mods...)
	h.wants("Welcome", "Welcome to Test OS.", "English").refuses("What to do")

	h.down().enter() // Deutsch
	h.wants("Was tun", "Test Installer", "Test Recovery")
}

// A row that opens something says what will happen on it, not what the thing is
// called: `action:` is the word, on the page asking which module to open and on
// the menu inside one. The title is for the sentences about it — the header
// among them, which is why it is not refused on the whole screen here.
func TestARowThatOpensAModuleCarriesWhatItDoes(t *testing.T) {
	tree := strings.Replace(testInstaller, "title: Test Installer\n",
		"title: Test Installer\naction: Set it up\n", 1)
	h := newHarness(t, map[string]string{treeFile: tree})
	h.down().enter() // a starting point
	h.typeIn("moritz").enter().enter()
	h.wants("Set it up").refuses(glyphs.cursor + "Test Installer")

	// And the title is what the sentences about it are written with.
	h.down()
	h.wants("Every value Test Installer will use.")
}

// A module that named none falls back on its own name: a row with nothing on it
// is worse than a row reading like a label.
func TestAModuleWithNoActionFallsBackOnItsName(t *testing.T) {
	h := newHarness(t, nil)
	h.down().enter() // a starting point
	h.typeIn("moritz").enter().enter()
	h.wants("Test Installer")
}

// The pages the runtime brings with it belong to whichever module was opened and
// are read in its name. A recovery whose settings talk about an installer is the
// runtime putting words in a program's mouth.
func TestTheRuntimesOwnPagesNameTheProgramThatWasOpened(t *testing.T) {
	h := start(t, both(t)...)
	h.down().enter() // the recovery
	h.wants("Disk").enter()
	h.wants("Snapshot").enter()

	h.down() // the settings row, which is where its description is read
	h.wants("Every value Test Recovery will use.").refuses("installer")
}

// The frame is titled after the product on every page, and once a module is
// open, after that module too: the header says what this run is even once the
// page that named it has scrolled away.
func TestTheFrameIsTitledAfterTheProductAndTheModuleOnceOneIsOpen(t *testing.T) {
	h := start(t, both(t)...)
	h.down().enter() // the recovery
	h.wants("Disk").enter()
	h.wants("Snapshot").enter()
	h.wants(testRuntime.Title + " " + glyphs.crumb + " Test Recovery")
}

// ─── Starting points ─────────────────────────────────────────────────────────

// A starting point is only a starting point once: a machine that has answered
// before is not asked again, because every value it filled in is by then an
// ordinary answer somebody may have changed.
func TestAPresetIsOnlyOfferedOnce(t *testing.T) {
	h := newHarness(t, nil)
	h.wants("Setup", "Full", "Bare")
	h.down().enter().typeIn("moritz").enter().enter()
	h.wants("Test Installer", "Settings")

	h.restart()
	h.wants("Test Installer", "Settings").refuses("Full", "Bare")
}

// A preset fills in answers, and an answer is an answer whether it was typed or
// chosen in one keypress. This module ties its words on screen to one of them and
// puts the other into effect on the machine, which are the two things an answer
// can do beyond being stored.
func presetTreeTying(apply string) map[string]string {
	declared := strings.Replace(testInstaller,
		"          EXTRAS: \"true\"\n",
		"          EXTRAS: \"true\"\n          LOCALE: de_DE\n", 1)
	return map[string]string{
		treeFile: declared +
			"  - name: LOCALE\n    title: System language\n    required: true\n    values: [de_DE, en_US]\n" + apply +
			"language: LOCALE\n",
		"locales/de.po": "msgid \"English\"\nmsgstr \"Deutsch\"\n\nmsgid \"User name\"\nmsgstr \"Benutzername\"\n",
	}
}

func TestAPresetCanChangeTheLanguageItIsReadIn(t *testing.T) {
	h := newHarness(t, presetTreeTying(""))
	h.enter() // English, the language the interface opens in
	h.enter() // Full, which fills in a German locale
	h.wants("Benutzername").refuses("User name")
}

func TestAPresetPutsWhatItFilledInInForce(t *testing.T) {
	loaded := filepath.Join(t.TempDir(), "loaded")
	h := newHarness(t, presetTreeTying("    apply: echo \"$LOCALE\" > "+loaded+"\n"))
	if _, err := os.Stat(loaded); err == nil {
		t.Fatal("a value was applied before the page that fills it in was answered")
	}

	h.enter() // English
	h.enter() // Full
	got, err := os.ReadFile(loaded)
	if err != nil {
		t.Fatalf("what the preset filled in was never put in force: %v", err)
	}
	if strings.TrimSpace(string(got)) != "de_DE" {
		t.Errorf("applied %q, want the value the preset filled in", got)
	}
}

// ─── The language of the interface ───────────────────────────────────────────

// A module that speaks more than one language but ties none of them to an answer
// of its own. The words on screen are then a setting of this program, and the
// module's own language question is a question about the machine like any other.
func twoLanguageTree() map[string]string {
	return map[string]string{
		treeFile: testInstaller +
			"  - name: LOCALE\n    title: System language\n    required: true\n    values: [de_DE, en_US]\n",
		"locales/de.po": "msgid \"English\"\nmsgstr \"Deutsch\"\n\nmsgid \"Setup\"\nmsgstr \"Einrichtung\"\n",
	}
}

// And it is read before there is a language to read it in, so it is written in
// one and stays there: a catalog offering a translation of its words is never
// asked for one, and a machine that chose German last time opens on the same
// English page.
func TestTheLandingPageIsNeverTranslated(t *testing.T) {
	tree := twoLanguageTree()
	tree["locales/de.po"] += "\nmsgid \"Welcome\"\nmsgstr \"Willkommen\"\n" +
		"\nmsgid \"Welcome to %s.\"\nmsgstr \"Willkommen bei %s.\"\n"
	h := newHarness(t, tree)
	h.down().enter() // Deutsch
	h.restart()
	h.wants("Welcome", "Welcome to Test OS.").refuses("Willkommen")
}

// The landing page leads, because every word of every page after it is in the
// language chosen on it.
func TestTheLandingPageIsTheFirstThingDrawn(t *testing.T) {
	h := newHarness(t, twoLanguageTree())
	h.wants("Welcome", "Deutsch").refuses("Full", "Bare")

	h.down().enter() // Deutsch
	h.wants("Einrichtung", "Full", "Bare")
}

// And it answers nothing about the machine being installed: the module's own
// language question is still asked, and what was chosen here does not answer it.
func TestTheInterfaceLanguageIsNotAnAnswer(t *testing.T) {
	h := newHarness(t, twoLanguageTree())
	h.down().enter() // Deutsch
	if got := h.a.store.Get("LOCALE"); got != "" {
		t.Errorf("LOCALE = %q, want the interface language to have answered nothing", got)
	}

	h.down().enter()                   // Bare
	h.typeIn("moritz").enter().enter() // the user name, the disk
	h.wants("System language", "de_DE", "en_US")
}

// ─── The network ─────────────────────────────────────────────────────────────

// A module with no internet problem to begin with never sees the network
// screen at all: it is a fix offered when one is needed, not a page every
// installation has to click through.
func TestNetworkScreenIsSkippedWhenAlreadyOnline(t *testing.T) {
	h := newHarness(t, map[string]string{
		"hooks/@online/check/hook.yaml": "title: Online\nscript: exit 0\n",
	})
	h.wants("Full", "Bare").refuses("Wireless network")
}

// Offline and nothing declared to join with: the screen says so and lets the
// installation carry on regardless — the module's own preflight is what refuses
// properly if the connection still matters.
func TestNetworkScreenOffersToContinueWithoutWhenNotJoinable(t *testing.T) {
	h := newHarness(t, map[string]string{
		"hooks/@online/check/hook.yaml": "title: Online\nscript: exit 1\n",
	})
	h.wants("There is no internet connection.", "wireless network before continuing.")
	h.enter()
	h.wants("Full", "Bare")
}

// Offline with a full network description: the screen lists what is in range,
// joining one is what makes the check pass, and the installer moves on to its
// own opening exactly as it would have if there had been a cable plugged in.
func TestNetworkScreenJoinsAWirelessNetworkWhenOffline(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "online")
	h := newHarness(t, map[string]string{
		"hooks/@online/check/hook.yaml":        "title: Online\nscript: test -e " + marker + "\n",
		"hooks/@wlan-device/station/hook.yaml": "title: Device\nscript: printf wlan0\n",
		"hooks/@wlan-networks/scan/hook.yaml":  "title: Networks\nscript: printf 'HomeNet\\nCafeNet\\n'\n",
		"hooks/@wlan-connect/join/hook.yaml":   "title: Join\nscript: touch " + marker + "\n",
	})
	h.wants("Wireless network", "HomeNet", "CafeNet")

	h.enter() // join HomeNet
	h.wants("HomeNet", "Passphrase")

	h.typeIn("secret").enter()
	h.wants("Full", "Bare") // online now, straight into the installer's own opening
}

// ─── The opening ─────────────────────────────────────────────────────────────

// One language on offer means no landing page: the one thing it asks is not a
// question, and a greeting is not reason enough to stop a run on a page nobody
// can answer.
func TestOneLanguageIsNoLandingPage(t *testing.T) {
	newHarness(t, nil).wants("Full", "Bare").refuses("Welcome", "Language")
}

// A module may tie the words on screen to one of its own answers — see
// `language:` in a module's declaration. Answering it then also settles the
// language, whatever was chosen on the way in.
func TestAModuleCanTieTheInterfaceToOneOfItsOwnAnswers(t *testing.T) {
	h := newHarness(t, regionTree)
	h.enter() // English, the language the interface opens in
	h.wants("Language and region", "de_DE")

	// The answer is a locale rather than the name of a catalog, and it is read
	// as one: de_DE is German.
	h.enter()
	h.wants("Einrichtung")
	if got := h.a.prefs.Lang(); got != "de" {
		t.Errorf("the language kept = %q, want the one the answer came closest to", got)
	}
}

// And the other way: an answer no catalog fits leaves the language everything
// is written in standing.
func TestAnAnswerNoCatalogFitsLeavesTheSourceLanguage(t *testing.T) {
	h := newHarness(t, regionTree)
	h.enter()        // English
	h.down().enter() // en_US
	h.wants("Setup").refuses("Einrichtung")
}

// And the settings page does not offer a second way to set it: the module's own
// row is the language, and a runtime row above it could only disagree with it.
func TestSettingsOffersNoLanguageOfItsOwnWhenTheModuleOwnsIt(t *testing.T) {
	h := newHarness(t, regionTree)
	h.enter()        // English
	h.down().enter() // en_US, so this page stays in the language it is read in
	h.down().enter().typeIn("moritz").enter().enter()
	h.down().enter() // Settings
	h.wants("Language and region", "en_US").refuses("Deutsch")
}

// The module the three tests above are about: one question standing for a whole
// region, and a catalog for one of the languages it can come to.
var regionTree = map[string]string{
	treeFile: testInstaller +
		"  - name: LOCALE\n    title: Language and region\n    required: true\n    first: true\n" +
		"    values: [de_DE, en_US]\n" +
		"language: LOCALE\n",
	"locales/de.po": "msgid \"English\"\nmsgstr \"Deutsch\"\n\nmsgid \"Setup\"\nmsgstr \"Einrichtung\"\n",
}

func TestTheOpeningRunsPresetThenQuestionsThenHub(t *testing.T) {
	h := newHarness(t, nil)
	h.wants("Full", "Everything at once.")

	// Bare, so the conditional question and the conditional stage stay away.
	h.down().enter()
	h.wants("User name", "The account you log in with.", "1 of 2")

	h.typeIn("moritz").enter()
	h.wants("Disk", "2 of 2", "/dev/sda  1TB")

	h.enter()
	h.wants("Test Installer", "Settings")
}

// A preset is a set of answers and nothing more.
func TestAPresetFillsInAnswersAndStopsMattering(t *testing.T) {
	h := newHarness(t, nil)
	h.enter() // Full
	if got := h.a.store.Get("EXTRAS"); got != "true" {
		t.Fatalf("EXTRAS = %q", got)
	}
	// Extras being on opens a question that would not otherwise be asked.
	h.wants("1 of 3")
	h.typeIn("moritz").enter().enter()
	h.wants("Driver", "mesa", "nvidia")
}

// A run of questions is one place you stay in, not a path through the program.
func TestTheOpeningQuestionsShowACounterRatherThanATrail(t *testing.T) {
	h := newHarness(t, nil)
	h.down().enter().typeIn("moritz").enter()
	h.wants("2 of 2").refuses("User name ›")
}

// And the pages in front of that run are the same idea: they come one after
// another rather than one inside the other, so each is read under the heading
// of the opening instead of behind every page already answered.
func TestTheOpeningPagesStandUnderOneHeadingRatherThanInsideEachOther(t *testing.T) {
	h := newHarness(t, map[string]string{
		treeFile: testInstaller +
			"  - name: LOCALE\n    title: Language and formats\n    required: true\n    first: true\n    values: [de, en]\n" +
			"  - name: KEYMAP\n    title: Console keyboard\n    required: true\n    first: true\n    values: [de, us]\n",
	})
	h.wants("Start", "Language and formats")

	h.enter()
	h.wants("Start", "Console keyboard").refuses("Language and formats")

	h.enter()
	h.wants("Start", "Setup").refuses("Console keyboard")

	// And the run of questions leaves the opening behind it entirely.
	h.enter()
	h.wants("User name", "1 of 3").refuses("Setup")
}

func TestAnAnswerThatBreaksTheRulesIsRefusedWithTheReason(t *testing.T) {
	h := newHarness(t, nil)
	h.down().enter()
	h.typeIn("Moritz1").enter()
	h.wants("Lower case letters only.", "1 of 2")
	if got := h.a.store.Get("USER"); got != "" {
		t.Errorf("USER = %q, want the refused value not stored", got)
	}
}

func TestGoingBackReturnsToThePreviousQuestion(t *testing.T) {
	h := newHarness(t, nil)
	h.down().enter().typeIn("moritz").enter()
	h.wants("2 of 2")
	h.esc()
	h.wants("User name", "1 of 2")
}

// ─── Answers ─────────────────────────────────────────────────────────────────

// A tab in a command's output separates what is stored from what is read.
func TestADiskIsStoredByPathAndChosenBySize(t *testing.T) {
	h := newHarness(t, nil)
	h.down().enter().typeIn("moritz").enter()
	h.wants("/dev/sda  1TB", "/dev/sdb  2TB")
	h.down().enter()
	if got := h.a.store.Get("DISK"); got != "/dev/sdb" {
		t.Errorf("DISK = %q, want the path rather than the label", got)
	}
}

func TestEveryAnswerIsWrittenDownAsItIsGiven(t *testing.T) {
	h := newHarness(t, nil)
	h.down().enter().typeIn("moritz").enter()
	raw, err := os.ReadFile(h.a.store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "USER='moritz'") {
		t.Fatalf("the answer file does not hold the answer:\n%s", raw)
	}
}

// ─── Settings ────────────────────────────────────────────────────────────────

func TestSettingsShowsEveryAnswerOnOnePage(t *testing.T) {
	h := newHarness(t, nil)
	h.down().enter().typeIn("moritz").enter().enter()
	h.down().enter() // Settings
	h.wants("Identity", "User name", "moritz", "Storage", "Disk", "/dev/sda", "Extras", "No")
}

// A secret has no page behind it: it is not stored, so there is nothing here to
// change.
func TestSettingsSaysASecretIsAskedForLater(t *testing.T) {
	h := newHarness(t, nil)
	h.down().enter().typeIn("moritz").enter().enter()
	h.down().enter()
	h.wants("Password", "asked just before the run")
}

func TestChangingAValueInSettingsShowsTheNewOne(t *testing.T) {
	h := newHarness(t, nil)
	h.down().enter().typeIn("moritz").enter().enter()
	h.down().enter() // Settings
	h.enter()        // the first row, which is the first value
	h.wants("User name", "The account you log in with.")
	h.typeIn("x").enter()
	h.wants("moritzx")
}

// A page of answers narrows like any other long list, and keeps the heading the
// surviving rows sit under.
func TestSettingsNarrowsToTheAnswerBeingLookedFor(t *testing.T) {
	h := newHarness(t, nil)
	h.down().enter().typeIn("moritz").enter().enter()
	h.down().enter() // Settings
	h.typeIn("/disk")
	h.wants("Storage", "Disk", "/dev/sda").refuses("User name", "Identity")
}

// The heading counts as well as the name: a setting remembered as one of the
// storage ones is found by that.
func TestSettingsNarrowsByTheHeadingToo(t *testing.T) {
	h := newHarness(t, nil)
	h.down().enter().typeIn("moritz").enter().enter()
	h.down().enter()
	h.typeIn("/storage")
	h.wants("Storage", "Disk", "Extras").refuses("User name")
}

func TestSettingsSaysSoWhenNothingMatches(t *testing.T) {
	h := newHarness(t, nil)
	h.down().enter().typeIn("moritz").enter().enter()
	h.down().enter()
	h.typeIn("/zzz")
	h.wants("No matches").refuses("User name", "Disk")
}

// Narrowing is left before the page is: the first esc closes the box, the
// second goes back.
func TestEscClosesTheSettingsFilterBeforeItLeavesThePage(t *testing.T) {
	h := newHarness(t, nil)
	h.down().enter().typeIn("moritz").enter().enter()
	h.down().enter()
	h.typeIn("/disk").esc()
	h.wants("User name", "Disk")
	h.esc()
	h.wants("Test Installer", "Settings")
}

// The query is how the row was found, so changing its value does not throw it
// away on the way back.
func TestTheSettingsFilterSurvivesChangingAValue(t *testing.T) {
	h := newHarness(t, nil)
	h.down().enter().typeIn("moritz").enter().enter()
	h.down().enter()
	h.typeIn("/disk").enter()
	h.down().enter() // the second disk
	h.wants("Disk", "/dev/sdb").refuses("User name")
}

// Turning a setting on can call for an answer nothing has asked for yet —
// extras on means a driver has to be chosen. Backing out of settings must run
// into that question rather than hand the hub a machine one enter key away
// from installing without it.
func TestTurningOnASettingAsksForWhatItNowRequiresOnTheWayOut(t *testing.T) {
	h := newHarness(t, nil)
	h.down().enter().typeIn("moritz").enter().enter()
	h.down().enter()            // Settings
	h.typeIn("/extras").enter() // the Extras row, currently No
	h.wants("Extras")
	h.key(tea.KeyUp).enter() // Yes is the row above No
	h.wants("Extras", "Yes")
	h.esc().esc() // close the filter, then leave settings
	h.wants("Driver", "mesa", "nvidia").refuses("Settings")
	h.enter() // mesa, the focused row
	h.wants("Test Installer", "Settings")
}

// ─── Installing ──────────────────────────────────────────────────────────────

func TestTheConfirmationNamesTheDiskItIsAbout(t *testing.T) {
	h := newHarness(t, nil)
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter() // Install
	h.wants("Ready to start", "Erasing /dev/sda.", "Start Test Installer")
}

func TestTheSecretIsAskedForTwiceAndOnlyThenTheRunBegins(t *testing.T) {
	h := newHarness(t, nil)
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter().enter() // Install, then start

	h.wants("Password")
	h.typeIn("hunter2").enter()
	h.wants("Repeat")
	h.typeIn("different").enter()
	h.wants("The entries do not match.", "Password")

	h.typeIn("hunter2").enter().typeIn("hunter2").enter()
	h.ran()
	h.wants("Test Installer complete", "First", "Second").refuses("Only with extras")
}

// A task that fails stops the run there, on the same page a finished run stops
// on and under the other mark. Everything about the failure is one keystroke
// behind it, laid out the way every other failure in this program is.
func TestAFailedTaskStopsTheRunAndSaysWhereItBroke(t *testing.T) {
	h := newHarness(t, map[string]string{
		"tasks/@go/b-second/task.sh": "echo starting\nls /definitely/not/here\necho never\n",
	})
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter().enter()
	h.typeIn("x").enter().typeIn("x").enter()
	h.ran()
	h.wants("Test Installer failed", "It stopped at Second").refuses("Script", "Exit code")

	h.enter().wants("Second", "Module", "Task", "Script", "Command", "Exit code", "not/here")

	// And from there back to the answers, which is where a wrong one is fixed.
	h.enter().wants("Settings")
}

// A task may ask before it runs, which is how a module offers something
// rather than does it. Declining skips that one and the run carries on.
func TestAnTaskThatAsksIsOfferedRatherThanRun(t *testing.T) {
	h := newHarness(t, map[string]string{
		"tasks/@finish/d-reboot/task.yaml": "title: Reboot\nconfirm: Restart {{DISK}} now?\n",
		"tasks/@finish/d-reboot/task.sh":   "echo never\n",
	})
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter().enter()
	h.typeIn("x").enter().typeIn("x").enter()

	h.asked()
	h.wants("Reboot", "Restart /dev/sda now?", "Yes", "No")

	// No: the row keeps its place in the list, marked as passed over.
	h.down().enter()
	h.ran()
	h.wants("Test Installer complete", "First", "Second", "Reboot")
}

// An offer opens on yes unless the task says otherwise, and one that says `no`
// is answered no by an enter nobody aimed at it — which is the whole point:
// the run is over, and the offer under it is an extra.
func TestAnOfferCanOpenOnNo(t *testing.T) {
	h := newHarness(t, map[string]string{
		"tasks/@finish/d-shell/task.yaml": "title: Shell\nconfirm: Open a shell?\ndefault: no\n",
		// Would fail the run if it were ever started.
		"tasks/@finish/d-shell/task.sh": "exit 1\n",
	})
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter().enter()
	h.typeIn("x").enter().typeIn("x").enter()

	h.asked()
	h.wants("Shell", "Open a shell?", "Yes", "No")

	h.enter()
	h.ran()
	h.wants("Test Installer complete", "Shell")
}

// A value that could not have been known before the work started: the run
// stops where the list of tasks was, asks, and carries on with the answer — and
// the offer after it can name what was just chosen.
func TestATaskCanAskForAValueInTheMiddleOfTheRun(t *testing.T) {
	h := newHarness(t, map[string]string{
		treeFile: testInstaller + `
  - name: SNAPSHOT
    title: Snapshot
    description: Which one to go back to.
    required: true
    command: printf 'one\ntwo\n'
`,
		"tasks/@finish/d-roll/task.yaml": "title: Roll back\nasks: SNAPSHOT\nconfirm: Replace @ with {{SNAPSHOT}}?\n",
		"tasks/@finish/d-roll/task.sh":   "echo rolled\n",
	})
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter().enter()
	h.typeIn("x").enter().typeIn("x").enter()

	h.askedFor()
	h.wants("Roll back", "Which one to go back to.", "one", "two")

	// The answer is stored, and the offer that follows reads it back.
	h.down().enter()
	h.asked()
	h.wants("Replace @ with two?")
	if got := h.a.store.Get("SNAPSHOT"); got != "two" {
		t.Errorf("SNAPSHOT = %q, want two", got)
	}

	h.enter()
	h.ran()
	h.wants("Test Installer complete", "Roll back")
}

// A question the run stopped for that turns out to have no answers is the end
// of the run: the work has happened, and what it was waiting for is not here.
func TestAskingForSomethingThatIsNotThereEndsTheRun(t *testing.T) {
	h := newHarness(t, map[string]string{
		treeFile: testInstaller + `
  - name: SNAPSHOT
    title: Snapshot
    command: "true"
`,
		"tasks/@finish/d-roll/task.yaml": "title: Roll back\nasks: SNAPSHOT\n",
		"tasks/@finish/d-roll/task.sh":   "echo never\n",
	})
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter().enter()
	h.typeIn("x").enter().typeIn("x").enter()
	h.ran()
	h.wants("Test Installer failed", "Roll back")
	h.enter().wants("there is nothing to choose from")
}

// Nothing typed while a run is going may dismiss its result, and nothing said
// in confidence survives it.
func TestASecretIsForgottenWhenTheRunIsOver(t *testing.T) {
	h := newHarness(t, nil)
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter().enter()
	h.typeIn("hunter2").enter().typeIn("hunter2").enter()
	h.ran()
	if got := h.a.store.Get("PW"); got != "" {
		t.Errorf("PW = %q after the run", got)
	}
}

// ─── The system check ────────────────────────────────────────────────────────

func TestAFailedSystemCheckIsAWall(t *testing.T) {
	h := newHarness(t, map[string]string{
		"hooks/@preflight/machine/hook.yaml": "title: Check\nscript: |\n  echo Set the boot mode to UEFI. >&2\n  exit 1\n",
	})
	// What the check said is what the page is about, in the words it was
	// written in — the rest is behind it.
	h.wants("Cannot continue", "Set the boot mode to UEFI.").refuses("Exit code")

	h.enter().wants("Check", "Module", "Exit code")
	// Esc is not a way off a failure: the reflex key must not carry away the one
	// explanation this run is going to give.
	h.esc().wants("Check", "Module", "Exit code")
	if h.m.quitting {
		t.Fatal("esc closed the page the failure is on")
	}
	// Nothing leads anywhere from here: saying yes to it leaves.
	h.enter()
	if !h.m.quitting {
		t.Error("enter on the wall did not leave")
	}
}

func TestASystemCheckThatPassesLeadsStraightOn(t *testing.T) {
	h := newHarness(t, map[string]string{
		"hooks/@preflight/machine/hook.yaml": "title: Check\nscript: |\n  echo fine\n",
	})
	h.wants("Full", "Bare")
}

// Nothing this program draws may run past the edge of the terminal, at any size
// a terminal comes in. 80x24 is the smallest one is guaranteed to be.
func TestNoPageEverRunsPastTheEdge(t *testing.T) {
	sizes := [][2]int{{80, 24}, {100, 30}, {200, 60}, {34, 13}}
	pages := []func(*harness){
		func(h *harness) {},                                                                   // the preset page
		func(h *harness) { h.down().enter() },                                                 // the first question
		func(h *harness) { h.down().enter().typeIn("moritz").enter() },                        // a list of answers
		func(h *harness) { h.down().enter().typeIn("moritz").enter().enter() },                // the hub
		func(h *harness) { h.down().enter().typeIn("moritz").enter().enter().down().enter() }, // settings
		func(h *harness) { h.down().enter().typeIn("moritz").enter().enter().enter() },        // the confirmation
		func(h *harness) { h.down().enter().typeIn("moritz").enter().enter().typeIn("q") },    // the way out
	}
	for _, open := range pages {
		h := newHarness(t, leaveTree("true", "true"))
		open(h)
		for _, size := range sizes {
			h.send(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			for _, line := range strings.Split(h.screen(), "\n") {
				if w := lipgloss.Width(line); w > size[0] {
					t.Fatalf("at %dx%d a line is %d wide:\n%s", size[0], size[1], w, line)
				}
			}
		}
	}
}

// A failure report is the one page that has to be readable on the narrowest
// terminal there is: it is what somebody photographs and sends to a forum.
func TestAFailureReportFitsTheSmallestTerminal(t *testing.T) {
	h := newHarness(t, map[string]string{
		"tasks/@go/a-first/task.sh": "echo starting\nls /definitely/not/here\necho never\n",
	})
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter().enter().typeIn("x").enter().typeIn("x").enter()
	h.ran().enter()
	h.send(tea.WindowSizeMsg{Width: 80, Height: 24})
	view := h.screen()
	for _, want := range []string{"Module", "Task", "Script", "Command", "Exit code"} {
		if !strings.Contains(view, want) {
			t.Errorf("the report is missing %q at 80x24:\n%s", want, view)
		}
	}
}

// The smallest module that is still an installer: some questions and something to
// do. Everything else the runtime offers — a language to pick, a starting
// point, a task that asks first — is a page that simply does not appear.
func TestTheSmallestTreeStillWorks(t *testing.T) {
	h := newHarness(t, map[string]string{
		treeFile:                       "title: Test Installer\nstages: [go]\nvariables:\n  - name: USER\n    title: User name\n    required: true\n",
		"tasks/@go/a-first/task.yaml":  "title: Do it\n",
		"tasks/@go/b-second/task.yaml": "",
		"tasks/@go/b-second/task.sh":   "",
		"tasks/@go/c-extras/task.yaml": "",
		"tasks/@go/c-extras/task.sh":   "",
	})
	// Straight to the one question: no preset page, because there are no presets.
	h.wants("User name", "1 of 1")
	h.typeIn("moritz").enter()
	h.wants("Test Installer", "Settings")

	// No confirmation sentence to show, and no secret to ask for.
	h.enter().wants("Ready to start", "Start Test Installer")
	h.enter().ran()
	h.wants("Test Installer complete", "Do it")

	// Nothing follows a finished installation: enter on the result leaves.
	h.enter()
	if !h.m.quitting {
		t.Error("enter on a finished installation did not leave")
	}
}

// ─── Leaving ─────────────────────────────────────────────────────────────────

// A module that says how this machine is put down is a module saying the installer
// cannot simply be quit: the machine booted to run it and there is nothing
// behind it to quit into.
func leaveTree(restart, shutdown string) map[string]string {
	return map[string]string{
		"hooks/" + spec.HookRestart + "/reboot/hook.yaml":    "title: Reboot\nscript: " + restart + "\n",
		"hooks/" + spec.HookShutdown + "/poweroff/hook.yaml": "title: Power off\nscript: " + shutdown + "\n",
	}
}

func TestQuittingAsksWhatToDoWithTheMachine(t *testing.T) {
	h := newHarness(t, leaveTree("true", "true"))
	h.down().enter().typeIn("moritz").enter().enter()
	h.wants("Test Installer", "Settings")

	h.typeIn("q")
	h.wants("Restart", "Shut down").refuses("Exit")
	if h.m.quitting {
		t.Fatal("q left the program instead of asking")
	}

	// And it is a question like any other: esc is the way back to the hub.
	h.esc()
	h.wants("Test Installer", "Settings")
}

// Where the module says there is a console behind the installer, there is a third
// way out: the program stops and the machine keeps running. What it leaves on
// the terminal is the module's own sentence, because a bare prompt says nothing
// about how to get back.
func TestLeavingToTheConsoleClosesOnlyTheProgram(t *testing.T) {
	const back = "Type installer to start it again."
	files := leaveTree("true", "true")
	files[treeFile] = testInstaller + "console: " + back + "\n"

	h := newHarness(t, files)
	h.down().enter().typeIn("moritz").enter().enter()
	h.typeIn("q")
	h.wants("Restart", "Shut down", "Exit")

	// The sentence belongs to the row, so it is under the list once the cursor
	// is on it.
	h.down().down()
	h.wants(back)

	h.enter()
	if !h.m.quitting {
		t.Fatal("choosing the console did not leave the program")
	}
	if h.a.farewell != back {
		t.Errorf("farewell = %q, want the sentence the module wrote", h.a.farewell)
	}
}

func TestChoosingRestartRunsTheTreesOwnCommand(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "restarted")
	h := newHarness(t, leaveTree("touch "+marker, "true"))
	h.down().enter().typeIn("moritz").enter().enter()
	h.typeIn("q").enter()

	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("the machine was never restarted: %v", err)
	}
	if !h.m.quitting {
		t.Error("the interface stayed up after the machine was put down")
	}
}

// A command that does not work leaves the machine running, so the page has to
// stay standing and say so rather than closing into a terminal nobody is
// looking at.
func TestAMachineThatWillNotRestartIsSaidSo(t *testing.T) {
	h := newHarness(t, leaveTree("exit 1", "true"))
	h.down().enter().typeIn("moritz").enter().enter()
	h.typeIn("q").enter()

	if h.m.quitting {
		t.Fatal("the interface left although the machine is still running")
	}
	h.wants("did not respond", "Restart", "Shut down")
}

// Nothing follows a finished installation but the machine being put down.
func TestAFinishedInstallationEndsOnTheWayOut(t *testing.T) {
	h := newHarness(t, leaveTree("true", "true"))
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter().enter()
	h.typeIn("x").enter().typeIn("x").enter()
	h.ran()
	h.wants("Test Installer complete")

	h.enter()
	h.wants("Restart", "Shut down")
	if h.m.quitting {
		t.Error("a finished installation left rather than asking")
	}
}

// ctrl+c and esc during a run ask how to leave; neither stops it. The page is
// drawn over the run, the run carries on behind it, and going back lands on an
// installation that never noticed.
func TestAskingToLeaveDuringARunDoesNotStopIt(t *testing.T) {
	files := leaveTree("true", "true")
	files["tasks/@go/a-first/task.sh"] = "sleep 30\n"
	h := newHarness(t, files)
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter().enter()
	h.typeIn("x").enter().typeIn("x").enter()
	h.wants("Test Installer · 00:0")

	h.esc()
	h.wants("Restart", "Shut down", "continues behind this page")
	if !working(h.m.top()) {
		t.Error("the run was stopped by somebody asking how to leave it")
	}

	h.esc()
	h.wants("Test Installer · 00:0").refuses("Restart")

	h.ctrlC()
	h.wants("Restart", "Shut down")
}

// And choosing one of its rows is what does stop it: from there on this is a
// decision rather than a question, and a package transaction writing to a disk
// nobody is watching any more is worse than an interrupted one.
func TestChoosingAWayOutStopsTheRun(t *testing.T) {
	files := leaveTree("true", "true")
	files["tasks/@go/a-first/task.sh"] = "sleep 30\n"
	h := newHarness(t, files)
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter().enter()
	h.typeIn("x").enter().typeIn("x").enter()
	h.wants("Test Installer · 00:0")

	run, ok := h.m.top().(*runScreen)
	if !ok || run.session == nil {
		t.Fatalf("nothing is running to be stopped; the page on top is %T", h.m.top())
	}
	// Held on to here: the page lets go of it the moment it is stopped, which is
	// the thing being watched for.
	session := run.session

	h.ctrlC().enter() // Restart
	select {
	case <-session.Done():
	case <-time.After(5 * time.Second):
		t.Error("the task was left running after a way out had been chosen")
	}
	if !h.m.quitting {
		t.Error("the machine was restarted and the program stayed up")
	}
}

// The clock is the whole point of the headline: an installation is minutes of a
// list filling in, and how long it has been going is the one thing nobody
// watching can work out for themselves.
func TestTheHeadlineCarriesTheClock(t *testing.T) {
	h := newHarness(t, nil)
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter().enter()
	h.typeIn("x").enter().typeIn("x").enter()
	h.ran()
	h.wants("Test Installer complete in 00:0")
}

func TestClockReadsAsAClock(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "00:00"},
		{9 * time.Second, "00:09"},
		{8*time.Minute + 21*time.Second, "08:21"},
		{59*time.Minute + 59*time.Second, "59:59"},
		{time.Hour + 5*time.Minute + 3*time.Second, "1:05:03"},
		{-time.Second, "00:00"},
	}
	for _, c := range cases {
		if got := clock(c.d); got != c.want {
			t.Errorf("clock(%s) = %q, want %q", c.d, got, c.want)
		}
	}
}

// ─── The same keys on every page ─────────────────────────────────────────────

// Four keys, one meaning each, wherever they are pressed: enter says yes, esc
// and backspace say back, and q and ctrl+c ask to leave the program. What
// follows is that promise, page by page.

// Backspace is back, exactly where esc is.
func TestBackspaceGoesBackWhereEscDoes(t *testing.T) {
	h := newHarness(t, nil)
	h.down().enter().typeIn("moritz").enter()
	h.wants("2 of 2")
	h.erase()
	h.wants("User name", "1 of 2")
}

// Except in front of a text box, where it is the delete key first and only
// means back once there is nothing left to delete.
func TestBackspaceDeletesBeforeItGoesBack(t *testing.T) {
	h := newHarness(t, nil)
	h.down().enter()
	h.typeIn("moritz").erase()
	h.wants("morit").refuses("moritz")

	h.erase().erase().erase().erase().erase()
	h.wants("User name")
	h.erase()
	h.wants("Setup", "Full", "Bare")
}

// The question of which module to open is a page like any other: it was pushed
// onto the language, so esc lands back on it.
func TestTheQuestionOfWhichModuleIsBackedOutOfLikeAnyOther(t *testing.T) {
	mods := both(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "de.po"), []byte("msgid \"What to do\"\nmsgstr \"Was tun\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := startIn(t, dir, mods...)
	h.enter() // English, and on to the question of which module
	h.wants("What to do", "Test Installer")
	h.esc()
	h.wants("Welcome", "English")
}

// q asks to leave from wherever it is pressed, not only from the menu — and
// what it opens is drawn over the page rather than instead of it, so either
// key that means back lands on exactly what was there before.
func TestQAsksToLeaveFromAnyPageAndComesBackToIt(t *testing.T) {
	h := newHarness(t, leaveTree("true", "true"))
	h.down().enter().typeIn("moritz").enter()
	h.wants("Disk", "/dev/sda")

	h.typeIn("q")
	h.wants("Restart", "Shut down")
	h.esc()
	h.wants("Disk", "/dev/sda", "2 of 2")

	// And backspace closes it again just as esc does.
	h.ctrlC()
	h.wants("Restart", "Shut down")
	h.erase()
	h.wants("Disk", "/dev/sda")
}

// Wherever something is being typed, q is a letter. A page that took it for the
// way out would be a program that cannot be told about a user called quinn.
func TestQIsACharacterWhereSomethingIsBeingTyped(t *testing.T) {
	h := newHarness(t, leaveTree("true", "true"))
	h.down().enter()
	h.typeIn("quinn")
	h.refuses("Restart", "Shut down")
	h.enter()
	if got := h.a.store.Get("USER"); got != "quinn" {
		t.Errorf("USER = %q, want the letter typed rather than the way out", got)
	}

	// The same in a narrowing box, which is a text box the moment it is open.
	h.enter()        // the disk, and the hub follows
	h.down().enter() // settings
	h.typeIn("/q")
	h.wants("No matches").refuses("Restart", "Shut down")

	// And in a password, which may hold any letter there is.
	h.esc().esc()            // close the box, then leave settings
	h.key(tea.KeyUp).enter() // the install row, and the warning it opens
	h.enter()                // start, which asks for the password first
	h.typeIn("q")
	h.wants("Password").refuses("Restart", "Shut down")
}

// A question a run stopped for has no page behind it — the task waiting on the
// answer has already started — so back means the same thing there that ctrl+c
// means everywhere, and the run is still standing on it afterwards.
func TestAQuestionInARunIsLeftRatherThanBackedOutOf(t *testing.T) {
	files := leaveTree("true", "true")
	files["tasks/@finish/d-reboot/task.yaml"] = "title: Reboot\nconfirm: Restart now?\n"
	files["tasks/@finish/d-reboot/task.sh"] = "echo never\n"
	h := newHarness(t, files)
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter().enter()
	h.typeIn("x").enter().typeIn("x").enter()

	h.asked()
	h.wants("Restart now?", "Yes", "No")

	h.erase()
	h.wants("Leave", "Shut down")
	h.esc()
	h.wants("Restart now?", "Yes", "No")
}

// ─── What a run has to report ────────────────────────────────────────────────

// A run of two dozen identical-looking rows cannot say that the work is done
// and everything after it is an offer. So a task may stop the run and say it,
// once, on a page of its own — with whatever it produced drawn as a code for
// the machine in somebody's hand.
func TestATaskCanReportWhatItProduced(t *testing.T) {
	h := newHarness(t, map[string]string{
		treeFile: testInstaller + `
  - name: LINK
    title: Shared at
`,
		"tasks/@finish/d-share/task.yaml": "title: Share\nshows: LINK\nreport: |\n  Installed on {{DISK}}\n\n  Everything after this is offered rather than needed.\n",
		// A script answers by writing one line of the answer file, which is the
		// only channel there is and the same one a person editing it uses.
		"tasks/@finish/d-share/task.sh": `printf "LINK='https://example.test/abc'\n" >>"$MODULE_CONF"` + "\n",
	})
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter().enter()
	h.typeIn("x").enter().typeIn("x").enter()

	h.reported()
	// The words are the module's, filled in from the answers, and the value the
	// task wrote is on the page as itself.
	h.wants("Installed on /dev/sda", "Everything after this is offered", "https://example.test/abc")
	// A page being read is not a run in progress: no counter, no turning mark.
	h.refuses("of 3")

	// And it is an answer like any other from here on.
	if got := h.a.store.Get("LINK"); got != "https://example.test/abc" {
		t.Errorf("LINK = %q, want the address the task wrote", got)
	}

	h.enter()
	h.ran()
	h.wants("Test Installer complete", "Share")
}

// A task that produced nothing still says what it has to say. Not being able to
// share a configuration is not a reason to withhold the news that the machine
// is installed.
func TestAReportWithNothingToShowIsStillShown(t *testing.T) {
	h := newHarness(t, map[string]string{
		treeFile: testInstaller + `
  - name: LINK
    title: Shared at
`,
		"tasks/@finish/d-share/task.yaml": "title: Share\nshows: LINK\nreport: Installed on {{DISK}}\n",
		"tasks/@finish/d-share/task.sh":   "echo 'it did not work' >&2\n",
	})
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter().enter()
	h.typeIn("x").enter().typeIn("x").enter()

	h.reported()
	h.wants("Installed on /dev/sda")
}

// ─── What a run proved about itself ──────────────────────────────────────────

// installed drives the whole opening and leaves the run finished and
// answerable, which is where every test in this section starts.
func installed(t *testing.T, files map[string]string) *harness {
	t.Helper()
	h := newHarness(t, files)
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter().enter().typeIn("x").enter().typeIn("x").enter()
	return h.ran()
}

// A check runs after the task it belongs to, on the machine that task worked
// on, and what they all came to is one page at the end of the run.
func TestTestsThatPassAreCountedAndNothingMore(t *testing.T) {
	h := installed(t, map[string]string{
		"tasks/@go/a-first/task.yaml":  "title: First\ntest: \"true\"\n",
		"tasks/@go/b-second/task.yaml": "title: Second\ntest: \"true\"\n",
	})
	// One line under the run, and no page: a run that agreed with itself has
	// nothing anybody could open.
	h.wants("Test Installer complete", "2 of 2 tests passed")
	h.enter()
	if !h.m.quitting {
		t.Error("a run that agreed with itself stopped on a page")
	}
}

// A check that disagrees is a thing to look at, not a reason to abandon an
// installation that is already on the disk.
func TestAFailedTestDoesNotStopTheRun(t *testing.T) {
	h := installed(t, map[string]string{
		"tasks/@go/a-first/task.yaml": "title: First\ntest: ./check.sh\n",
		"tasks/@go/a-first/check.sh":  "echo the disk is empty >&2\nexit 1\n",
	})
	h.wants("Test Installer complete", "First", "0 of 1 tests passed")
	h.refuses("failed")

	h.enter().wants("Validation", "0 of 1 tests passed", "First")
}

// Opening one is the whole point of the list: what somebody needs from here is
// the file and the line, laid out exactly as a failed run's is.
func TestAFailedTestOpensOnWhereItBroke(t *testing.T) {
	h := installed(t, map[string]string{
		"tasks/@go/a-first/task.yaml":  "title: First\ntest: \"true\"\n",
		"tasks/@go/b-second/task.yaml": "title: Second\n",
		"tasks/@go/b-second/test.sh":   "echo starting\nls /definitely/not/here\n",
	})
	h.enter().wants("1 of 2 tests passed", "Second")
	h.enter().wants("Module", "Task", "Second", "Script", "Exit code")
	// Back to the list, and on from the row that says so. Esc does nothing on
	// either page: this is the only place these failures are ever laid out.
	h.enter().wants("1 of 2 tests passed")
	h.esc().wants("1 of 2 tests passed")
	h.down().enter()
	if !h.m.quitting {
		t.Error("the row that leaves the validation page did not leave")
	}
}

// A test is judged by its exit status, whichever way it leaves — a failing
// command, or a plain `return 1`. A task's own work is the one thing whose
// final status is dropped, and getting the two mixed up made a failing test
// read as a passing one.
func TestATestThatReturnsNonZeroIsAFailure(t *testing.T) {
	h := installed(t, map[string]string{
		"tasks/@go/a-first/task.yaml":  "title: First\ntest: \"true\"\n",
		"tasks/@go/b-second/task.yaml": "title: Second\ntest: return 1\nreport: Installed\n",
		"tasks/@go/b-second/task.sh":   "true\n",
	})
	h.wants("Installed", "1 of 2 tests passed")
}

// And in the file a task keeps it in, which is where it is actually written.
func TestATestFileThatReturnsNonZeroIsAFailure(t *testing.T) {
	h := installed(t, map[string]string{
		"tasks/@go/a-first/task.yaml":  "title: First\ntest: \"true\"\n",
		"tasks/@go/b-second/task.yaml": "title: Second\nreport: Installed\n",
		"tasks/@go/b-second/task.sh":   "true\n",
		"tasks/@go/b-second/test.sh":   "return 1\n",
	})
	h.wants("Installed", "1 of 2 tests passed")
}

// Where a report says something disagreed, the page after it is the list — and
// the run carries on into whatever it was going to offer next once that page is
// left. That is the whole point of putting it there: a run whose last offer is
// a restart is a run most people never see the end of.
func TestAReportWithAFailedTestIsFollowedByTheList(t *testing.T) {
	h := installed(t, map[string]string{
		"tasks/@go/a-first/task.yaml":  "title: First\ntest: \"true\"\n",
		"tasks/@go/b-second/task.yaml": "title: Second\nreport: Installed\n",
		"tasks/@go/b-second/task.sh":   "true\n",
		"tasks/@go/b-second/test.sh":   "echo the disk is empty >&2\nls /definitely/not/here\n",
		"tasks/@go/c-extras/task.yaml": "title: Share this configuration\nconfirm: Put these answers online?\n",
		"tasks/@go/c-extras/task.sh":   "true\n",
	})
	h.wants("Installed", "1 of 2 tests passed")

	// The list, then the one failure on it: which module, which task, which
	// file and line, and what the tool said.
	h.enter().wants("Validation", "1 of 2 tests passed", "Second")
	// Where first, then what the tool said.
	h.enter().wants("Module", "Task", "Second", "Script", "test.sh", "Exit code", "the disk is empty")
	h.enter().wants("Validation", "Second")

	// And on into the offer the run was going to make anyway, once the row that
	// leaves this page has been chosen.
	h.down().enter().wants("Put these answers online?")
}

// Offered once: it is a fact about the run rather than about the moment, so the
// next page that stops for something does not put it up again.
func TestTheListOfFailedTestsIsOfferedOnce(t *testing.T) {
	h := installed(t, map[string]string{
		"tasks/@go/a-first/task.yaml":  "title: First\ntest: return 1\nreport: Installed\n",
		"tasks/@go/a-first/task.sh":    "true\n",
		"tasks/@go/b-second/task.yaml": "title: Second\nreport: Shared\n",
		"tasks/@go/b-second/task.sh":   "true\n",
	})
	h.wants("Installed")
	h.enter().wants("Validation")
	// The last row leaves it; enter on any other opens the failure under it.
	h.down().enter().wants("Shared", "0 of 1 tests passed")
	// On to the end of the run, which counts them again and offers nothing.
	h.enter().wants("Test Installer complete", "0 of 1 tests passed").refuses("Validation")
	h.enter()
	if !h.m.quitting {
		t.Error("the list was put up a second time")
	}
}

// And where a report has nothing to report about the tests, the run goes
// straight on to what it was going to offer next.
func TestAReportWithNoFailedTestGoesStraightOn(t *testing.T) {
	h := installed(t, map[string]string{
		"tasks/@go/a-first/task.yaml":  "title: First\ntest: \"true\"\nreport: Installed\n",
		"tasks/@go/a-first/task.sh":    "true\n",
		"tasks/@go/b-second/task.yaml": "title: Share this configuration\nconfirm: Put these answers online?\n",
	})
	h.wants("Installed", "1 of 1 tests passed")
	h.enter().wants("Put these answers online?").refuses("Validation")
}

// A run with nothing to test says nothing about testing and never stops on a
// page about it.
func TestARunWithNoTestsReportsNone(t *testing.T) {
	h := installed(t, nil)
	h.enter()
	if !h.m.quitting {
		t.Error("enter on a run with no checks did not leave")
	}
}

// A simulated run runs its tests like any other. What a run that changed
// nothing has to say about itself is the script's decision, not the runtime's:
// it is handed DEBUG and guards itself, exactly as the task it belongs to does.
func TestASimulatedRunStillRunsItsTests(t *testing.T) {
	h := newSimulated(t, map[string]string{
		"tasks/@go/a-first/task.yaml": "title: First\ntest: '[ \"$DEBUG\" = true ]'\n",
	})
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter().enter().typeIn("x").enter().typeIn("x").enter()
	h.ran().wants("1 of 1 tests passed")
}

// The switch is the runtime's own, and it holds for every module: turned off,
// nothing is run and there is nothing to report.
func TestValidationCanBeSwitchedOff(t *testing.T) {
	files := map[string]string{
		"tasks/@go/a-first/task.yaml": "title: First\ntest: \"false\"\n",
	}
	h := newHarness(t, files)
	h.a.prefs.SetValidates(false)
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter().enter().typeIn("x").enter().typeIn("x").enter()
	h.ran().enter()
	if !h.m.quitting {
		t.Error("a run with validation off stopped on the validation page")
	}
}

// And it is offered only where the module has something to check: a switch for
// a thing that would never happen is a row that reads as a promise nothing
// keeps.
func TestTheValidationSettingIsOfferedOnlyWhereThereIsSomethingToTest(t *testing.T) {
	h := newHarness(t, map[string]string{
		"tasks/@go/a-first/task.yaml": "title: First\ntest: \"true\"\n",
	})
	h.down().enter().typeIn("moritz").enter().enter()
	h.down().enter().wants("Validate", "Installation scripts", "Yes")

	plain := newHarness(t, nil)
	plain.down().enter().typeIn("moritz").enter().enter()
	plain.down().enter().refuses("Validate", "Installation scripts")
}

// Turning it off from the settings page is what the switch is for, and the row
// says so on the way back.
func TestTheValidationSettingTurnsTestsOff(t *testing.T) {
	h := newHarness(t, map[string]string{
		"tasks/@go/a-first/task.yaml": "title: First\ntest: \"true\"\n",
	})
	h.down().enter().typeIn("moritz").enter().enter()
	h.down().enter().enter()              // Settings, then the validation row
	h.wants("Installation scripts", "No") // the page, opened on Yes with No under it
	h.down().enter()
	h.wants("Installation scripts", "No")
	if h.a.prefs.Validates() {
		t.Error("the switch was answered No and validation is still on")
	}
}

// ─── A starting point that is fetched rather than written down ───────────────

// The third kind of starting point: not a set of answers in the module but a code
// somebody was handed, and the answers behind it. One row, one question, and
// from the next page on nothing about it is any different.
func TestAPresetCanFetchItsAnswers(t *testing.T) {
	h := newHarness(t, presetFetches("printf \"USER='moritz'\\nDISK='/dev/sdb'\\nEXTRAS='false'\\n\" >>\"$MODULE_CONF\""))
	h.down().down()
	h.wants("Online")

	// An answer has already been written down once, which is what a real run
	// looks like by the time it reaches this page — so the file carries an empty
	// line for the code, and reading it back would undo the answer about to be
	// given unless that answer is written down first.
	if err := h.a.store.Save(); err != nil {
		t.Fatal(err)
	}

	h.enter()
	h.wants("Configuration code")
	h.typeIn("abc12").enter().answered()

	// Everything the code stood for is an answer now, and with nothing left
	// open the hub is what follows.
	h.wants("Test Installer", "Settings")
	for name, want := range map[string]string{"USER": "moritz", "DISK": "/dev/sdb", "EXTRAS": "false"} {
		if got := h.a.store.Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	if got := h.a.store.Get("SOURCE"); got != "abc12" {
		t.Errorf("SOURCE = %q, want abc12", got)
	}
}

// A code that stands for nothing is a page that has not moved, with the reason
// on it in the shell's own words — there is nothing to do about it but read
// that and try another.
func TestAPresetThatCannotFetchSaysWhyAndStaysPut(t *testing.T) {
	h := newHarness(t, presetFetches("echo 'Nothing is shared under that code' >&2; exit 1"))
	h.down().down().enter()
	h.typeIn("nope").enter().answered()

	h.wants("Configuration code", "Nothing is shared under that code")
	h.refuses("Settings")
	if _, ok := h.m.top().(*fieldScreen); !ok {
		t.Errorf("the page moved on to %T", h.m.top())
	}
}

// presetFetches is the test module with a third starting point on it: one that
// asks for a code and runs the given shell to make something of it.
func presetFetches(apply string) map[string]string {
	declared := strings.Replace(testInstaller, "\nvariables:", `
      - title: Online
        description: Take the answers from somewhere else.
        asks: SOURCE
        apply: `+apply+`
variables:`, 1)
	return map[string]string{treeFile: declared + `
  - name: SOURCE
    title: Configuration code
    description: The code of a configuration somebody shared.
    required: true
`}
}

// ─── Failures are not closed by accident ─────────────────────────────────────

// The page a failed check is laid out on is the only account of it this run
// gives: nothing reopens it, and a run that has just gone wrong is exactly when
// somebody reaches for the key that means back. So it answers to yes and to
// nothing else.
func TestAFailureIsNotClosedByTheKeyThatMeansBack(t *testing.T) {
	h := installed(t, map[string]string{
		"tasks/@go/a-first/task.yaml":  "title: First\ntest: \"true\"\n",
		"tasks/@go/b-second/task.yaml": "title: Second\n",
		"tasks/@go/b-second/test.sh":   "ls /definitely/not/here\n",
	})
	h.enter().wants("Validation", "Second")
	h.enter().wants("Module", "Task", "Second", "Exit code")

	for _, press := range []func() *harness{h.esc, h.erase, h.down} {
		press().wants("Module", "Task", "Second", "Exit code")
	}
	h.enter().wants("Validation", "Second")
}

// And the list they are laid out on is left by choosing the row that says so,
// not by a keystroke that means something else everywhere else in the program.
func TestTheListOfFailuresIsLeftByTheRowThatSaysSo(t *testing.T) {
	h := installed(t, map[string]string{
		"tasks/@go/a-first/task.yaml":  "title: First\ntest: \"true\"\n",
		"tasks/@go/b-second/task.yaml": "title: Second\nreport: Installed\n",
		"tasks/@go/b-second/task.sh":   "true\n",
		"tasks/@go/b-second/test.sh":   "ls /definitely/not/here\n",
		"tasks/@go/c-extras/task.yaml": "title: After\nconfirm: Carry on?\n",
		"tasks/@go/c-extras/task.sh":   "true\n",
	})
	h.wants("Installed", "1 of 2 tests passed")
	h.enter().wants("Validation", "Second", "Continue")

	// Every way of saying back leaves the page standing.
	h.esc().wants("Validation", "Second")
	h.erase().wants("Validation", "Second")

	// The row says what it costs, and choosing it is what moves the run on.
	h.down().wants("not shown again")
	h.enter().wants("Carry on?")
}
