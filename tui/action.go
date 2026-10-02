package tui

import (
	"strings"
	"time"

	"github.com/murkl/oak/internal/exec"
	"github.com/murkl/oak/internal/logging"
	"github.com/murkl/oak/internal/spec"

	tea "github.com/charmbracelet/bubbletea"
)

// An action is run in one of two ways. By itself, as a question: whether the
// work may begin, whether another action is offered. Or opened by somebody —
// from a row, a starting point, or on the failure of one that said no — which
// is its one page where it has one, and its script. What any of it is for is
// the module's business: this file only walks through what action.yaml
// declares.

// offeredMsg is what the actions an action requires said about this machine.
// The model takes it, whichever page is in front when it lands.
type offeredMsg struct {
	a   *spec.Action
	yes bool
}

// lookFor asks whether this machine has each of the actions that require
// others, beside the drawing rather than in it. One that requires nothing is
// known without asking — see has.
func (a *app) lookFor() tea.Cmd {
	var cmds []tea.Cmd
	for _, act := range a.module.Actions {
		if len(act.Rules.OfferIf) == 0 {
			continue
		}
		ask := a.runner.Offered(act)
		cmds = append(cmds, func() tea.Msg { return offeredMsg{a: act, yes: ask()} })
	}
	return tea.Batch(cmds...)
}

// has reports whether this machine has an action, as what it requires last
// said. Until that has said, the row is not shown: a row that would be taken
// away again is worse than one that lands a moment late.
func (a *app) has(act *spec.Action) bool { return len(act.Rules.OfferIf) == 0 || a.offered[act] }

// fallback is the first action an action names on failure that this machine
// has, or nil.
func (a *app) fallback(act *spec.Action) *spec.Action {
	for _, fb := range a.module.Named(act.Rules.OnFailure) {
		if a.has(fb) {
			return fb
		}
	}
	return nil
}

// rows is the actions a place names that this machine has, in its order.
func (a *app) rows(names []string) []*spec.Action {
	var out []*spec.Action
	for _, act := range a.module.Named(names) {
		if a.has(act) {
			out = append(out, act)
		}
	}
	return out
}

// keyAction is what a row standing for an action is keyed by, in front of the
// action's folder. The NUL prefix cannot collide with anything a module names.
const keyAction = "\x00action:"

// actionRow is the row an action stands as: its title, and what it does under
// it.
func actionRow(act *spec.Action) item {
	return item{title: act.Label(), detail: act.Help(), key: keyAction + act.ID()}
}

// action is the action a row stands for, or nil where the row stands for
// something else.
func (a *app) action(key string) *spec.Action {
	name, ok := strings.CutPrefix(key, keyAction)
	if !ok {
		return nil
	}
	return a.module.Action(name)
}

// asks reports whether an action, opened, puts a question before its work —
// rather than running the moment it is opened.
func (a *app) asks(act *spec.Action) bool { return a.question(act, 0) < len(act.Vars) }

// question is the first of an action's questions from the i-th on that applies
// to the answers, or how many it has where none does.
func (a *app) question(act *spec.Action, i int) int {
	for i < len(act.Vars) && !act.Vars[i].Applies(a.store.Get) {
		i++
	}
	return i
}

// openAction is an action opened by somebody: its page where it asks one, then
// its work.
func (a *app) openAction(act *spec.Action) tea.Cmd { return a.openFrom(act, nil) }

// openFrom is that, with then to carry on with once it has worked, for whoever
// opened it with somewhere to go next.
func (a *app) openFrom(act *spec.Action, then func() tea.Cmd) tea.Cmd {
	return push(a.firstPage(act, 0, then))
}

// firstPage is the page an opened action stands on first, on top of depth
// pages of the actions before it — the one that opened this on failure. Every page
// is pushed onto the one before it, so esc goes back a page, and the work knows
// how many to take away again once it is done.
func (a *app) firstPage(act *spec.Action, depth int, then func() tea.Cmd) screen {
	return a.page(act, 0, depth, then)
}

// page is the action's question from the i-th on, each pushed onto the one
// before, and its work once none is left.
func (a *app) page(act *spec.Action, i, depth int, then func() tea.Cmd) screen {
	i = a.question(act, i)
	if i == len(act.Vars) {
		return &actionScreen{app: a, act: act, depth: depth + 1, then: then}
	}
	v := act.Vars[i]
	after := func() tea.Cmd { return push(a.page(act, i+1, depth+1, then)) }
	if v.Secret() {
		return newSecret(a, v, after)
	}
	return newField(a, v, after).under(act.Label())
}

// actionScreen is an action's script running.
//
// It goes the moment the script has worked, taking the pages before it along,
// back to wherever the action was opened from — or to the page it reports on
// first, where it has one. Where it did not work, what it opens on failure is opened on top
// of those pages, and where it has none, the page every failure opens on, whose
// way back is to the last page: the next thing to try is another go at it.
type actionScreen struct {
	app   *app
	act   *spec.Action
	depth int // the pages that go once it has worked, this one included
	then  func() tea.Cmd

	session *exec.Session
}

// actionRanMsg is the script coming back.
type actionRanMsg struct{ err error }

func (s *actionScreen) Title() string { return "" }

func (s *actionScreen) working() bool { return true }

func (s *actionScreen) Hint() string { return labelHintRunning() }

func (s *actionScreen) Init() tea.Cmd {
	if s.act.TTY && !(s.app.store.Debug() && !s.act.Simulates) {
		// The interface stands down for the length of this one, and the frame
		// is restored exactly as it was when the script exits.
		return tea.Exec(s.app.runner.Terminal(s.act), func(err error) tea.Msg {
			return actionRanMsg{s.app.runner.Fail(s.act, err)}
		})
	}
	session, err := s.app.runner.Open(s.act)
	if err != nil || session == nil {
		return func() tea.Msg { return actionRanMsg{err} }
	}
	s.session = session
	return func() tea.Msg {
		<-session.Done()
		return actionRanMsg{session.Err()}
	}
}

func (s *actionScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	ran, ok := msg.(actionRanMsg)
	if !ok {
		return s, nil
	}
	s.session = nil
	// Whatever the pages were given is gone the moment it is used, whether it
	// worked or not.
	s.app.store.Forget()
	if ran.err != nil {
		logging.Error("%s", ran.err)
		if fb := s.app.fallback(s.act); fb != nil {
			return s, replace(s.app.firstPage(fb, s.depth-1, s.then))
		}
		return s, replace(newFailure(s.act.Label(), ran.err, pop).saying(s.act.Refusal(s.app.store.Get)))
	}
	// What the header keeps an eye on may be what this changed.
	done := func() tea.Cmd {
		var then tea.Cmd
		if s.then != nil {
			then = s.then()
		}
		return tea.Batch(recheck(), back(s.depth, then))
	}
	if !s.act.Reports() {
		return s, done()
	}
	// A report shows what the script wrote down, so the answer file is read
	// back first — the same moment a task's report reads it.
	if err := s.app.runner.Imported(); err != nil {
		logging.Warn("%s: %s", s.act.Title, err)
	}
	headline, body := s.act.ReportText(s.app.store.Get)
	told := newReport(headline, body, s.app.store.Get(s.act.Shows))
	return s, replace(&toldScreen{page: told, done: done})
}

// stop kills the script, where somebody chose to leave while it ran.
func (s *actionScreen) stop() {
	if s.session != nil {
		s.session.Kill()
		s.session = nil
	}
}

func (s *actionScreen) View(width, height int) string {
	return accentStyle.Render(spinFrame()) + field(" ") + boldStyle.Render(truncate(s.act.Label(), max(width-2, 1)))
}

// toldScreen is an action's report: read, and left the way the report of a run
// is, with enter or esc.
type toldScreen struct {
	page *report
	done func() tea.Cmd
}

func (s *toldScreen) Title() string { return "" }
func (s *toldScreen) Hint() string  { return s.page.Hint() }

func (s *toldScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && answers(key) {
		return s, s.done()
	}
	return s, nil
}

func (s *toldScreen) View(width, height int) string { return s.page.View(width, height) }

// gateScreen is the actions the work requires, asked one after another, in the
// order the module names them. It stands on the first that says no, for as long
// as it says no: its fail is the page, it asks again by itself every few
// seconds, and enter opens the action it falls back on where this machine has
// that. Once every one says yes, the opening goes on.
//
// An action to fall back on that asks something first is opened straight away,
// once: its question is the next thing to do, and esc from it shows this page.
// One that would run at once waits for enter, since choosing it is the consent.
type gateScreen struct {
	opening
	app  *app
	all  []*spec.Action
	at   int
	next func() screen

	checked bool
	why     string

	// fallback is what the action standing now opens on failure on this
	// machine, as its last look found it, or nil.
	fallback *spec.Action

	// opened is whether the action standing now has had what it falls back on
	// opened by itself already. Once is help; every time the page comes back
	// would be a page nobody can get to.
	opened bool

	// round is which look still counts. Every new one starts a round, so a clock
	// set before the page moved on is ignored rather than answered twice.
	round int
}

// gateEvery is how often the page asks again by itself: often enough that a
// cable plugged in carries on before anybody reaches for r, rarely enough that
// a check going out to the network is not a load of its own.
const gateEvery = 5 * time.Second

func newGate(a *app, all []*spec.Action, next func() screen) *gateScreen {
	return &gateScreen{app: a, all: all, next: next}
}

type (
	gateMsg struct {
		refused  bool
		fallback *spec.Action
		round    int
	}
	gateDueMsg struct{ round int }
)

func (s *gateScreen) action() *spec.Action { return s.all[s.at] }

func (s *gateScreen) Title() string { return s.action().Label() }

// working is the first question to each action, while it runs. The ones after
// it leave the page standing as it is.
func (s *gateScreen) working() bool { return !s.checked }

func (s *gateScreen) Hint() string {
	switch {
	case !s.checked:
		return labelHintRunning()
	case s.fallback != nil:
		return labelHintOpen()
	}
	return labelHintRetry()
}

// Init asks again from the action standing now: when the page first comes up,
// and whenever it is back on top after what it opens on failure was opened.
func (s *gateScreen) Init() tea.Cmd {
	s.checked = false
	return s.check()
}

// check asks the action standing now, as a new round, whether the work may go
// on — and whether this machine has what it opens on failure.
func (s *gateScreen) check() tea.Cmd {
	s.round++
	round := s.round
	says := s.app.runner.Says(s.action())
	fallbacks := s.app.module.Named(s.action().Rules.OnFailure)
	offers := make([]func() bool, len(fallbacks))
	for i, fb := range fallbacks {
		offers[i] = s.app.runner.Offered(fb)
	}
	return func() tea.Msg {
		if says() {
			return gateMsg{round: round}
		}
		for i, offered := range offers {
			if offered() {
				return gateMsg{refused: true, fallback: fallbacks[i], round: round}
			}
		}
		return gateMsg{refused: true, round: round}
	}
}

// wait sets the clock for the next question, as a new round.
func (s *gateScreen) wait() tea.Cmd {
	s.round++
	round := s.round
	return after(gateEvery, func(time.Time) tea.Msg { return gateDueMsg{round: round} })
}

func (s *gateScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case gateMsg:
		if msg.round != s.round {
			return s, nil
		}
		if !msg.refused {
			if s.at+1 < len(s.all) {
				s.at++
				s.opened = false
				return s, s.Init()
			}
			return s, reset(s.next())
		}
		s.checked, s.why, s.fallback = true, s.action().Refusal(s.app.store.Get), msg.fallback
		if s.fallback != nil && !s.opened && s.app.asks(s.fallback) {
			s.opened = true
			return s, s.app.openAction(s.fallback)
		}
		return s, s.wait()

	case gateDueMsg:
		if msg.round != s.round {
			return s, nil
		}
		return s, s.check()

	case tea.KeyMsg:
		if !s.checked {
			return s, nil
		}
		switch {
		case confirms(msg) && s.fallback != nil:
			return s, s.app.openAction(s.fallback)
		case msg.String() == "r":
			return s, s.check()
		case backs(msg):
			return s, pop()
		}
	}
	return s, nil
}

// View is what the action said, under the mark that says it said no, and the
// line of the action it falls back on under that where enter opens it. What
// does not fit goes from the end: that line first, then the end of the
// sentence, which is marked as cut.
func (s *gateScreen) View(width, height int) string {
	if !s.checked {
		return accentStyle.Render(spinFrame()) + field(" ") + boldStyle.Render(truncate(s.action().Label(), max(width-2, 1)))
	}
	lines := wrap(s.why, max(bodyWidth(width)-2, 1))
	if len(lines) == 0 {
		lines = []string{""}
	}
	var help []string
	if s.fallback != nil {
		help = wrap(s.fallback.Help(), bodyWidth(width))
	}
	if len(lines)+1+len(help) > height {
		help = nil
	}
	if len(lines) > height {
		lines = lines[:max(height, 1)]
		last := len(lines) - 1
		lines[last] = truncate(lines[last], max(width-4, 1)) + " " + glyphs.dash
	}
	var b strings.Builder
	b.WriteString(failStyle.Render(glyphs.fail) + field(" ") + boldStyle.Render(lines[0]))
	for _, line := range lines[1:] {
		b.WriteString("\n" + field("  ") + boldStyle.Render(line))
	}
	if len(help) > 0 {
		b.WriteString("\n\n" + textStyle.Render(strings.Join(help, "\n")))
	}
	return b.String()
}

// back takes n pages off the stack at once, and carries on with then.
func back(n int, then tea.Cmd) tea.Cmd {
	return func() tea.Msg { return popScreenMsg{n: n, then: then} }
}
