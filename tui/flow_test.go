package tui

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/murkl/oak/internal/i18n"
	"github.com/murkl/oak/internal/runner"
	"github.com/murkl/oak/internal/spec"
	"github.com/murkl/oak/internal/store"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// What a test module's declaration is called: the runtime takes whichever yaml
// it finds in the folder, and these tests use the name the real modules use.
const treeFile = spec.FileModule

// A whole program, driven by keystrokes against a folder written for the test,
// and asserted on what a person would see. So a refactor that keeps the
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

	// wake is told whenever a command ends, with a message or without one: a
	// loop waiting on the messages alone sits out its whole patience for a
	// command that answered nothing.
	wake chan struct{}

	// gap, where a test sets it, runs between finding the queue empty and
	// counting what is out there: where a command's message can still slip in.
	gap func()
}

// reallyStart is the last page before the work.
const reallyStart = "Do you really want to start?"

// The module every flow test starts from: two starting points, a handful of
// questions, three tasks, one of them conditional.
const testInstaller = `
title: Test Installer
stages: [go, finish]
confirm: true
presets:
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
    type: text
    title: User name
    description: The account you log in with.
    group: Identity
    required: true
    pattern: '^[a-z]+$'
    error: Lower case letters only.
  - name: PW
    title: Password
    type: new-password
    required: true
  - name: DISK
    type: list
    title: Disk
    group: Storage
    required: true
    options-from: disks()
  - name: EXTRAS
    title: Extras
    group: Storage
    type: bool
  - name: DRIVER
    type: list
    title: Driver
    options: [mesa, nvidia]
    required: true
    conditions: EXTRAS == true
`

// The product's library the flow tests run with, holding what the declarations
// call. The harness keeps it in the module's folder; a product keeps it beside
// oak.yaml.
const testShell = `disks() { printf '/dev/sda\t/dev/sda  1TB\n/dev/sdb\t/dev/sdb  2TB\n'; }
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

// writeModule puts one module on disk - the standard one, with whatever a test
// changed about it - and answers with the folder it went into.
func writeModule(t *testing.T, dir string, files map[string]string) string {
	t.Helper()
	base := map[string]string{treeFile: testInstaller, spec.FileRuntimeShell: testShell}
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

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func loadModule(t *testing.T, dir string) *spec.Module {
	t.Helper()
	mod, err := spec.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if shell := filepath.Join(dir, spec.FileRuntimeShell); fileExists(shell) {
		mod.Shell = shell
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
		// discovers them - so a test that writes one is testing what ships.
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
//
// A fresh one each time rather than one shared: a test about what the product
// says of itself says it here, and says it for its own run only.
func testRuntime() *spec.Runtime {
	return &spec.Runtime{Title: "Test OS", Modules: []string{"installer", "recovery"}}
}

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
	return startWith(t, testRuntime(), openModule(t), locales, mods...)
}

// startWith is the same again for a run opened some other way - with --debug,
// for the tests that are about what a script is handed, and for the pages that
// are about the product rather than about any of its modules.
func startWith(t *testing.T, rt *spec.Runtime, open Open, locales string, mods ...*spec.Module) *harness {
	t.Helper()
	return startAs(t, rt, open, locales, Opening{}, mods...)
}

// startAs is the same for a run whose command line said something about the
// run itself: the language it is read in, or that it is a kiosk.
func startAs(t *testing.T, rt *spec.Runtime, open Open, locales string, line Opening, mods ...*spec.Module) *harness {
	t.Helper()
	i18n.Use(i18n.SourceLang)
	a := &app{
		runtime: rt, modules: mods, open: open, version: "test",
		prefs:   store.NewPreferences(filepath.Join(t.TempDir(), "runtime.conf")),
		settled: line.Settled, kiosk: line.Kiosk,
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
	h := &harness{t: t, a: a, msgs: make(chan tea.Msg, 64), wake: make(chan struct{}, 1)}
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
	return startWith(t, testRuntime(), openModuleIn(t, true), "", loadModule(t, writeModule(t, t.TempDir(), files)))
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
		defer func() {
			h.inflight.Add(-1)
			select {
			case h.wake <- struct{}{}:
			default:
			}
		}()
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
}

// quiet is how long the loop waits on a command that is still out there while a
// task runs, before it takes the interface to have settled anyway. A task is a
// real process, and a test watching a run needs the screen while that is still
// going, so it is the one thing here deliberately not waited out.
const quiet = 60 * time.Millisecond

// patience bounds the wait for a command that never answers. A page answers in
// microseconds, but under the race detector a key sent before its page arrived
// would be lost, so the page is waited for.
const patience = 20 * time.Second

// drain handles everything waiting, and everything that arrives while it is
// handling it, until nothing is left and nothing is still coming.
func (h *harness) drain() {
	for {
		var msg tea.Msg
		select {
		case msg = <-h.msgs:
		default:
			if h.gap != nil {
				h.gap()
			}
			// Nothing queued and nothing out there. A command is counted out
			// only once its message is queued, so one look more is the last.
			if h.inflight.Load() == 0 {
				select {
				case msg = <-h.msgs:
					h.handle(msg)
					continue
				default:
					return
				}
			}
			wait := patience
			if h.running() {
				wait = quiet
			}
			select {
			case msg = <-h.msgs:
			case <-h.wake:
				continue
			case <-time.After(wait):
				return
			}
		}
		h.handle(msg)
	}
}

// A command that ends without a message wakes the loop waiting on it, rather
// than leaving it to sit out its patience: on a busy machine that is what a
// command answering nothing looks like, and the suite spent minutes on it.
func TestTheLoopWakesWhenACommandAnswersNothing(t *testing.T) {
	h := newHarness(t, nil)
	start := time.Now()
	h.run(func() tea.Msg {
		time.Sleep(50 * time.Millisecond)
		return nil
	})
	h.drain()
	if waited := time.Since(start); waited > patience/2 {
		t.Fatalf("the loop waited %v for a command that answered nothing", waited)
	}
}

// A command queues its message and is counted out right after, so it can do
// both in the moment the loop finds the queue empty. That message is still
// handled before the loop says it has settled, or the next key meets the page
// before it.
func TestTheLoopHandlesAMessageThatArrivesAsItSettles(t *testing.T) {
	h := newHarness(t, nil)
	h.gap = func() {
		h.gap = nil
		h.msgs <- spinMsg{}
	}
	h.drain()
	if n := len(h.msgs); n != 0 {
		t.Fatalf("the loop settled with %d message(s) still queued", n)
	}
}

// running is whether a task is out there - anywhere on the stack, since a page
// asking to leave is drawn over a run that carries on behind it.
func (h *harness) running() bool {
	for _, s := range h.m.stack {
		if r, ok := s.(*runScreen); ok && r.session != nil && !r.done {
			return true
		}
	}
	return false
}

// handle is one message put through the program, exactly as bubbletea would.
func (h *harness) handle(msg tea.Msg) {
	switch msg := msg.(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			h.run(c)
		}
	case tickMsg, spinMsg, statusDueMsg, gateDueMsg:
	// A clock carries nothing and re-arms itself, so a test that wants one
	// sends it. The opening's runs out, and where it leaves the palette is
	// behaviour.
	default:
		if blink(msg) {
			return
		}
		m, cmd := h.m.Update(msg)
		h.m = m.(*Model)
		h.run(cmd)
	}
}

// blink reports whether a message is a text cursor asking to be redrawn - the
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
	h.t.Helper()
	h.send(tea.KeyMsg{Type: k})
	return h
}

func (h *harness) enter() *harness { h.t.Helper(); return h.key(tea.KeyEnter) }
func (h *harness) up() *harness    { return h.key(tea.KeyUp) }

// yes answers the page before the work, which opens on No.
func (h *harness) yes() *harness   { h.t.Helper(); return h.up().enter() }
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

// ─── What comes first ────────────────────────────────────────────────────────

// The one thing that cannot wait for the opening run of questions: a question
// marked `first` is asked before anything the work waits for, because a
// passphrase typed into an action is already typed on the keyboard this answer
// settles.
func TestAFirstQuestionIsAskedBeforeWhatTheWorkWaitsFor(t *testing.T) {
	tree := wireless(filepath.Join(t.TempDir(), "online"), "exit 0", true)
	tree[treeFile] = testInstaller +
		"  - name: LOCALE\n    type: list\n    title: Language and formats\n    required: true\n    first: true\n    options: [de, en]\n" +
		wirelessRules(true)
	h := newHarness(t, tree)
	h.wants("Language and formats", "de", "en").refuses("internet connection")

	// Answered, and only now is anything waited for: the network to join is
	// open, on top of the page saying there is none.
	h.enter()
	h.wants("Network", "HomeNet")
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
			"  - name: LOCALE\n    type: list\n    title: Language and formats\n    required: true\n    first: true\n" +
			"    options: [de, en]\n    apply: ./apply.sh\n",
		"apply.sh": "echo \"$LOCALE\" > " + loaded + "\n",
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
// a second start goes straight on to what the work waits for, exactly as it
// would if nothing had ever been marked.
func TestAnAnsweredFirstQuestionIsNotAskedAgain(t *testing.T) {
	tree := wireless(filepath.Join(t.TempDir(), "online"), "exit 0", true)
	tree[treeFile] = testInstaller +
		"  - name: LOCALE\n    type: list\n    title: Language and formats\n    required: true\n    first: true\n    default: de\n    options: [de, en]\n" +
		wirelessRules(true)
	h := newHarness(t, tree)
	h.wants("HomeNet").refuses("Language and formats")
}

// A question asked first is asked before loadkeys has run, so even the key that
// would normally open the filter is typed on a layout nobody has chosen yet.
// Its box is up from the first frame however short the list is, and typing
// narrows straight away - no / needed first, and nothing in the yaml to say so.
func TestAQuestionAskedFirstOpensItsFilterFromTheStart(t *testing.T) {
	h := newHarness(t, map[string]string{
		treeFile: testInstaller +
			"  - name: KEYMAP\n    type: list\n    title: Console keyboard\n    required: true\n    first: true\n    options: [us, de]\n",
	})
	h.wants("Console keyboard", "Filter …")
	h.typeIn("de")
	h.wants("de").refuses("us")
}

// ─── Which program ───────────────────────────────────────────────────────────

// A runtime offering more than one module asks which to open before anything
// follows from it - and what it asks with is entirely each module's own words.
// What is chosen then decides which questions there are, where the answers go
// and what the run is called from there on.
const testRecovery = `
title: Test Recovery
stages: [open]
variables:
  - name: DISK
    type: list
    title: Disk
    required: true
    options: [/dev/sda]
  - name: SNAPSHOT
    type: list
    title: Snapshot
    required: true
    options: [one, two]
`

// both is a folder holding two modules, the way a build leaves one: the
// standard installer, and a recovery beside it.
func both(t *testing.T) []*spec.Module {
	t.Helper()
	dir := t.TempDir()
	installer := writeModule(t, filepath.Join(dir, "installer"), map[string]string{
		treeFile: testInstaller,
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
	h.wants(forkQuestion, "Test Installer", "Test Recovery")

	h.down().enter() // the recovery
	h.wants("Disk").enter()
	h.wants("Snapshot", "one", "two").enter()

	// The frame carries that module's name from here on, and only its own tasks
	// run. It asks for no yes before them.
	h.wants("Test Recovery", glyphs.cursor+"Start").enter()
	h.ran()
	h.wants("Finished in", "Open the disk")
	h.refuses("First")
}

// The other module's questions are not this one's: they are declared in a folder
// this run never opened.
func TestTheOtherProgramsQuestionsAreNotAsked(t *testing.T) {
	h := start(t, both(t)...)
	h.enter() // the installer, the row the page opens on
	h.wants(labelPresets(), "Everything at once.")
	h.refuses("Snapshot")
}

// What the fork asks, and a catalog answering it in German: the question is
// read in the language the welcome page settled.
const forkQuestion = "What would you like to start?"

func forkCatalog(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	po := "msgid \"" + forkQuestion + "\"\nmsgstr \"Was tun\"\n"
	if err := os.WriteFile(filepath.Join(dir, "de.po"), []byte(po), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// The page in front of all of them stands under the wordmark with the welcome
// page rather than in the frame: the frame is titled after the module, and
// there is none yet. Choosing one is what opens it.
func TestTheQuestionOfWhichModuleStandsUnderTheWordmark(t *testing.T) {
	corner := lipgloss.NormalBorder().TopLeft
	h := start(t, both(t)...)
	h.m = newModel(h.a, testLogo)
	h.run(h.m.Init())
	h.drain()
	h.wants("TEST OS", forkQuestion, "Test Installer").refuses(corner)

	h.enter() // the installer
	h.wants(corner, testRuntime().Title+" "+glyphs.crumb+" Test Installer").refuses("TEST OS")
}

// Pushed after the welcome page rather than handed over by the splash, it
// still stands under the same wordmark, and its keys say there is a page
// behind it.
func TestTheQuestionOfWhichModuleStandsUnderTheWordmarkAfterTheWelcomePage(t *testing.T) {
	h := startIn(t, forkCatalog(t), both(t)...)
	h.m = newModel(h.a, testLogo)
	h.run(h.m.Init())
	h.drain()
	h.enter() // English
	h.wants("TEST OS", forkQuestion, "Test Installer", labelHintChoose())
}

// Where nothing was asked before it, the fork is the first page, and the way
// out of it is the way out of the program.
func TestTheQuestionOfWhichModuleIsLeftWithQWhereItComesFirst(t *testing.T) {
	h := start(t, both(t)...)
	h.wants(forkQuestion, labelHintMenu()).refuses(labelHintChoose())
}

// And whatever the terminal, it stays inside it.
func TestTheQuestionOfWhichModuleNeverRunsPastTheEdge(t *testing.T) {
	h := start(t, both(t)...)
	h.m = newModel(h.a, testLogo)
	h.run(h.m.Init())
	h.drain()
	for _, size := range [][2]int{{80, 24}, {100, 30}, {200, 60}, {34, 13}, {20, 6}} {
		h.send(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		lines := strings.Split(h.screen(), "\n")
		if len(lines) > size[1] {
			t.Errorf("at %dx%d the page is %d rows tall:\n%s", size[0], size[1], len(lines), h.screen())
		}
		for _, line := range lines {
			if w := lipgloss.Width(line); w > size[0] {
				t.Errorf("at %dx%d a line is %d wide:\n%s", size[0], size[1], w, line)
			}
		}
	}
}

// And the landing page comes in front of that: the words the rest is read in
// belong to the runtime rather than to any module, and the question of which
// module to open is itself read in them.
func TestTheLandingPageComesBeforeTheQuestionOfWhichModule(t *testing.T) {
	mods := both(t)
	dir := forkCatalog(t)
	h := startIn(t, dir, mods...)
	h.wants(landingChoose, "English").refuses(forkQuestion)

	h.down().enter() // Deutsch
	h.wants("Was tun", "Test Installer", "Test Recovery")
}

// The menu is home: it draws no trail of its own, and the pages it opens stand
// behind it.
func TestTheMenuHeadsTheTrailOfThePagesItOpensAndDrawsNoneItself(t *testing.T) {
	h := newHarness(t, nil)
	h.down().enter() // a starting point
	h.typeIn("moritz").enter().enter()
	h.wants("Start").refuses(labelMenu())

	h.down().enter()
	h.wants(labelMenu() + " " + glyphs.crumb + " " + rowSetup)
}

// A module has one name, and the frame carries it on every page. So the rows
// inside it are named after what pressing them does rather than after the
// module all over again.
func TestTheRowsInsideAModuleAreNamedAfterWhatTheyDo(t *testing.T) {
	h := newHarness(t, nil)
	h.down().enter() // a starting point
	h.typeIn("moritz").enter().enter()
	h.wants("Start", "Setup").refuses(glyphs.cursor + "Test Installer")
}

// menuWith is the menu of a module drawing the icon given, under a runtime
// drawing the one given; empty draws none.
func menuWith(t *testing.T, module, product string) *harness {
	t.Helper()
	head := "title: Test Installer"
	if module != "" {
		head += "\nicon: '" + module + "'"
	}
	tree := map[string]string{treeFile: strings.Replace(testInstaller, "title: Test Installer", head, 1)}
	rt := testRuntime()
	rt.Icon = product
	h := startWith(t, rt, openModule(t), "", loadModule(t, writeModule(t, t.TempDir(), tree)))
	h.down().enter().typeIn("moritz").enter().enter()
	return h
}

// body is what the frame holds under its rule, each line without the border
// and the padding.
func body(h *harness) []string {
	h.t.Helper()
	var out []string
	lines := strings.Split(h.screen(), "\n")
	ruled := func(line string) bool {
		return strings.HasPrefix(line, "│") && strings.Contains(line, glyphs.rule+glyphs.rule)
	}
	for i, line := range lines {
		if !ruled(line) {
			continue
		}
		for _, l := range lines[i+1:] {
			if ruled(l) {
				return out
			}
			r := []rune(strings.TrimSuffix(strings.TrimPrefix(l, "│"), "│"))
			out = append(out, string(r[padH:len(r)-padH]))
		}
	}
	h.t.Fatalf("no frame:\n%s", h.screen())
	return nil
}

// The icon over the menu is the module's, or else the runtime's, or else the
// tick.
func TestTheMenuIconIsTheModulesThenTheRuntimesThenTheTick(t *testing.T) {
	tick := strings.TrimSpace(glyphTick[2])

	menuWith(t, "", "").wants(tick)
	menuWith(t, "", "<A>").wants("<A>").refuses(tick)
	menuWith(t, "[M]", "<A>").wants("[M]").refuses("<A>")
}

// The menu is the icon and the two rows under it, a blank line apart, centred
// as one block between the frame's rules, and nothing else.
func TestTheMenuIsTheIconCentredOverItsRows(t *testing.T) {
	h := menuWith(t, "<A>", "")
	// An even room and an odd one: the row left over has a side to go to.
	for _, height := range []int{24, 25} {
		h.send(tea.WindowSizeMsg{Width: 80, Height: height})
		menuIsCentred(t, body(h))
	}
}

func menuIsCentred(t *testing.T, rows []string) {
	t.Helper()
	top := 0
	for top < len(rows) && strings.TrimSpace(rows[top]) == "" {
		top++
	}

	want := []string{"<A>", "", glyphs.cursor + "Start", "Setup"}
	if top+len(want) > len(rows) {
		t.Fatalf("the menu is not all there:\n%s", strings.Join(rows, "\n"))
	}
	for i, w := range want {
		if got := strings.TrimSpace(rows[top+i]); got != strings.TrimSpace(w) {
			t.Fatalf("row %d of the menu is %q, want %q:\n%s", i, got, strings.TrimSpace(w), strings.Join(rows, "\n"))
		}
	}
	for _, line := range rows[top+len(want):] {
		if strings.TrimSpace(line) != "" {
			t.Errorf("the menu shows %q under its rows", line)
		}
	}
	// A row left over goes under the block.
	if below := len(rows) - top - len(want); below-top > 1 || top > below {
		t.Errorf("the menu stands %d rows under the rule over it and %d over the one under it", top, below)
	}

	// The icon centres, and so do the titles under it: the cursor stands in the
	// margin in front of them.
	blank := strings.Repeat(" ", lipgloss.Width(glyphs.cursor))
	for _, block := range [][]string{rows[top : top+1], rows[top+2 : top+4]} {
		left, right := lipgloss.Width(block[0]), lipgloss.Width(block[0])
		for _, line := range block {
			line = strings.Replace(line, glyphs.cursor, blank, 1)
			left = min(left, lipgloss.Width(line)-lipgloss.Width(strings.TrimLeft(line, " ")))
			right = min(right, lipgloss.Width(line)-lipgloss.Width(strings.TrimRight(line, " ")))
		}
		if right-left > 1 || left > right {
			t.Errorf("%q stands %d cells from the left and %d from the right", strings.TrimSpace(block[0]), left, right)
		}
	}
}

// A page opened from the settings stands in the menu's trail, and so does the
// page of an action opened there.
func TestThePagesOfTheSettingsStandInTheMenusTrail(t *testing.T) {
	h := newHarness(t, twoLanguageTree())
	h.enter().enter()                         // English, Full
	h.typeIn("moritz").enter().enter()        // the user name, the disk
	h.enter().enter().wants("Start", "Setup") // the driver, the system language
	h.down().enter().enter()                  // the settings, the interface language
	h.wants(labelMenu() + " " + glyphs.crumb + " " + rowSetup + " " + glyphs.crumb + " " + labelLanguage())

	h = newHarness(t, wireless(filepath.Join(t.TempDir(), "online"), "exit 0", false))
	h.down().enter().typeIn("moritz").enter().enter()
	h.down().enter().enter() // the settings, the wireless network
	h.wants(labelMenu() + " " + glyphs.crumb + " " + rowSetup + " " + glyphs.crumb + " Wireless network " + glyphs.crumb + " Network")
}

// The last page before the work stands under the menu, like the passwords
// before it.
func TestTheConfirmationStandsUnderTheMenu(t *testing.T) {
	h := newHarness(t, nil)
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter().typeIn("x").enter().typeIn("x").enter()
	h.wants(labelMenu()+" "+glyphs.crumb+" "+labelConfirmation(), reallyStart)
}

// The frame is titled after the product on every page, and once a module is
// open, after that module too: the header says what this run is even once the
// page that named it has scrolled away.
func TestTheFrameIsTitledAfterTheProductAndTheModuleOnceOneIsOpen(t *testing.T) {
	h := start(t, both(t)...)
	h.down().enter() // the recovery
	h.wants("Disk").enter()
	h.wants("Snapshot").enter()
	h.wants(testRuntime().Title + " " + glyphs.crumb + " Test Recovery")
}

// ─── The header's status ─────────────────────────────────────────────────────

// statusTree is a module whose header keeps an eye on something, and whose check
// answers the way it is told to.
func statusTree(answer string) map[string]string {
	return map[string]string{
		treeFile:    testInstaller + "status:\n  check: ./status.sh\n  pass: Online\n  fail: Offline\n",
		"status.sh": answer + "\n",
	}
}

// Opposite the name, as the check last answered: Oak's own mark for yes or no,
// and the module's words for it.
func TestTheHeaderSaysWhatTheStatusCheckAnswered(t *testing.T) {
	online := newHarness(t, statusTree("exit 0"))
	online.wants(glyphs.on + " Online").refuses("Offline")

	offline := newHarness(t, statusTree("exit 1"))
	offline.wants(glyphs.off + " Offline").refuses("Online")
}

// It shares its place with the mark that turns while something runs, and with a
// page's own count: those are about what is happening now, and it is not.
func TestTheStatusGivesWayToWhatIsHappening(t *testing.T) {
	h := newHarness(t, statusTree("exit 0"))
	h.down().enter() // Bare, and on to the questions
	h.wants(labelCounter(1, 2)).refuses("Online")

	h.typeIn("moritz").enter().enter()
	h.wants("Online")
}

// ─── Starting points ─────────────────────────────────────────────────────────

// A starting point is only a starting point once: a machine that has answered
// before is not asked again, because every value it filled in is by then an
// ordinary answer somebody may have changed.
func TestAPresetIsOnlyOfferedOnce(t *testing.T) {
	h := newHarness(t, nil)
	h.wants(labelPresets(), "Full", "Bare")
	h.down().enter().typeIn("moritz").enter().enter()
	h.wants("Test Installer", "Setup")

	h.restart()
	h.wants("Test Installer", "Setup").refuses("Full", "Bare")
}

// A preset fills in answers, and an answer is an answer whether it was typed or
// chosen in one keypress. This module ties its words on screen to one of them and
// puts the other into effect on the machine, which are the two things an answer
// can do beyond being stored.
func presetTreeTying(apply string) map[string]string {
	declared := strings.Replace(testInstaller,
		"      EXTRAS: \"true\"\n",
		"      EXTRAS: \"true\"\n      LOCALE: de_DE\n", 1)
	return map[string]string{
		treeFile: declared +
			"  - name: LOCALE\n    type: list\n    title: System language\n    required: true\n    options: [de_DE, en_US]\n" + apply +
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
	tree := presetTreeTying("    apply: ./apply.sh\n")
	tree["apply.sh"] = "echo \"$LOCALE\" > " + loaded + "\n"
	h := newHarness(t, tree)
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
			"  - name: LOCALE\n    type: list\n    title: System language\n    required: true\n    options: [de_DE, en_US]\n",
		"locales/de.po": "msgid \"English\"\nmsgstr \"Deutsch\"\n\nmsgid \"Full\"\nmsgstr \"Vollständig\"\n",
	}
}

// And it is read before there is a language to read it in, so it is written in
// one and stays there: a catalog offering a translation of its words is never
// asked for one, and a machine that chose German last time opens on the same
// English page - the name of the module under the wordmark included.
func TestTheLandingPageIsNeverTranslated(t *testing.T) {
	tree := twoLanguageTree()
	tree["locales/de.po"] += "\nmsgid \"" + landingChoose + "\"\nmsgstr \"Bitte Sprache wählen:\"\n" +
		"\nmsgid \"" + landingHint + "\"\nmsgstr \"↑↓ bewegen · ⏎ bestätigen · q beenden\"\n" +
		"\nmsgid \"Test Installer\"\nmsgstr \"Test-Einrichtung\"\n"
	h := newHarness(t, tree)
	h.down().enter() // Deutsch
	h.restart()
	h.wants(landingChoose, landingHint, "Test Installer").refuses("Bitte", "bewegen", "Einrichtung")
}

// A product that draws a wordmark, for the pages that stand under one. Letters
// rather than block pixels, so what the tests read is what the logo says.
const testLogo = "Made for testing\n\nTEST OS\n"

// dressed is a run of a product with a wordmark: the splash comes up first and
// the welcome page stands under it, as it does on a real terminal.
func dressed(t *testing.T, files map[string]string) *harness {
	t.Helper()
	h := newHarness(t, files)
	h.m = newModel(h.a, testLogo)
	h.run(h.m.Init())
	h.drain()
	return h
}

// The welcome page stands under the wordmark the splash left, on the field
// rather than in the frame. What opens the frame is answering it.
func TestTheWelcomePageStandsUnderTheWordmarkRatherThanInTheFrame(t *testing.T) {
	corner := lipgloss.NormalBorder().TopLeft
	h := dressed(t, twoLanguageTree())
	h.wants("TEST OS", landingChoose, "English", landingHint).refuses(corner, labelOpening())

	h.enter() // English
	h.wants(corner, testRuntime().Title).refuses("TEST OS")
}

// The splash hands over without the wordmark moving: the page lays it out on the
// rows it will take, and only the version under it gives way to the question.
func TestTheWordmarkStaysWhereTheSplashLeftIt(t *testing.T) {
	h := newHarness(t, twoLanguageTree())
	h.a.version = "7.8.9"
	m := newModel(h.a, testLogo)
	m.width, m.height = 100, 30

	m.splash.elapsed = sweepFor // swept in, the one stretch left to run
	during := strings.Split(m.View(), "\n")
	run(m.splash)
	after := strings.Split(m.View(), "\n")

	if !slices.ContainsFunc(during, func(l string) bool { return strings.Contains(l, "7.8.9") }) {
		t.Fatalf("the splash does not say its version:\n%s", strings.Join(during, "\n"))
	}
	if strings.Contains(strings.Join(after, "\n"), "7.8.9") {
		t.Errorf("the version outstayed the splash:\n%s", strings.Join(after, "\n"))
	}
	for i, line := range during {
		if !strings.Contains(line, "TEST OS") && !strings.Contains(line, "Made for testing") {
			continue
		}
		if i >= len(after) || after[i] != line {
			t.Fatalf("the wordmark moved when the question arrived:\n%s\n---\n%s",
				strings.Join(during, "\n"), strings.Join(after, "\n"))
		}
	}
}

// A module named on the way in is said under the wordmark, since the page that
// would name it is never drawn. One chosen after the welcome page is not put
// back over it.
func TestTheWelcomePageNamesTheModuleOnlyWhereItWasSettledOnTheWayIn(t *testing.T) {
	dressed(t, twoLanguageTree()).wants("TEST OS", "Test Installer", landingChoose)

	mods := both(t)
	dir := forkCatalog(t)
	h := startIn(t, dir, mods...)
	h.wants(landingChoose).refuses("Test Installer", "Test Recovery")
	h.enter().enter() // English, then the installer
	h.esc().esc()
	h.wants(landingChoose).refuses("Test Installer")
}

// On a terminal too short for all of it, the wordmark gives way and the rows do
// not: a page that says whose it is with no languages under it asks a question
// it offers no way to answer.
func TestTheLanguagesOutliveTheWordmarkOverThem(t *testing.T) {
	h := dressed(t, twoLanguageTree())
	h.send(tea.WindowSizeMsg{Width: 95, Height: 12})

	h.wants(landingChoose, "English", "Deutsch").refuses("TEST OS")
}

// And whatever the terminal, the page stays inside it.
func TestTheWelcomePageNeverRunsPastTheEdge(t *testing.T) {
	h := dressed(t, twoLanguageTree())
	for _, size := range [][2]int{{80, 24}, {100, 30}, {200, 60}, {34, 13}, {20, 6}} {
		h.send(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		lines := strings.Split(h.screen(), "\n")
		if len(lines) > size[1] {
			t.Errorf("at %dx%d the page is %d rows tall:\n%s", size[0], size[1], len(lines), h.screen())
		}
		for _, line := range lines {
			if w := lipgloss.Width(line); w > size[0] {
				t.Errorf("at %dx%d a line is %d wide:\n%s", size[0], size[1], w, line)
			}
		}
	}
}

// A language named on the command line is the answer the welcome page would
// have asked for, so the page is not drawn at all and the run opens where it
// would have gone next.
func TestALanguageNamedOnTheCommandLineSkipsTheWelcomePage(t *testing.T) {
	mod := loadModule(t, writeModule(t, t.TempDir(), twoLanguageTree()))
	h := startAs(t, testRuntime(), openModule(t), "", Opening{Settled: true}, mod)
	h.wants(labelPresets(), "Full", "Bare").refuses(landingChoose)
}

// The landing page leads, because every word of every page after it is in the
// language chosen on it.
func TestTheLandingPageIsTheFirstThingDrawn(t *testing.T) {
	h := newHarness(t, twoLanguageTree())
	h.wants(landingChoose, "Deutsch").refuses("Full", "Bare")

	h.down().enter() // Deutsch
	h.wants("Vollständig", "Bare")
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

// ─── Actions ─────────────────────────────────────────────────────────────────

// wireless is a network written the way a module needing the internet writes
// one in actions: a card where card says yes, and an internet check that says
// yes once marker exists and otherwise falls back on joining. The open network
// joins as chosen, the other asks for its passphrase, and both write what they
// joined with to marker.
func wireless(marker, card string, waits bool) map[string]string {
	tree := map[string]string{
		treeFile:                   testInstaller + wirelessRules(waits),
		"actions/card/action.yaml": "title: A wireless card\n",
		"actions/card/action.sh":   card + "\n",
		"actions/wlan/action.yaml": `
title: Wireless network
description: Join a wireless network.
rules:
  offer-if: [card]
  on-failure: [wlan-passphrase]
variables:
  - name: WLAN_SSID
    type: list
    title: Network
    options: [HomeNet, CafeNet]
`,
		"actions/wlan/action.sh": "[ \"$WLAN_SSID\" = CafeNet ] || exit 1\nprintf '%s' \"$WLAN_SSID\" > " + marker + "\n",
		"actions/wlan-passphrase/action.yaml": `
title: Wireless network
error: "{{WLAN_SSID}} did not accept that passphrase."
variables:
  - name: WLAN_PASSPHRASE
    title: Passphrase
    type: password
`,
		"actions/wlan-passphrase/action.sh": `printf '%s %s' "$WLAN_SSID" "$WLAN_PASSPHRASE" > ` + marker + "\n",
	}
	if waits {
		tree["actions/internet/action.yaml"] = "title: Internet\nerror: There is no internet connection. Plug in a cable, or join a wireless network.\nrules:\n  on-failure: [wlan]\n"
		tree["actions/internet/action.sh"] = "test -e " + marker + "\n"
	}
	return tree
}

// wirelessRules is where that module names its actions, for a test that writes
// a declaration of its own.
func wirelessRules(waits bool) string {
	if waits {
		return "rules:\n  start-if: [internet]\n  on-settings: [wlan]\n"
	}
	return "rules:\n  on-settings: [wlan]\n"
}

// A requirement that says yes is never seen: it is a wait for something missing, not
// a page every run clicks through.
func TestNothingIsWaitedForWhereTheRequirementSaysYes(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "online")
	if err := os.WriteFile(marker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, wireless(marker, "exit 0", true))
	h.wants("Full", "Bare").refuses("internet connection")
}

// While it says no, what it said is the page, and nothing behind it is reached:
// the work would only stop at its first download.
func TestTheWorkWaitsWhileARequirementSaysNo(t *testing.T) {
	h := newHarness(t, wireless(filepath.Join(t.TempDir(), "online"), "exit 0", true))
	h.esc()
	h.wants("Internet", "There is no internet connection.", "Join a wireless network.", "open").
		refuses("Full", "Bare")
}

// What it falls back on is opened straight away where that asks something
// first: the question is the next thing to do, and it stands on top of the wait.
func TestAFallbackThatAsksIsOpenedStraightAway(t *testing.T) {
	h := newHarness(t, wireless(filepath.Join(t.TempDir(), "online"), "exit 0", true))
	h.wants("Wireless network", "Network", "HomeNet", "CafeNet").refuses("Full", "Bare")
	if _, ok := h.m.stack[len(h.m.stack)-2].(*gateScreen); !ok {
		t.Errorf("the page under the network is %T, want the wait", h.m.stack[len(h.m.stack)-2])
	}
}

// One that would run the moment it is opened waits for enter: choosing it is
// the consent, and nobody has chosen anything yet.
func TestAFallbackThatRunsAtOnceWaitsForEnter(t *testing.T) {
	ran := filepath.Join(t.TempDir(), "ran")
	h := newHarness(t, map[string]string{
		treeFile:                       testInstaller + "rules:\n  start-if: [internet]\n",
		"actions/internet/action.yaml": "title: Internet\nerror: There is no internet connection.\nrules:\n  on-failure: [dhcp]\n",
		"actions/internet/action.sh":   "exit 1\n",
		"actions/dhcp/action.yaml":     "title: Ask for an address\n",
		"actions/dhcp/action.sh":       "touch " + ran + "\n",
	})
	h.wants("There is no internet connection.", "open")
	if _, err := os.Stat(ran); err == nil {
		t.Error("the fallback ran before anybody chose it")
	}
}

// It looks again by itself, so a cable plugged in carries on without a key.
func TestTheWaitCarriesOnByItself(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "online")
	h := newHarness(t, wireless(marker, "exit 1", true))
	h.wants("There is no internet connection.")

	if err := os.WriteFile(marker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	h.send(gateDueMsg{round: h.m.top().(*gateScreen).round})
	h.wants("Full", "Bare")
}

// A clock set before the page moved on is not answered: only the look the page
// is waiting for counts, so two never run side by side.
func TestTheWaitAnswersOnlyItsOwnClock(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "online")
	h := newHarness(t, wireless(marker, "exit 1", true))
	if err := os.WriteFile(marker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	h.send(gateDueMsg{round: h.m.top().(*gateScreen).round - 1})
	h.wants("There is no internet connection.")
}

// The fallback, and where that says no the one it names in turn, a page each,
// after which the wait looks again and the opening goes on as with a cable.
// Each script gets the pages' answers under their names.
func TestTheFallbackOfTheWaitCarriesOn(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "online")
	h := newHarness(t, wireless(marker, "exit 0", true))
	h.wants("Wireless network", "Network", "HomeNet", "CafeNet")

	h.enter() // HomeNet, which wants a passphrase
	h.wants("Passphrase")
	h.typeIn("secret").enter()
	h.wants("Full", "Bare")
	if got, _ := os.ReadFile(marker); string(got) != "HomeNet secret" {
		t.Errorf("the action was handed %q, want the network and its passphrase", got)
	}
	if got := h.a.store.Get("WLAN_PASSPHRASE"); got != "" {
		t.Errorf("the passphrase is still held after the action ran: %q", got)
	}
}

// An action that says yes opens no fallback: the open network is joined the
// moment it is chosen, and nothing asks for a passphrase it does not have.
func TestAnActionThatWorksOpensNoFallback(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "online")
	h := newHarness(t, wireless(marker, "exit 0", true))
	h.down().enter() // CafeNet
	h.wants("Full", "Bare").refuses("Passphrase")
	if got, _ := os.ReadFile(marker); string(got) != "CafeNet" {
		t.Errorf("the action was handed %q, want the open network", got)
	}
}

// Backing out of a fallback's page is a page back in the chain: the action
// before it, to choose again.
func TestBackingOutOfAFallbacksPageGoesBackAlongTheChain(t *testing.T) {
	h := newHarness(t, wireless(filepath.Join(t.TempDir(), "online"), "exit 0", true))
	h.enter()
	h.wants("Passphrase")

	h.esc()
	h.wants("Network", "HomeNet", "CafeNet")
}

// Backing out of the fallback is not backing out of the wait: the page behind it
// still says what is missing, and does not open it again by itself. Enter does.
func TestBackingOutOfTheFallbackStillWaits(t *testing.T) {
	h := newHarness(t, wireless(filepath.Join(t.TempDir(), "online"), "exit 0", true))
	h.wants("HomeNet")

	h.esc()
	h.wants("There is no internet connection.").refuses("HomeNet")

	h.typeIn("r")
	h.wants("There is no internet connection.").refuses("HomeNet")

	h.enter()
	h.wants("HomeNet")
}

// A machine without what the fallback requires is not offered it: the page says what
// is missing and nothing more, and enter opens nothing.
func TestWithoutWhatTheFallbackRequiresTheWaitOffersNothingToOpen(t *testing.T) {
	h := newHarness(t, wireless(filepath.Join(t.TempDir(), "online"), "exit 1", true))
	h.wants("There is no internet connection.", "r retry").refuses("Join a wireless network.", "open")

	h.enter()
	h.wants("There is no internet connection.").refuses("HomeNet")
}

// A script that does not work, with no fallback, says so in the action's own
// words and with where it broke, and the way back is to the last page: the
// next thing to try is another go at the answer.
func TestAFailingActionSaysWhyAndGoesBackAPage(t *testing.T) {
	tree := wireless(filepath.Join(t.TempDir(), "online"), "exit 0", true)
	tree["actions/wlan-passphrase/action.sh"] = "echo wrong key >&2\nexit 1\n"
	h := newHarness(t, tree)
	h.enter().typeIn("wrong").enter()
	h.wants("Wireless network", "HomeNet did not accept that passphrase.", "Action", "action.sh", "wrong key")

	h.enter()
	h.wants("Passphrase")
}

// The actions the work requires are asked in turn, in the order the module
// names them, and the page moves on to the next the moment one says yes.
func TestTheActionsTheWorkRequiresAreAskedInTurn(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "root")
	h := newHarness(t, map[string]string{
		treeFile:                       testInstaller + "rules:\n  start-if: [root, firmware]\n",
		"actions/root/action.yaml":     "title: Root\nerror: Log in as root.\n",
		"actions/root/action.sh":       "test -e " + marker + "\n",
		"actions/firmware/action.yaml": "title: Firmware\nerror: Set the boot mode to UEFI.\n",
		"actions/firmware/action.sh":   "echo bios >&2\nexit 1\n",
	})
	h.wants("Root", "Log in as root.").refuses("UEFI")

	if err := os.WriteFile(marker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	h.typeIn("r")
	h.wants("Firmware", "Set the boot mode to UEFI.").refuses("Log in as root.", "Full")
}

// intoHub answers the opening of the test module.
func intoHub(h *harness) *harness {
	return h.down().enter().typeIn("moritz").enter().enter() // Bare, the user, the disk
}

// toSettings opens the settings from the menu, wherever its cursor is.
func toSettings(h *harness) *harness {
	for h.m.top().(*hub).picker.selected() != keySettings {
		h.down()
	}
	return h.enter()
}

// toAction opens the settings and puts the cursor on the row of an action.
func toAction(h *harness, title string) *harness {
	toSettings(h)
	s := h.m.top().(*settingsScreen)
	for i := 0; s.picker.items[s.picker.cursor].title != title; i++ {
		if i > len(s.picker.items) {
			h.t.Fatalf("the settings have no row %q:\n%s", title, h.screen())
		}
		h.down()
	}
	return h
}

// An action the module names under `settings` is a row there rather than on the
// menu: first, in its own words and under no heading, ahead of the answers.
func TestAnActionStandsOnTheSettingsPageAheadOfTheAnswers(t *testing.T) {
	h := intoHub(newHarness(t, wireless(filepath.Join(t.TempDir(), "online"), "exit 0", false)))
	h.wants("Start", "Setup").refuses("Wireless network")

	toSettings(h)
	view := h.screen()
	action, answers := strings.Index(view, "Wireless network"), strings.Index(view, "User name")
	if action < 0 || answers < 0 || action > answers {
		t.Errorf("the settings do not read the action, then the answers:\n%s", view)
	}
	if s := h.m.top().(*settingsScreen); s.picker.items[0].heading {
		t.Errorf("the action stands under a heading:\n%s", view)
	}
}

// Under the language, where the interface offers one: both change the session
// rather than an answer, and stand together for that.
func TestAnActionStandsUnderTheLanguage(t *testing.T) {
	tree := wireless(filepath.Join(t.TempDir(), "online"), "exit 0", false)
	tree["locales/de.po"] = "msgid \"English\"\nmsgstr \"Deutsch\"\n"
	h := newHarness(t, tree)
	h.enter() // English
	toSettings(intoHub(h))
	view := h.screen()
	language, action := strings.Index(view, "Interface language"), strings.Index(view, "Wireless network")
	if language < 0 || action < 0 || language > action {
		t.Errorf("the settings do not read the language, then the action:\n%s", view)
	}
}

// A row this machine cannot use is a row nobody needed.
func TestAnActionTheMachineDoesNotHaveIsNoRow(t *testing.T) {
	h := toSettings(intoHub(newHarness(t, wireless(filepath.Join(t.TempDir(), "online"), "exit 1", false))))
	h.wants("User name").refuses("Wireless network")
}

// Opened from the settings, it comes back to them once it has worked, and the
// header says what changed the moment it has rather than an interval later.
func TestAnActionOpenedFromTheSettingsComesBackToThem(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "online")
	tree := wireless(marker, "exit 0", false)
	tree[treeFile] = testInstaller + wirelessRules(false) +
		"status:\n  check: ./status.sh\n  pass: Online\n  fail: Offline\n"
	tree["status.sh"] = "test -e " + marker + "\n"
	h := toAction(intoHub(newHarness(t, tree)), "Wireless network")
	h.wants("Offline")

	h.enter().enter().typeIn("secret").enter()
	h.wants("User name", "Online").refuses("Offline", "Passphrase")
	if _, ok := h.m.top().(*settingsScreen); !ok {
		t.Errorf("the page on top is %T, want the settings", h.m.top())
	}
}

// An action's pages are its own: they are not answers the work reads, so they
// are neither on the settings page nor in the answer file.
func TestAnActionsPagesAreNeitherSettingsNorKept(t *testing.T) {
	h := toAction(intoHub(newHarness(t, wireless(filepath.Join(t.TempDir(), "online"), "exit 0", false))), "Wireless network")
	h.enter().enter().typeIn("secret").enter()
	h.wants("User name").refuses("Network", "Passphrase")

	conf, err := os.ReadFile(h.a.store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(conf), "WLAN_") {
		t.Errorf("an action's page is in the answer file:\n%s", conf)
	}
}

// An action asks its questions in the order it lists them, a page each, and its
// script is handed every answer.
func TestAnActionAsksEachOfItsQuestionsInTurn(t *testing.T) {
	joined := filepath.Join(t.TempDir(), "joined")
	tree := map[string]string{
		treeFile: testInstaller + "rules:\n  on-settings: [join]\n",
		"actions/join/action.yaml": `
title: Join a network
variables:
  - name: SSID
    type: list
    title: Network
    options: [HomeNet, CafeNet]
  - name: KEY
    type: password
    title: Passphrase
`,
		"actions/join/action.sh": `printf '%s %s' "$SSID" "$KEY" > ` + joined + "\n",
	}
	h := toAction(intoHub(newHarness(t, tree)), "Join a network")

	h.enter().wants("Network", "HomeNet", "CafeNet")
	h.down().enter().wants("Passphrase")
	h.typeIn("secret").enter()

	h.wants("User name")
	got, err := os.ReadFile(joined)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "CafeNet secret" {
		t.Errorf("the script was handed %q, want both answers", got)
	}
}

// An action that says no opens the first of what it names on failure that this
// machine has: one it does not have is passed over.
func TestAFailureOpensTheFirstFallbackTheMachineHas(t *testing.T) {
	tree := map[string]string{
		treeFile:                     testInstaller + "rules:\n  on-settings: [try]\n",
		"actions/try/action.yaml":    "title: Try it\nrules:\n  on-failure: [first, second]\n",
		"actions/try/action.sh":      "exit 1\n",
		"actions/first/action.yaml":  "title: First fallback\nrules:\n  offer-if: [never]\nvariables:\n  - name: A\n    type: text\n    title: The first one asks\n",
		"actions/first/action.sh":    "true\n",
		"actions/never/action.yaml":  "title: Never\n",
		"actions/never/action.sh":    "exit 1\n",
		"actions/second/action.yaml": "title: Second fallback\nvariables:\n  - name: B\n    type: text\n    title: The second one asks\n",
		"actions/second/action.sh":   "true\n",
	}
	h := toAction(intoHub(newHarness(t, tree)), "Try it")
	h.enter()
	h.wants("The second one asks").refuses("The first one asks")
}

// An action that looked first and found something asks before it acts on it:
// opening on No, which goes back without running, and Yes runs it.
func TestAnActionAsksBeforeItsWork(t *testing.T) {
	ran := filepath.Join(t.TempDir(), "ran")
	tree := map[string]string{
		treeFile:                      testInstaller + "rules:\n  on-settings: [check]\n",
		"actions/check/action.yaml":   "title: Look for an update\nrules:\n  on-failure: [install]\n",
		"actions/check/action.sh":     "exit 1\n",
		"actions/install/action.yaml": "title: Install the update\nconfirm: Install the update over {{USER}}'s system?\nreport: Installed\n",
		"actions/install/action.sh":   "touch " + ran + "\n",
	}
	h := toAction(intoHub(newHarness(t, tree)), "Look for an update")
	h.enter()
	h.wants("Install the update over moritz's system?", "Yes", "No")
	h.enter()
	h.refuses("Install the update over")
	if _, err := os.Stat(ran); err == nil {
		t.Fatal("No ran the action")
	}

	h.enter()
	h.wants("Install the update over moritz's system?")
	h.up().enter()
	h.wants("Installed")
	if _, err := os.Stat(ran); err != nil {
		t.Fatalf("Yes did not run the action: %v", err)
	}
}

// A list that only suggests offers one more row, under its answers, for an
// answer of one's own - and that row opens a box.
func TestAnOpenListOffersAnAnswerOfOnesOwn(t *testing.T) {
	h := newHarness(t, map[string]string{
		treeFile: strings.Replace(testInstaller, "    type: list\n    title: Disk\n", "    type: open-list\n    title: Disk\n", 1),
	})
	h.down().enter().typeIn("moritz").enter()
	h.wants("Disk", "An answer of your own")
}

// A closed list has no such row.
func TestAListOffersOnlyItsAnswers(t *testing.T) {
	h := newHarness(t, nil)
	h.down().enter().typeIn("moritz").enter()
	h.wants("Disk").refuses("An answer of your own")
}

// ─── The opening ─────────────────────────────────────────────────────────────

// One language on offer means no landing page: the one thing it asks is not a
// question, and a greeting is not reason enough to stop a run on a page nobody
// can answer.
func TestOneLanguageIsNoLandingPage(t *testing.T) {
	newHarness(t, nil).wants("Full", "Bare").refuses(landingChoose, "Language")
}

// A module may tie the words on screen to one of its own answers - see
// `language:` in a module's declaration. Answering it then also settles the
// language, whatever was chosen on the way in.
func TestAModuleCanTieTheInterfaceToOneOfItsOwnAnswers(t *testing.T) {
	h := newHarness(t, regionTree)
	h.enter() // English, the language the interface opens in
	h.wants("Language and region", "de_DE")

	// The answer is a locale rather than the name of a catalog, and it is read
	// as one: de_DE is German.
	h.enter()
	h.wants("Vollständig")
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
	h.wants("Full").refuses("Vollständig")
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
		"  - name: LOCALE\n    type: list\n    title: Language and region\n    required: true\n    first: true\n" +
		"    options: [de_DE, en_US]\n" +
		"language: LOCALE\n",
	"locales/de.po": "msgid \"English\"\nmsgstr \"Deutsch\"\n\nmsgid \"Full\"\nmsgstr \"Vollständig\"\n",
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
	h.wants("Test Installer", "Setup")
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
			"  - name: LOCALE\n    type: list\n    title: Language and formats\n    required: true\n    first: true\n    options: [de, en]\n" +
			"  - name: KEYMAP\n    type: list\n    title: Console keyboard\n    required: true\n    first: true\n    options: [de, us]\n",
	})
	h.wants("Start", "Language and formats")

	h.enter()
	h.wants("Start", "Console keyboard").refuses("Language and formats")

	h.enter()
	h.wants(labelPresets()).refuses("Console keyboard")

	// And the run of questions leaves the opening behind it entirely.
	h.enter()
	h.wants("User name", "1 of 3").refuses(labelPresets())
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

// A secret is not on the page at all. It is never stored, so the row could only
// show what cannot be read and open on nothing that can be typed - and a
// settings page is a promise that every row on it can be opened.
func TestSettingsLeavesASecretOffThePage(t *testing.T) {
	h := newHarness(t, nil)
	h.down().enter().typeIn("moritz").enter().enter()
	h.down().enter()
	h.wants("User name", "Disk").refuses("Password")
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

// Length is not what decides this. A list of thirty waits for the key exactly as
// a list of two does, because how long it turns out to be here is a fact about
// this machine, and a page that changed shape with it would be two pages.
func TestALongListStillWaitsForTheKey(t *testing.T) {
	h := newHarness(t, map[string]string{
		treeFile: testInstaller +
			"  - name: ZONE\n    type: list\n    title: Time zone\n    required: true\n    options-from: ./zones.sh\n",
		"zones.sh": "seq 1 30\n",
	})
	h.down().enter()           // Bare, past the presets
	h.typeIn("moritz").enter() // the user name
	h.enter()                  // the disk

	h.wants("Time zone", "/ filter").refuses("Filter …")

	h.typeIn("/")
	h.typeIn("29")
	h.wants("29").refuses("30")
}

// A list that is already all there needs nothing in front of it, and a box
// would take q and backspace off the page for a list nobody has to look for a
// row in. The key still opens one.
func TestAShortListKeepsItsFilterBehindTheKey(t *testing.T) {
	h := newHarness(t, nil)
	h.down().enter()           // Bare, past the presets
	h.typeIn("moritz").enter() // the user name

	h.wants("Disk", "/ filter").refuses("Filter …")

	h.typeIn("/")
	h.wants("Filter …", "/dev/sda", "/dev/sdb", "esc close")
	h.typeIn("sdb")
	h.wants("/dev/sdb").refuses("/dev/sda")

	// And the box is what esc leaves first, which is the other half of keeping
	// it behind the key: on a list carrying one there is nothing to close.
	h.esc()
	h.wants("Disk", "/dev/sda", "/ filter")
}

// What the list will not say, the question says: a page that has to be scrolled
// through on one machine carries its box on every machine, however few answers
// this one turns out to have.
func TestAQuestionCanCarryItsFilterWhateverTheList(t *testing.T) {
	h := newHarness(t, map[string]string{
		treeFile: testInstaller +
			"  - name: VARIANT\n    type: list\n    title: Keyboard variant\n    required: true\n    filter: open\n    options-from: ./variants.sh\n",
		"variants.sh": "printf 'none\\ndead keys\\n'\n",
	})
	h.down().enter()
	h.typeIn("moritz").enter()
	h.enter()

	h.wants("Keyboard variant", "Filter …", "dead keys")
	h.typeIn("dead")
	h.wants("dead keys").refuses("none")
}

// And the other way about, spelled out: a question may say it keeps its letters,
// which is what every question that says nothing gets. The key still opens one.
func TestAQuestionCanKeepItsFilterBehindTheKeyWhateverTheList(t *testing.T) {
	h := newHarness(t, map[string]string{
		treeFile: testInstaller +
			"  - name: ZONE\n    type: list\n    title: Time zone\n    required: true\n    filter: collapsed\n    options-from: ./zones.sh\n",
		"zones.sh": "seq 1 30\n",
	})
	h.down().enter()
	h.typeIn("moritz").enter()
	h.enter()

	h.wants("Time zone", "/ filter").refuses("Filter …")
	h.typeIn("/")
	h.wants("Filter …", "esc close")
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
	h.wants("Test Installer", "Setup")
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

// Turning a setting on can call for an answer nothing has asked for yet -
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
	h.wants("Driver", "mesa", "nvidia").refuses("Setup")
	h.enter() // mesa, the focused row
	h.wants("Test Installer", "Setup")
}

// ─── Starting over ───────────────────────────────────────────────────────────

// The last row of the settings page drops every answer this module holds and
// opens it where a machine that has answered nothing opens it - the starting
// points among them, since being offered once is the whole of what one is.
func TestResettingForgetsEveryAnswerAndOffersTheStartingPointsAgain(t *testing.T) {
	h := newHarness(t, nil)
	h.down().enter().typeIn("moritz").enter().enter()
	conf := h.a.store.Path()

	// Started again, the way a machine that has answered before starts: the
	// starting points are spent by then, and bringing them back is half of what
	// the reset is for.
	h.restart()
	h.wants("Setup").refuses(labelPresets())

	h.down().enter()           // Settings
	h.typeIn("/reset").enter() // the one row there that is not an answer
	h.wants("Reset all answers", "cannot be undone", "Yes", "No")

	// It opens on No: an enter meant for the row above it must not throw an
	// hour of answers away.
	h.enter()
	h.wants("Reset all answers").refuses("cannot be undone")
	if _, err := os.Stat(conf); err != nil {
		t.Fatalf("the answers were dropped by a no: %v", err)
	}

	h.enter()                // the row again
	h.key(tea.KeyUp).enter() // Yes, the row above No
	h.wants(labelPresets(), "Everything at once.")
	if _, err := os.Stat(conf); !os.IsNotExist(err) {
		t.Errorf("the answer file is still there after a reset: %v", err)
	}
}

// ─── Installing ──────────────────────────────────────────────────────────────

// The last page before the work comes after every password, and is a yes or no
// that opens on No: the enter that confirmed the password does not start
// anything.
func TestTheLastPageBeforeTheWorkOpensOnNo(t *testing.T) {
	h := newHarness(t, nil)
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter() // Start
	h.wants("Password").refuses(reallyStart)
	h.typeIn("x").enter().typeIn("x").enter()
	h.wants(reallyStart, "Yes", "No")
	if s, ok := h.m.top().(*confirmScreen); !ok || s.picker.selected() != keyNo {
		t.Fatalf("the page does not open on No; the page on top is %T", h.m.top())
	}

	h.enter()
	h.wants("Start", "Setup").refuses(reallyStart)
}

// The passwords before the work follow one another rather than one inside the
// other: each stands under the menu, none behind the one before it.
func TestEachPasswordBeforeTheWorkStandsUnderTheMenu(t *testing.T) {
	h := newHarness(t, map[string]string{
		treeFile: testInstaller + "  - name: PW2\n    title: Disk password\n    type: new-password\n    required: true\n",
	})
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter() // Start
	h.wants(labelMenu() + " " + glyphs.crumb + " Password")
	h.typeIn("x").enter().typeIn("x").enter()
	h.wants(labelMenu() + " " + glyphs.crumb + " Disk password").refuses("Password " + glyphs.crumb)
}

// The last page stands for itself: the password pages before it are done with,
// and a trail naming one of them over a question about the whole run is wrong.
func TestTheLastPageHasNoTrail(t *testing.T) {
	h := newHarness(t, nil)
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter() // Start
	h.wants("Password")
	h.typeIn("x").enter().typeIn("x").enter()
	h.wants(reallyStart).refuses("Password")
}

// A module that does not declare confirm starts its work straight after the
// passwords.
func TestAModuleWithoutConfirmStartsTheWorkAfterThePasswords(t *testing.T) {
	h := newHarness(t, map[string]string{
		treeFile: strings.Replace(testInstaller, "confirm: true\n", "", 1),
	})
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter().typeIn("x").enter().typeIn("x").enter()
	h.refuses(reallyStart).ran()
	h.wants("Finished in")
}

// No on the last page is the menu again, not the password that led to it, and
// the password is forgotten there.
func TestNoOnTheLastPageForgetsThePassword(t *testing.T) {
	h := newHarness(t, nil)
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter().typeIn("hunter2").enter().typeIn("hunter2").enter()
	h.wants(reallyStart)

	h.enter()
	h.wants("Start", "Setup").refuses("Password")
	if got := h.a.store.Get("PW"); got != "" {
		t.Errorf("PW = %q after No", got)
	}
}

// It carries no heading of its own: the frame names the module, and a line
// under it saying the same again would say nothing.
func TestTheLastPageBeforeTheWorkRepeatsNoName(t *testing.T) {
	h := newHarness(t, nil)
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter().typeIn("x").enter().typeIn("x").enter() // Start, and the password
	if got := strings.Count(h.screen(), "Test Installer"); got != 1 {
		t.Errorf("the module is named %d times:\n%s", got, h.screen())
	}
}

func TestTheSecretIsAskedForTwiceAndOnlyThenTheRunBegins(t *testing.T) {
	h := newHarness(t, nil)
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter() // Install, which asks for the password first

	h.wants("Password")
	h.typeIn("hunter2").enter()
	h.wants("Repeat")
	h.typeIn("different").enter()
	h.wants("The entries do not match.", "Password")

	h.typeIn("hunter2").enter().typeIn("hunter2").enter().yes()
	h.ran()
	h.wants("Finished in", "First", "Second").refuses("Only with extras")
}

// A run hands its tasks the answers it runs with, written out whole: a task that
// copies or shares the file passes on exactly those, not a line appended by hand
// twice or a key an older release asked and this one does not.
func TestARunStartsFromItsAnswersWrittenOutWhole(t *testing.T) {
	copied := filepath.Join(t.TempDir(), "copied.conf")
	h := newHarness(t, map[string]string{"tasks/@go/a-first/task.sh": "cp \"$MODULE_CONF\" " + copied + "\n"})
	h.down().enter().typeIn("moritz").enter().enter()
	f, err := os.OpenFile(h.a.store.Path(), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("USER='moritz'\nGONE='asked by an older release'\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := h.a.store.Load(); err != nil {
		t.Fatal(err)
	}

	h.enter().typeIn("hunter2").enter().typeIn("hunter2").enter().yes()
	h.ran()

	raw, err := os.ReadFile(copied)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "GONE") || strings.Count(string(raw), "USER=") != 1 {
		t.Fatalf("the run was handed the file as it was left, not its answers:\n%s", raw)
	}
}

// A password the machine already has is asked once. The repeat is there to
// catch a typo nothing else would - and here the disk catches it seconds later
// and says which entry was wrong, which is more than a second box can.
func TestASecretThatAlreadyExistsIsAskedOnce(t *testing.T) {
	h := newHarness(t, map[string]string{treeFile: strings.Replace(testInstaller,
		"    title: Password\n    type: new-password\n",
		"    title: Password\n    type: password\n", 1)})
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter() // Install, which asks for the password first

	h.wants("Password")
	h.typeIn("hunter2").enter()
	h.refuses("Repeat")

	h.yes().ran()
	h.wants("Finished in", "First", "Second")
}

// Where the module can tell a wrong one, it says so on the page it was typed on,
// in its own words, and asks again - rather than starting a run that stops on
// the first step that needed it.
func TestASecretTheModuleChecksIsRefusedWhereItWasTyped(t *testing.T) {
	h := newHarness(t, map[string]string{
		treeFile: strings.Replace(testInstaller,
			"    title: Password\n    type: new-password\n",
			"    title: Password\n    type: password\n"+
				"    check: ./check.sh\n    error: That is not the password.\n", 1),
		"check.sh": "[ \"$PW\" = hunter2 ]\n",
	})
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter() // Install, which asks for the password first

	h.typeIn("hunter3").enter()
	h.wants("Password", "That is not the password.").refuses("Finished in")
	if got := h.a.store.Get("PW"); got != "" {
		t.Errorf("PW = %q, want a refused password never taken", got)
	}

	h.typeIn("hunter2").enter().yes()
	h.ran()
	h.wants("Finished in")
}

// A task that fails stops the run there, on the same page a finished run stops
// on and under the other mark. Everything about the failure is one keystroke
// behind it, laid out the way every other failure in this program is.
func TestAFailedTaskStopsTheRunAndSaysWhereItBroke(t *testing.T) {
	h := newHarness(t, map[string]string{
		"tasks/@go/b-second/task.sh": "echo starting\nls /definitely/not/here\necho never\n",
	})
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter().typeIn("x").enter().typeIn("x").enter().yes()
	h.ran()
	h.wants("Failed", "It stopped at Second").refuses("Script", "Exit code")

	h.enter().wants("Second", "Module", "Task", "Script", "Command", "Exit code", "not/here")

	// And from there back to the answers, which is where a wrong one is fixed,
	// once that is said: the page is the only account of the failure.
	h.enter().wants("Back to the menu", "Yes", "No")
	h.up().enter().wants("Setup")
}

// A task may ask before it runs, which is how a module offers something
// rather than does it. Declining skips that one and the run carries on.
func TestAnTaskThatAsksIsOfferedRatherThanRun(t *testing.T) {
	h := newHarness(t, map[string]string{
		"tasks/@finish/d-reboot/task.yaml": "title: Reboot\nconfirm: Restart {{DISK}} now?\n",
		"tasks/@finish/d-reboot/task.sh":   "echo never\n",
	})
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter().typeIn("x").enter().typeIn("x").enter().yes()

	h.asked()
	h.wants("Reboot", "Restart /dev/sda now?", "Yes", "No")

	// No: the row keeps its place in the list, marked as passed over.
	h.down().enter()
	h.ran()
	h.wants("Finished in", "First", "Second", "Reboot")
}

// A task that asks first opens on No as well: the enter that answered the page
// before it does not walk into it.
func TestATaskThatAsksOpensOnNo(t *testing.T) {
	h := newHarness(t, map[string]string{
		"tasks/@finish/d-reboot/task.yaml": "title: Reboot\nconfirm: Restart now?\n",
		"tasks/@finish/d-reboot/task.sh":   "echo never\n",
	})
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter().typeIn("x").enter().typeIn("x").enter().yes()

	h.asked()
	h.wants("Restart now?")
	if r, ok := h.m.top().(*runScreen); !ok || r.asking.selected() != keyNo {
		t.Fatalf("the question does not open on No; the page on top is %T", h.m.top())
	}
}

// A task that follows from one before it opens on Yes once that one has run,
// and on No where it was passed over.
func TestATaskOpensOnYesOnceTheTaskItFollowsHasRun(t *testing.T) {
	files := map[string]string{
		"tasks/@finish/d-roll/task.yaml":    "title: Roll back\nconfirm: Roll back now?\n",
		"tasks/@finish/d-roll/task.sh":      "echo rolled\n",
		"tasks/@finish/e-rebuild/task.yaml": "title: Rebuild\nneeds: [d-roll]\nconfirm: Rebuild now?\nyes-after: [d-roll]\n",
		"tasks/@finish/e-rebuild/task.sh":   "echo rebuilt\n",
	}
	for _, rolled := range []bool{true, false} {
		h := newHarness(t, files)
		h.down().enter().typeIn("moritz").enter().enter()
		h.enter().typeIn("x").enter().typeIn("x").enter().yes()
		h.asked().wants("Roll back now?")
		if rolled {
			h.yes()
		} else {
			h.enter()
		}

		h.asked().wants("Rebuild now?")
		want := keyNo
		if rolled {
			want = keyYes
		}
		if r, ok := h.m.top().(*runScreen); !ok || r.asking.selected() != want {
			t.Errorf("rolled back %v: the question does not open on %q", rolled, want)
		}
	}
}

// A value that could not have been known before the work started: the run
// stops where the list of tasks was, asks, and carries on with the answer - and
// the offer after it can name what was just chosen.
func TestATaskCanAskForAValueInTheMiddleOfTheRun(t *testing.T) {
	h := newHarness(t, map[string]string{
		treeFile: testInstaller + `
  - name: SNAPSHOT
    title: Snapshot
    description: Which one to go back to.
    type: deferred
    required: true
    options-from: ./snapshots.sh
`,
		"snapshots.sh":                   "printf 'one\\ntwo\\n'\n",
		"tasks/@finish/d-roll/task.yaml": "title: Roll back\nasks: SNAPSHOT\nconfirm: Replace @ with {{SNAPSHOT}}?\n",
		"tasks/@finish/d-roll/task.sh":   "echo rolled\n",
	})
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter().typeIn("x").enter().typeIn("x").enter().yes()

	h.askedFor()
	h.wants("Roll back", "Which one to go back to.", "one", "two")

	// The answer is stored, and the offer that follows reads it back.
	h.down().enter()
	h.asked()
	h.wants("Replace @ with two?")
	if got := h.a.store.Get("SNAPSHOT"); got != "two" {
		t.Errorf("SNAPSHOT = %q, want two", got)
	}

	h.yes()
	h.ran()
	h.wants("Finished in", "Roll back")
}

// A choice that holds for one run, such as the device about to be written, is
// asked every time the work is started, before any password, and nowhere else:
// not on the settings page and not in the answer file.
func TestTheModuleAsksEachTimeTheWorkStarts(t *testing.T) {
	h := newHarness(t, map[string]string{
		treeFile: strings.Replace(testInstaller, "confirm: true\n", "confirm: true\nasks: [DEVICE]\n", 1) + `
  - name: DEVICE
    title: Device
    description: The one to write.
    type: deferred
    required: true
    options-from: disks()
`,
	})
	h.down().enter().typeIn("moritz").enter().enter()
	h.down().enter().refuses("Device").esc()

	h.up().enter()
	h.wants("Device", "The one to write.", "/dev/sda", "/dev/sdb").refuses("Password", reallyStart)
	h.down().enter()
	h.wants("Password")
	if got := h.a.store.Get("DEVICE"); got != "/dev/sdb" {
		t.Errorf("DEVICE = %q, want /dev/sdb", got)
	}

	// Back out to the menu and start again: asked again, on the answer just given.
	h.esc().esc()
	h.enter()
	h.wants("Device").enter()
	h.typeIn("x").enter().typeIn("x").enter().yes()
	h.ran()
	raw, err := os.ReadFile(h.a.store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "DEVICE") {
		t.Errorf("the answer file keeps the device:\n%s", raw)
	}
}

// With nothing to choose from, the question says so and goes nowhere but back.
func TestAQuestionAsTheWorkStartsWithNothingToChooseGoesBack(t *testing.T) {
	h := newHarness(t, map[string]string{
		treeFile: strings.Replace(testInstaller, "confirm: true\n", "confirm: true\nasks: [DEVICE]\n", 1) + `
  - name: DEVICE
    title: Device
    type: deferred
    required: true
    error: Plug a device in.
    options-from: ./none.sh
`,
		"none.sh": "true\n",
	})
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter()
	h.wants("Device", "Plug a device in.")
	h.enter().wants("Device", "Plug a device in.").refuses("Password")
	h.esc().wants("Start", "Setup")
}

// A box that is up from the first frame hands esc on rather than closing, and
// the question it sits over has nothing behind it: esc asks how to leave, as it
// does over every other question in a run, and the hint says so.
func TestEscOverAQuestionWithItsBoxUpAsksHowToLeave(t *testing.T) {
	files := leaveTree("true", "true")
	files[treeFile] = testInstaller + `
  - name: SNAPSHOT
    title: Snapshot
    type: deferred
    required: true
    filter: open
    options-from: ./snapshots.sh
rules:
  on-leave: [restart, shutdown]
`
	files["snapshots.sh"] = "printf 'one\\ntwo\\n'\n"
	files["tasks/@finish/d-roll/task.yaml"] = "title: Roll back\nasks: SNAPSHOT\n"
	files["tasks/@finish/d-roll/task.sh"] = "echo rolled\n"
	h := newHarness(t, files)
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter().typeIn("x").enter().typeIn("x").enter().yes()

	h.askedFor()
	h.wants("Filter …", "one", "two", "esc quit").refuses("esc back")

	h.esc()
	h.wants("Restart", "Shut down")

	h.esc()
	h.wants("Filter …", "one", "two").refuses("Restart")
}

// A question the run stopped for that turns out to have no answers is a task
// with nothing to do: this machine has no snapshot to go back to. The step is
// skipped and the run carries on, because what there is to choose from is read
// off work that has only just happened and no module could have declared it.
func TestAskingForSomethingThatIsNotThereSkipsTheTask(t *testing.T) {
	h := newHarness(t, map[string]string{
		treeFile: testInstaller + `
  - name: SNAPSHOT
    title: Snapshot
    type: deferred
    options-from: ./snapshots.sh
`,
		"snapshots.sh":                   "true\n",
		"tasks/@finish/d-roll/task.yaml": "title: Roll back\nasks: SNAPSHOT\n",
		"tasks/@finish/d-roll/task.sh":   "exit 1\n",
	})
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter().typeIn("x").enter().typeIn("x").enter().yes()
	h.ran()
	h.wants("Finished in", "Roll back")
}

// A command that failed is the other outcome and stops the run: nothing was
// read, so nothing is known, and skipping on that would leave a step out
// because a script had a typo in it.
func TestAskingWithACommandThatFailsEndsTheRun(t *testing.T) {
	h := newHarness(t, map[string]string{
		treeFile: testInstaller + `
  - name: SNAPSHOT
    title: Snapshot
    type: deferred
    options-from: ./snapshots.sh
`,
		"snapshots.sh":                   "exit 3\n",
		"tasks/@finish/d-roll/task.yaml": "title: Roll back\nasks: SNAPSHOT\n",
		"tasks/@finish/d-roll/task.sh":   "echo never\n",
	})
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter().typeIn("x").enter().typeIn("x").enter().yes()
	h.ran()
	h.wants("Failed", "Roll back")
}

// Nothing typed while a run is going may dismiss its result, and nothing said
// in confidence survives it.
func TestASecretIsForgottenWhenTheRunIsOver(t *testing.T) {
	h := newHarness(t, nil)
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter().typeIn("hunter2").enter().typeIn("hunter2").enter().yes()
	h.ran()
	if got := h.a.store.Get("PW"); got != "" {
		t.Errorf("PW = %q after the run", got)
	}
}

// ─── The system check ────────────────────────────────────────────────────────

// A check of the machine is an action the work requires with nothing to fall
// back on: what it said is the page, in the words it was written in, and
// nothing gets past it.
func TestAFailedSystemCheckIsWaitedOn(t *testing.T) {
	h := newHarness(t, map[string]string{
		treeFile:                       testInstaller + "rules:\n  start-if: [firmware]\n",
		"actions/firmware/action.yaml": "title: Check\nerror: Set the boot mode to UEFI.\n",
		"actions/firmware/action.sh":   "echo bios >&2\nexit 1\n",
	})
	h.wants("Check", "Set the boot mode to UEFI.", "r retry").refuses("Full", "open", "bios")

	h.enter()
	h.wants("Set the boot mode to UEFI.").refuses("Full")
}

func TestASystemCheckThatPassesLeadsStraightOn(t *testing.T) {
	h := newHarness(t, map[string]string{
		treeFile:                       testInstaller + "rules:\n  start-if: [firmware]\n",
		"actions/firmware/action.yaml": "title: Check\nerror: Set the boot mode to UEFI.\n",
		"actions/firmware/action.sh":   "echo fine\n",
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

// The page the work waits on says whatever the action wrote, and a module is
// free to write a lot: at every size it stays inside the terminal.
func TestTheWaitNeverRunsPastTheEdge(t *testing.T) {
	long := strings.Repeat("This machine is not ready yet, and here is a long account of why. ", 6)
	h := newHarness(t, map[string]string{
		treeFile:                       testInstaller + "rules:\n  start-if: [internet]\n",
		"actions/internet/action.yaml": "title: Internet\nrules:\n  on-failure: [wlan]\nerror: " + long + "\n",
		"actions/internet/action.sh":   "exit 1\n",
		"actions/wlan/action.yaml":     "title: Wireless network\ndescription: Join a wireless network.\n",
		"actions/wlan/action.sh":       "true\n",
	})
	h.wants("long account of why")
	for _, size := range [][2]int{{80, 24}, {100, 30}, {34, 13}, {20, 6}} {
		h.send(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		lines := strings.Split(h.screen(), "\n")
		if len(lines) > size[1] {
			t.Errorf("at %dx%d the page is %d rows tall:\n%s", size[0], size[1], len(lines), h.screen())
		}
		for _, line := range lines {
			if w := lipgloss.Width(line); w > size[0] {
				t.Errorf("at %dx%d a line is %d wide:\n%s", size[0], size[1], w, line)
			}
		}
	}
}

// failedRun is a module whose first task fails, and which names an action for a
// run that failed: sharing its log, with the address it put online drawn as a
// code.
func failedRun(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t, map[string]string{
		treeFile: testInstaller + "rules:\n  on-failure: [share-log]\n",
		"actions/share-log/action.yaml": "title: Share the log\ndescription: Put the log online.\n" +
			"shows: LOG_URL\nreport: The log is online\n",
		"actions/share-log/action.sh": "printf \"LOG_URL='https://paste.example/abc'\\n\" >>\"$MODULE_CONF\"\n",
		"tasks/@go/a-first/task.sh":   "ls /definitely/not/here\n",
	})
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter().typeIn("x").enter().typeIn("x").enter().yes()
	return h.ran().enter()
}

// A run that failed stops on the report of where, and under it the actions the
// module names for that: a way to hand the log to somebody who can read it.
func TestAFailedRunOffersTheModulesActionsForIt(t *testing.T) {
	h := failedRun(t)
	h.wants("Exit code", "Share the log", "Continue")

	h.key(tea.KeyUp).enter()
	h.wants("The log is online", "https://paste.example/abc")

	h.enter()
	h.wants("Exit code", "Share the log", "Continue")
}

// The rows open on the one that leaves: an action is chosen on purpose, and an
// enter meant for the page before runs nothing. Leaving asks first, opening on
// No, since the menu offers nothing about the failure.
func TestTheRowsUnderAFailedRunOpenOnContinue(t *testing.T) {
	h := failedRun(t)
	h.enter()
	h.wants("Back to the menu", "Yes", "No").refuses("The log is online")
	h.enter()
	h.wants("Exit code", "Share the log", "Continue")
	h.enter().up().enter()
	h.wants("Setup").refuses("The log is online")
	if got := h.a.store.Get("LOG_URL"); got != "" {
		t.Errorf("LOG_URL = %q, want nothing: the action was never chosen", got)
	}
}

// However much the tool said, the rows stay on the page: what it said gives up
// its front, where its last lines say why it stopped.
func TestTheRowsUnderAFailedRunOutlastALongFailure(t *testing.T) {
	h := newHarness(t, map[string]string{
		treeFile:                        testInstaller + "rules:\n  on-failure: [share-log]\n",
		"actions/share-log/action.yaml": "title: Share the log\n",
		"actions/share-log/action.sh":   "true\n",
		"tasks/@go/a-first/task.sh":     "for i in $(seq 60); do echo \"line $i\" >&2; done\nexit 1\n",
	})
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter().typeIn("x").enter().typeIn("x").enter().yes()
	h.ran().enter()
	h.send(tea.WindowSizeMsg{Width: 80, Height: 24})
	h.wants("Exit code", "line 60", "Share the log", "Continue").refuses("line 1\n", "line 2 ")
}

// finishedRun is a module whose run works, and which names an action for a run
// that finished: sharing what it was installed with, drawn as a code.
func finishedRun(t *testing.T) *harness {
	t.Helper()
	return installed(t, map[string]string{
		treeFile: testInstaller + "rules:\n  on-success: [share]\n",
		"actions/share/action.yaml": "title: Share the answers\ndescription: Put them online.\n" +
			"shows: LINK\nreport: The answers are online\n",
		"actions/share/action.sh": "printf \"LINK='https://paste.example/def'\\n\" >>\"$MODULE_CONF\"\n",
	})
}

// A run that finished ends on its result, and under it the actions the module
// offers once the work is done - opening on the row that goes on.
func TestAFinishedRunOffersTheModulesActionsForIt(t *testing.T) {
	h := finishedRun(t)
	h.wants("Finished in", "Share the answers", "Continue").refuses("First", "Second")
	if r, ok := h.m.top().(*runScreen); !ok || r.after.selected() != keyGoOn {
		t.Fatalf("the rows do not open on Continue; the page on top is %T", h.m.top())
	}

	h.key(tea.KeyUp).enter()
	h.wants("The answers are online", "https://paste.example/def")

	h.enter()
	h.wants("Finished in", "Share the answers", "Continue")
	h.down().enter()
	if !h.m.quitting {
		t.Error("Continue on a finished run did not go where the run leads")
	}
}

// A failure report is the one page that has to be readable on the narrowest
// terminal there is: it is what somebody photographs and sends to a forum.
func TestAFailureReportFitsTheSmallestTerminal(t *testing.T) {
	h := newHarness(t, map[string]string{
		"tasks/@go/a-first/task.sh": "echo starting\nls /definitely/not/here\necho never\n",
	})
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter().typeIn("x").enter().typeIn("x").enter().yes()
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
// do. Everything else the runtime offers - a language to pick, a starting
// point, a task that asks first - is a page that simply does not appear.
func TestTheSmallestTreeStillWorks(t *testing.T) {
	h := newHarness(t, map[string]string{
		treeFile:                       "title: Test Installer\nstages: [go]\nvariables:\n  - name: USER\n    type: text\n    title: User name\n    required: true\n",
		"tasks/@go/a-first/task.yaml":  "title: Do it\n",
		"tasks/@go/b-second/task.yaml": "",
		"tasks/@go/b-second/task.sh":   "",
		"tasks/@go/c-extras/task.yaml": "",
		"tasks/@go/c-extras/task.sh":   "",
	})
	// Straight to the one question: no preset page, because there are no presets.
	h.wants("User name", "1 of 1")
	h.typeIn("moritz").enter()
	h.wants("Test Installer", "Setup")

	// Nothing to confirm and no secret to ask for: the row is the work.
	h.enter().ran()
	h.wants("Finished in", "Do it")

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
		treeFile:                       testInstaller + "rules:\n  on-leave: [restart, shutdown]\n",
		"actions/restart/action.yaml":  "title: Restart\ndescription: Close this machine down and start it again.\n",
		"actions/restart/action.sh":    restart + "\n",
		"actions/shutdown/action.yaml": "title: Shut down\ndescription: Switch this machine off.\n",
		"actions/shutdown/action.sh":   shutdown + "\n",
	}
}

func TestQuittingAsksWhatToDoWithTheMachine(t *testing.T) {
	h := newHarness(t, leaveTree("true", "true"))
	h.down().enter().typeIn("moritz").enter().enter()
	h.wants("Test Installer", "Setup")

	h.typeIn("q")
	h.wants("Restart", "Shut down", "Exit")
	if h.m.quitting {
		t.Fatal("q left the program instead of asking")
	}

	// And it is a question like any other: esc is the way back to the hub.
	h.esc()
	h.wants("Test Installer", "Setup")
}

// Under the module's ways out is the runtime's own: the program stops and the
// machine keeps running, which the row says under itself.
func TestLeavingToTheConsoleClosesOnlyTheProgram(t *testing.T) {
	h := newHarness(t, leaveTree("true", "true"))
	h.down().enter().typeIn("moritz").enter().enter()
	h.typeIn("q")
	h.down().down()
	h.wants("Exit", labelConsoleHelp())

	h.enter()
	if !h.m.quitting {
		t.Fatal("choosing the console did not leave the program")
	}
}

// A kiosk is a machine with nothing behind the program: Exit closes it for
// whatever keeps it running to start it again, and the answers stay for that
// start to open on.
func TestAKioskExitsWithEveryAnswerKept(t *testing.T) {
	mod := loadModule(t, writeModule(t, t.TempDir(), leaveTree("true", "true")))

	h := startAs(t, testRuntime(), openModule(t), "", Opening{Kiosk: true}, mod)
	h.down().enter().typeIn("moritz").enter().enter()
	if !h.a.store.Exists() {
		t.Fatal("the answers were never written, so keeping them would prove nothing")
	}
	h.typeIn("q")
	h.wants("Restart", "Shut down", "Exit").refuses("Reset")

	h.down().down()
	h.wants("Every answer is kept").refuses("The machine keeps running")
	h.enter()
	if !h.m.quitting {
		t.Fatal("Exit did not close the program")
	}
	if !h.a.store.Exists() {
		t.Error("Exit forgot the answers")
	}
}

// And it is a way out every module has there, so a module that says nothing
// about leaving still asks rather than quitting to a prompt nobody should see.
func TestAKioskAsksEvenWhereTheModuleSaysNothingAboutLeaving(t *testing.T) {
	mod := loadModule(t, writeModule(t, t.TempDir(), nil))
	h := startAs(t, testRuntime(), openModule(t), "", Opening{Kiosk: true}, mod)
	h.down().enter().typeIn("moritz").enter().enter()

	h.typeIn("q")
	if h.m.quitting {
		t.Fatal("q quit a kiosk instead of asking")
	}
	h.wants("Exit").refuses("Restart")
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
	h.wants("Failed", "Restart", "Shut down")
}

// Nothing follows a finished installation but the machine being put down.
func TestAFinishedInstallationEndsOnTheWayOut(t *testing.T) {
	h := newHarness(t, leaveTree("true", "true"))
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter().typeIn("x").enter().typeIn("x").enter().yes()
	h.ran()
	h.wants("Finished in")

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
	h.enter().typeIn("x").enter().typeIn("x").enter().yes()
	// The footer names the one key that works while it runs.
	h.wants("Working · 00:0", "esc quit").refuses("working …")

	h.esc()
	h.wants("Restart", "Shut down", "continues behind this page")
	if !working(h.m.top()) {
		t.Error("the run was stopped by somebody asking how to leave it")
	}

	h.esc()
	h.wants("Working · 00:0").refuses("Restart")

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
	h.enter().typeIn("x").enter().typeIn("x").enter().yes()
	h.wants("Working · 00:0")

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
	h.enter().typeIn("x").enter().typeIn("x").enter().yes()
	h.ran()
	h.wants("Finished in 00:0")
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

// In front of a box being typed into, backspace only deletes, so holding it
// down to clear the box never steps back a page on the next repeat. Esc is the
// way back there, the only one the hint promises.
func TestBackspaceOnlyDeletesInFrontOfABox(t *testing.T) {
	h := newHarness(t, nil)
	h.down().enter()
	h.typeIn("moritz").erase()
	h.wants("morit").refuses("moritz")

	// Held down well past the end of the text, which is what a repeat does.
	for range 10 {
		h.erase()
	}
	h.wants("User name").refuses(labelPresets(), "Full", "Bare")

	h.esc()
	h.wants(labelPresets(), "Full", "Bare")
}

// The same in a narrowing box: clearing a query cannot close the box and then
// walk off the page behind it.
func TestBackspaceInANarrowingBoxOnlyDeletes(t *testing.T) {
	h := newHarness(t, nil)
	h.down().enter().typeIn("moritz").enter().enter()
	h.down().enter() // Settings
	h.typeIn("/disk")
	h.wants("Disk").refuses("User name")

	for range 10 {
		h.erase()
	}
	h.wants("User name", "Disk")

	// The box is still open, so the first esc closes it and only the second
	// leaves the page.
	h.esc().wants("User name", "Disk")
}

// And in a password, which is a box from edge to edge.
func TestBackspaceInAPasswordOnlyDeletes(t *testing.T) {
	h := newHarness(t, nil)
	h.down().enter().typeIn("moritz").enter().enter()
	h.key(tea.KeyUp).enter() // the row that installs, which asks for the password first
	h.typeIn("hunter2")

	for range 12 {
		h.erase()
	}
	h.wants("Password").refuses("Ready to start")
}

// The question of which module to open is a page like any other: it was pushed
// onto the language, so esc lands back on it.
func TestTheQuestionOfWhichModuleIsBackedOutOfLikeAnyOther(t *testing.T) {
	mods := both(t)
	dir := forkCatalog(t)
	h := startIn(t, dir, mods...)
	h.enter() // English, and on to the question of which module
	h.wants(forkQuestion, "Test Installer")
	h.esc()
	h.wants(landingChoose, "English")
}

// q asks to leave from wherever it is pressed, not only from the menu - and
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
	h.key(tea.KeyUp).enter() // the install row, which asks for the password first
	h.typeIn("q")
	h.wants("Password").refuses("Restart", "Shut down")
}

// A question a run stopped for has no page behind it - the task waiting on the
// answer has already started - so back means the same thing there that ctrl+c
// means everywhere, and the run is still standing on it afterwards.
func TestAQuestionInARunIsLeftRatherThanBackedOutOf(t *testing.T) {
	files := leaveTree("true", "true")
	files["tasks/@finish/d-reboot/task.yaml"] = "title: Reboot\nconfirm: Restart now?\n"
	files["tasks/@finish/d-reboot/task.sh"] = "echo never\n"
	h := newHarness(t, files)
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter().typeIn("x").enter().typeIn("x").enter().yes()

	h.asked()
	h.wants("Restart now?", "Yes", "No")

	h.erase()
	h.wants("Leave", "Shut down")
	h.esc()
	h.wants("Restart now?", "Yes", "No")
}

// ─── What a run has to report ────────────────────────────────────────────────

// A run of two dozen identical-looking rows cannot say that the work is done.
// So a task may stop the run and say it, once, on a page of its own.
func TestATaskCanReportAMilestone(t *testing.T) {
	h := newHarness(t, map[string]string{
		"tasks/@finish/d-done/task.yaml": "title: Done\nreport: |\n  Installed on {{DISK}}\n\n  Everything after this is offered rather than needed.\n",
		"tasks/@finish/d-done/task.sh":   "true\n",
	})
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter().typeIn("x").enter().typeIn("x").enter().yes()

	h.reported()
	// The words are the module's, filled in from the answers.
	h.wants("Installed on /dev/sda", "Everything after this is offered")
	// A page being read is not a run in progress: no counter, no turning mark.
	h.refuses("of 3")

	h.enter()
	h.ran()
	h.wants("Finished in", "Done")
}

// ─── What a run proved about itself ──────────────────────────────────────────

// installed drives the whole opening and leaves the run finished and
// answerable, which is where every test in this section starts.
func installed(t *testing.T, files map[string]string) *harness {
	t.Helper()
	h := newHarness(t, files)
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter().typeIn("x").enter().typeIn("x").enter().yes()
	return h.ran()
}

// A check runs after the task it belongs to, on the machine that task worked
// on, and what they all came to is one page at the end of the run.
func TestTestsThatPassAreCountedAndNothingMore(t *testing.T) {
	h := installed(t, map[string]string{
		"tasks/@go/a-first/task.yaml":  "title: First\n",
		"tasks/@go/a-first/test.sh":    "true\n",
		"tasks/@go/b-second/task.yaml": "title: Second\n",
		"tasks/@go/b-second/test.sh":   "true\n",
	})
	// One line under the run, and no page: a run that agreed with itself has
	// nothing anybody could open.
	h.wants("Finished in", "2 of 2 tests passed")
	h.enter()
	if !h.m.quitting {
		t.Error("a run that agreed with itself stopped on a page")
	}
}

// A check that disagrees is a thing to look at, not a reason to abandon an
// installation that is already on the disk. It is read behind a row of its
// own under the run, in every module, whatever the module offers there.
func TestAFailedTestDoesNotStopTheRun(t *testing.T) {
	h := installed(t, map[string]string{
		"tasks/@go/a-first/test.sh": "echo the disk is empty >&2\nexit 1\n",
	})
	h.wants("Finished in", "0 of 1 tests passed", "Test results", "Continue")
	h.refuses("failed")

	h.up().enter().wants("Test results", "0 of 1 tests passed", "First")
}

// Opening one is the whole point of the list: what somebody needs from here is
// the file and the line, laid out exactly as a failed run's is.
func TestAFailedTestOpensOnWhereItBroke(t *testing.T) {
	h := installed(t, map[string]string{
		"tasks/@go/a-first/task.yaml":  "title: First\n",
		"tasks/@go/a-first/test.sh":    "true\n",
		"tasks/@go/b-second/task.yaml": "title: Second\n",
		"tasks/@go/b-second/test.sh":   "echo starting\nls /definitely/not/here\n",
	})
	h.up().enter().wants("1 of 2 tests passed", "Second")
	h.enter().wants("Module", "Task", "Second", "Script", "Exit code")

	// Back to the list, back to the run, and on from the row that says so.
	h.enter().wants("Test results", "Second")
	h.esc().wants("Finished in", "Test results", "Continue")
	h.down().enter()
	if !h.m.quitting {
		t.Error("Continue under the run did not go where the run leads")
	}
}

// A test is judged by its exit status, whichever way it leaves - a failing
// command, or a plain `return 1`. A task's own work is the one thing whose
// final status is dropped, and getting the two mixed up made a failing test
// read as a passing one.
func TestATestThatReturnsNonZeroIsAFailure(t *testing.T) {
	h := installed(t, map[string]string{
		"tasks/@go/a-first/task.yaml":  "title: First\n",
		"tasks/@go/a-first/test.sh":    "true\n",
		"tasks/@go/b-second/task.yaml": "title: Second\nreport: Installed\n",
		"tasks/@go/b-second/test.sh":   "return 1\n",
		"tasks/@go/b-second/task.sh":   "true\n",
	})
	h.wants("Installed", "1 of 2 tests passed")
}

// And in the file a task keeps it in, which is where it is actually written.
func TestATestFileThatReturnsNonZeroIsAFailure(t *testing.T) {
	h := installed(t, map[string]string{
		"tasks/@go/a-first/task.yaml":  "title: First\n",
		"tasks/@go/a-first/test.sh":    "true\n",
		"tasks/@go/b-second/task.yaml": "title: Second\nreport: Installed\n",
		"tasks/@go/b-second/task.sh":   "true\n",
		"tasks/@go/b-second/test.sh":   "return 1\n",
	})
	h.wants("Installed", "1 of 2 tests passed")
}

// A report counts what disagreed so far and the run goes straight on: the list
// waits under the end of the run, where it can be read as often as needed.
func TestAReportWithAFailedTestGoesStraightOn(t *testing.T) {
	h := installed(t, map[string]string{
		"tasks/@go/a-first/task.yaml":  "title: First\n",
		"tasks/@go/a-first/test.sh":    "true\n",
		"tasks/@go/b-second/task.yaml": "title: Second\nreport: Installed\n",
		"tasks/@go/b-second/task.sh":   "true\n",
		"tasks/@go/b-second/test.sh":   "echo the disk is empty >&2\nls /definitely/not/here\n",
		"tasks/@go/c-extras/task.yaml": "title: Share this configuration\nconfirm: Put these answers online?\n",
		"tasks/@go/c-extras/task.sh":   "true\n",
	})
	h.wants("Installed", "1 of 2 tests passed")
	h.enter().wants("Put these answers online?").refuses("Test results")

	h.yes().ran().wants("Finished in", "1 of 2 tests passed", "Test results")
	h.up().enter().wants("Test results", "Second")
	h.enter().wants("Module", "Task", "Second", "Script", "test.sh", "Exit code", "the disk is empty")
}

// The list is a page like any other: left with esc, and opened again from the
// same row for as long as the run is on screen.
func TestTheListOfFailedTestsCanBeOpenedAgain(t *testing.T) {
	h := installed(t, map[string]string{
		"tasks/@go/a-first/test.sh": "return 1\n",
	})
	h.up().enter().wants("Test results", "First")
	h.esc().wants("Finished in", "Test results")
	h.erase().wants("Finished in", "Test results")
	h.enter().wants("Test results", "First")
	h.erase().wants("Finished in", "Test results")
}

// And where nothing disagreed there is no row for it: the count says so.
func TestARunWithNoFailedTestHasNoResultsRow(t *testing.T) {
	h := installed(t, map[string]string{
		"tasks/@go/a-first/task.yaml": "title: First\nreport: Installed\n",
		"tasks/@go/a-first/test.sh":   "true\n",
		"tasks/@go/a-first/task.sh":   "true\n",
	})
	h.wants("Installed", "1 of 1 tests passed")
	h.enter().wants("Finished in", "1 of 1 tests passed").refuses("Test results")
}

// ─── What a run went on past ─────────────────────────────────────────────────

// A task the result stands without is gone past when it fails: the row keeps a
// cross, the task after it runs, and the run ends as finished rather than as
// stopped.
func TestAFailedOptionalTaskDoesNotStopTheRun(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "second")
	h := installed(t, map[string]string{
		"tasks/@go/a-first/task.yaml":  "title: First\nallow-failure: true\n",
		"tasks/@go/a-first/task.sh":    "echo the mirror is down >&2\nexit 1\n",
		"tasks/@go/b-second/task.yaml": "title: Second\n",
		"tasks/@go/b-second/task.sh":   "touch '" + marker + "'\n",
	})
	h.wants("Finished in", "1 of 1 optional tasks failed")
	h.refuses("Failed")
	h.up().enter().wants(glyphs.fail+" 1 of 1 optional tasks failed", "First")
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("the task after the failed one did not run: %v", err)
	}
}

// Opening it is the same as opening a failed test: where it broke, and what the
// tool said.
func TestAFailedOptionalTaskOpensOnWhereItBroke(t *testing.T) {
	h := installed(t, map[string]string{
		"tasks/@go/a-first/task.yaml": "title: First\nallow-failure: true\n",
		"tasks/@go/a-first/task.sh":   "echo the mirror is down >&2\nexit 1\n",
	})
	h.up().enter().wants("Test results", "1 of 1 optional tasks failed", "First")
	h.enter().wants("Module", "Task", "First", "Script", "task.sh", "Exit code", "the mirror is down")
	h.enter().wants("Test results")
}

// Work that did not happen has nothing for a test to read or a report to say.
func TestAFailedOptionalTaskSkipsItsTestAndReport(t *testing.T) {
	h := installed(t, map[string]string{
		"tasks/@go/a-first/task.yaml": "title: First\nallow-failure: true\nreport: Themed\n",
		"tasks/@go/a-first/test.sh":   "true\n",
		"tasks/@go/a-first/task.sh":   "exit 1\n",
	})
	h.wants("Finished in", "1 of 1 optional tasks failed").refuses("tests passed", "Themed")
}

// Optional work that came off is not news: nothing is counted and nothing is
// offered.
func TestAnOptionalTaskThatWorksSaysNothing(t *testing.T) {
	h := installed(t, map[string]string{
		"tasks/@go/a-first/task.yaml": "title: First\nallow-failure: true\n",
		"tasks/@go/a-first/task.sh":   "true\n",
	})
	h.wants("Finished in").refuses("optional tasks failed")
	h.enter()
	if !h.m.quitting {
		t.Error("a run whose optional work came off stopped on a page")
	}
}

// Both counts on one line, and both kinds on one page: the work that did not
// happen first, then the checks that disagreed with work that did.
func TestFailedOptionalTasksAndTestsAreReadTogether(t *testing.T) {
	h := installed(t, map[string]string{
		"tasks/@go/a-first/task.yaml":  "title: First\n",
		"tasks/@go/a-first/test.sh":    "false\n",
		"tasks/@go/b-second/task.yaml": "title: Second\nallow-failure: true\n",
		"tasks/@go/b-second/task.sh":   "exit 1\n",
	})
	h.wants("1 of 1 optional tasks failed · 0 of 1 tests passed")
	view := h.up().enter().wants("Test results", "First", "Second").screen()
	if strings.Index(view, "Second") > strings.Index(view, "First") {
		t.Errorf("the failed task is not listed before the failed test:\n%s", view)
	}
}

// The page a task stops the run on is the one somebody reads, so it counts the
// optional work that failed before it.
func TestAReportCountsTheOptionalTasksThatFailedBeforeIt(t *testing.T) {
	h := installed(t, map[string]string{
		"tasks/@go/a-first/task.yaml":  "title: First\nallow-failure: true\n",
		"tasks/@go/a-first/task.sh":    "exit 1\n",
		"tasks/@go/b-second/task.yaml": "title: Second\nreport: Installed\n",
		"tasks/@go/b-second/task.sh":   "true\n",
	})
	h.wants("Installed", "1 of 1 optional tasks failed")
	h.enter().wants("Finished in", "Test results")
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

// A simulated run starts no task and no test: the runtime cannot know what a
// script would change, so everything that has not said it simulates itself is
// only shown as run.
func TestASimulatedRunStartsNoTaskAndNoTest(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "touched")
	h := newSimulated(t, map[string]string{
		"tasks/@go/a-first/task.yaml": "title: First\n",
		"tasks/@go/a-first/test.sh":   "false\n",
		"tasks/@go/a-first/task.sh":   "touch '" + marker + "'\n",
	})
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter().typeIn("x").enter().typeIn("x").enter().yes()

	h.ran().refuses("tests passed")

	if _, err := os.Stat(marker); err == nil {
		t.Error("a simulated run started the task")
	}
}

// A task that declares it simulates itself is run under --debug, test and all,
// and decides with DEBUG what a run that changes nothing does.
func TestATaskThatSimulatesItselfRunsUnderDebug(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "simulated")
	h := newSimulated(t, map[string]string{
		"tasks/@go/a-first/task.yaml": "title: First\nsimulates: true\n",
		"tasks/@go/a-first/test.sh":   "[ \"$DEBUG\" = true ]\n",
		"tasks/@go/a-first/task.sh":   "[ \"$DEBUG\" = true ] && touch '" + marker + "'\n",
	})
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter().typeIn("x").enter().typeIn("x").enter().yes()

	h.ran().wants("1 of 1 tests passed")

	if _, err := os.Stat(marker); err != nil {
		t.Error("the task that simulates itself was not run")
	}
}

// The switch is the runtime's own, and it holds for every module: turned off,
// nothing is run and there is nothing to report.
func TestValidationCanBeSwitchedOff(t *testing.T) {
	files := map[string]string{
		"tasks/@go/a-first/task.yaml": "title: First\n",
		"tasks/@go/a-first/test.sh":   "false\n",
	}
	h := newHarness(t, files)
	h.a.prefs.SetValidates(false)
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter().typeIn("x").enter().typeIn("x").enter().yes()
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
		"tasks/@go/a-first/task.yaml": "title: First\n",
		"tasks/@go/a-first/test.sh":   "true\n",
	})
	h.down().enter().typeIn("moritz").enter().enter()
	h.down().enter().wants("Verify steps", "Yes")

	plain := newHarness(t, nil)
	plain.down().enter().typeIn("moritz").enter().enter()
	plain.down().enter().refuses("Verify steps")
}

// The two rows about the run rather than about a value stand together under
// everything else, with nothing between them.
func TestTheRowsAboutTheRunStandTogether(t *testing.T) {
	h := newHarness(t, map[string]string{
		"tasks/@go/a-first/task.yaml": "title: First\n",
		"tasks/@go/a-first/test.sh":   "true\n",
	})
	h.down().enter().typeIn("moritz").enter().enter()
	h.down().enter()
	for range 20 {
		h.down()
	}
	lines := strings.Split(h.screen(), "\n")
	verify := slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, "Verify steps") })
	if verify < 0 || verify+1 >= len(lines) || !strings.Contains(lines[verify+1], "Reset all answers") {
		t.Errorf("the reset does not stand right under the validation:\n%s", h.screen())
	}
}

// Turning it off from the settings page is what the switch is for, and the row
// says so on the way back.
func TestTheValidationSettingTurnsTestsOff(t *testing.T) {
	h := newHarness(t, map[string]string{
		"tasks/@go/a-first/task.yaml": "title: First\n",
		"tasks/@go/a-first/test.sh":   "true\n",
	})
	h.down().enter().typeIn("moritz").enter().enter()
	h.down().enter()                      // Settings
	h.typeIn("/").typeIn("Verify")        // the row stands last, so it is found rather than walked to
	h.enter().wants("Verify steps", "No") // the page, opened on Yes with No under it
	h.down().enter().wants("Verify steps", "No")
	if h.a.prefs.Validates() {
		t.Error("the switch was answered No and validation is still on")
	}
}

// The two rows that are not answers stand last, under every answer, and the one
// that cannot be taken back stands under the other.
func TestTheRowsThatAreNotAnswersStandLast(t *testing.T) {
	h := newHarness(t, map[string]string{
		"tasks/@go/a-first/task.yaml": "title: First\n",
		"tasks/@go/a-first/test.sh":   "true\n",
	})
	h.down().enter().typeIn("moritz").enter().enter()

	var keys []string
	for _, r := range newSettings(h.a).rows {
		keys = append(keys, r.key)
	}
	want := []string{"USER", "DISK", "EXTRAS", store.ValidateVar, keyReset}
	if !slices.Equal(keys, want) {
		t.Errorf("the settings rows are %v, want %v", keys, want)
	}
}

// ─── A starting point that is fetched rather than written down ───────────────

// The third kind of starting point: not a set of answers in the module but a code
// somebody was handed, and the answers behind it. One row opening one action,
// and from the next page on nothing about it is any different.
func TestAPresetCanFetchItsAnswers(t *testing.T) {
	h := newHarness(t, presetFetches("printf \"USER='moritz'\\nDISK='/dev/sdb'\\nEXTRAS='false'\\n\" >>\"$MODULE_CONF\""))
	h.down().down()
	h.wants("Online")

	// An answer has already been written down once, which is what a real run
	// looks like by the time it reaches this page - so the file carries an empty
	// line for the code, and reading it back would undo the answer about to be
	// given unless that answer is written down first.
	if err := h.a.store.Save(); err != nil {
		t.Fatal(err)
	}

	h.enter()
	h.wants("Configuration code")
	h.typeIn("abc12").enter()

	// Everything the code stood for is an answer now, and with nothing left
	// open the hub is what follows.
	h.wants("Test Installer", "Setup")
	for name, want := range map[string]string{"USER": "moritz", "DISK": "/dev/sdb", "EXTRAS": "false"} {
		if got := h.a.store.Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	if got := h.a.store.Get("SOURCE"); got != "abc12" {
		t.Errorf("SOURCE = %q, want abc12", got)
	}
}

// A code that stands for nothing says so in the action's own words, and the
// way back is to the code: there is nothing to do about it but try another.
func TestAPresetThatCannotFetchSaysWhyAndGoesBack(t *testing.T) {
	h := newHarness(t, presetFetches("exit 1"))
	h.down().down().enter()
	h.typeIn("nope").enter()

	h.wants("Nothing is shared under that code.").refuses("Setup")
	h.enter()
	h.wants("Configuration code").refuses("Setup")
	if _, ok := h.m.top().(*fieldScreen); !ok {
		t.Errorf("the page moved on to %T", h.m.top())
	}
}

// presetFetches is the test module with a third starting point on it: one that
// opens an action asking for a code, whose script makes something of it.
func presetFetches(script string) map[string]string {
	declared := strings.Replace(testInstaller, "\nvariables:", `
  - title: Online
    description: Take the answers from somewhere else.
    action: fetch
variables:`, 1)
	return map[string]string{
		treeFile: declared,
		"actions/fetch/action.yaml": `title: Online
error: Nothing is shared under that code.
variables:
  - name: SOURCE
    type: text
    title: Configuration code
    description: The code of a configuration somebody shared.
    required: true
`,
		"actions/fetch/action.sh": script + "\n",
	}
}

// ─── Failures are not closed by accident ─────────────────────────────────────

// The page a failure is laid out on answers to yes and to nothing else: a run
// that has just gone wrong is exactly when somebody reaches for the key that
// means back.
func TestAFailureIsNotClosedByTheKeyThatMeansBack(t *testing.T) {
	h := installed(t, map[string]string{
		"tasks/@go/a-first/task.yaml":  "title: First\n",
		"tasks/@go/a-first/test.sh":    "true\n",
		"tasks/@go/b-second/task.yaml": "title: Second\n",
		"tasks/@go/b-second/test.sh":   "ls /definitely/not/here\n",
	})
	h.up().enter().wants("Test results", "Second")
	h.enter().wants("Module", "Task", "Second", "Exit code")

	for _, press := range []func() *harness{h.esc, h.erase, h.down} {
		press().wants("Module", "Task", "Second", "Exit code")
	}
	h.enter().wants("Test results", "Second")
}

// progressing starts a run whose second task draws a progress bar and then
// waits at a gate the test opens, so the screen can be read while it runs.
// declared is whether that task says its output is its progress.
func progressing(t *testing.T, declared bool, bar string) (*harness, func()) {
	t.Helper()
	gate := filepath.Join(t.TempDir(), "gate")
	if err := syscall.Mkfifo(gate, 0o600); err != nil {
		t.Fatal(err)
	}
	yaml := "title: Second\n"
	if declared {
		yaml += "progress: true\n"
	}
	h := newHarness(t, map[string]string{
		"tasks/@go/b-second/task.yaml": yaml,
		"tasks/@go/b-second/task.sh":   "printf 'fetching\\n 40%%\\r" + bar + " 75%%'\nread -r _ <" + gate + "\n",
	})
	h.down().enter().typeIn("moritz").enter().enter()
	h.enter().typeIn("x").enter().typeIn("x").enter().yes()

	// Running, and at the gate: the second task has started and drawn its bar.
	deadline := time.Now().Add(20 * time.Second)
	for {
		r, ok := h.m.top().(*runScreen)
		if ok && r.at == 1 && r.session != nil && strings.HasSuffix(r.session.Latest(), "75%") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the second task never drew its bar; the page on top is %T", h.m.top())
		}
		h.drain()
	}
	open := func() {
		if err := os.WriteFile(gate, []byte("\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return h, open
}

// A task that says its output is its progress has the line it drew last under
// its name while it runs - the latest drawing of the bar, not the lines before
// it - and nothing of it is left once it is done.
func TestATaskThatDeclaresItsProgressShowsTheLineItDrewLast(t *testing.T) {
	h, open := progressing(t, true, "")
	h.wants("Second", "75%").refuses("fetching", "40%")

	open()
	h.ran()
	h.wants("Finished in").refuses("75%")
}

// A bar drawn wider than the page is cut at its start: how far it has got is
// what a progress line says at its end - curl's percentage, a copy's rate.
func TestAProgressLineTooWideForThePageKeepsItsEnd(t *testing.T) {
	h, open := progressing(t, true, strings.Repeat("#", 200))
	h.wants("75%")

	open()
	h.ran()
}

// Every other task shows nothing of what it prints, whatever that is.
func TestATaskThatDeclaresNothingShowsNothingOfWhatItPrints(t *testing.T) {
	h, open := progressing(t, false, "")
	h.wants("Second").refuses("75%", "fetching")

	open()
	h.ran()
}

// An answer file carried over from another machine names a disk this one does
// not have. Nothing about the value itself is wrong, so it is the list that
// says so, read once more on the way into the run and before any password: the
// question comes back, with the reason on it and on the list's own suggestion
// rather than on its first row.
func TestAnAnswerTheListNoLongerOffersIsAskedAgainBeforeTheRun(t *testing.T) {
	tree := strings.Replace(testInstaller, "    options-from: disks()",
		"    prefill: ./suggest.sh\n    options-from: disks()", 1)
	h := newHarness(t, map[string]string{treeFile: tree, "suggest.sh": "echo /dev/sdb\n"})
	h.down().enter().typeIn("moritz").enter().enter()
	h.wants("Setup")

	h.a.store.Set("DISK", "/dev/sdz")
	h.enter()
	h.wants("Disk", "This answer is not among the ones offered here.").refuses("Password")
	if f, ok := h.m.top().(*fieldScreen); !ok || f.picker.selected() != "/dev/sdb" {
		t.Fatalf("the question does not open on its suggestion; the page on top is %T", h.m.top())
	}

	h.enter()
	h.wants("Setup")
	h.enter().typeIn("x").enter().typeIn("x").enter().wants(reallyStart)
	if got := h.a.store.Get("DISK"); got != "/dev/sdb" {
		t.Errorf("DISK = %q, want the answer given again", got)
	}
}

// What turned an answer away was its list, so a list that offers it again - the
// stick plugged back in - takes it again, without a restart in between.
func TestAnAnswerTheListOffersAgainIsTakenAgain(t *testing.T) {
	disks := filepath.Join(t.TempDir(), "disks")
	offer := func(lines string) {
		t.Helper()
		if err := os.WriteFile(disks, []byte(lines), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	offer("/dev/sda\n/dev/sdb\n")
	tree := strings.Replace(testInstaller, "    options-from: disks()", "    options-from: ./disks.sh", 1)
	h := newHarness(t, map[string]string{treeFile: tree, "disks.sh": "cat " + disks + "\n"})
	h.down().enter().typeIn("moritz").enter().enter()
	h.wants("Setup")
	h.a.store.Set("DISK", "/dev/sdz")
	h.enter().wants("Disk", "This answer is not among the ones offered here.")

	offer("/dev/sda\n/dev/sdb\n/dev/sdz\n")
	h.esc().wants("Start", "Setup")
	h.down().enter().wants(labelMenu() + " " + glyphs.crumb + " " + rowSetup)
	h.down().enter().wants("/dev/sdz").refuses("This answer is not among the ones offered here.")
	h.enter().esc().wants("Setup")
	h.key(tea.KeyUp).enter().typeIn("x").enter().typeIn("x").enter().wants(reallyStart)
	if got := h.a.store.Get("DISK"); got != "/dev/sdz" {
		t.Errorf("DISK = %q, want the answer taken again", got)
	}
}

// Enter on the row that starts the work while its lists are still being read
// starts nothing twice: the first page opens once they are through.
func TestStartWhileTheListsAreReadOpensTheFirstPageOnceTheyAreThrough(t *testing.T) {
	h := newHarness(t, nil)
	h.down().enter().typeIn("moritz").enter().enter()
	menu, ok := h.m.top().(*hub)
	if !ok {
		t.Fatalf("the page on top is %T, want the menu", h.m.top())
	}

	if _, cmd := menu.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd == nil || !menu.checking {
		t.Fatal("enter on the row did not start reading the lists")
	}
	if _, cmd := menu.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
		t.Error("a second enter while the lists were read started something")
	}
	_, cmd := menu.Update(unofferedMsg{})
	next, ok := cmd().(pushScreenMsg)
	if !ok {
		t.Fatal("nothing followed the check")
	}
	if _, ok := next.s.(*secretScreen); !ok {
		t.Errorf("what followed the check was %T, want the password", next.s)
	}
}
