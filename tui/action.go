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
// from a row, or as the fallback of a question that said no — which is its
// yes or no where it asks one, its pages one to a screen, its script on a page
// of its own, and its report where it has one. What any of it is for is the
// module's business: this file only walks through what action.yaml declares.

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
		if len(act.Requires) == 0 {
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
func (a *app) has(act *spec.Action) bool { return len(act.Requires) == 0 || a.offered[act] }

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

// openAction is an action opened by somebody: its question where it asks one,
// then its pages, then its work. Every page is pushed onto the one before it,
// so esc goes back a page the way it does through any run of questions, and
// the work page knows how many to take away again once it has worked.
func (a *app) openAction(act *spec.Action) tea.Cmd {
	if !act.Confirms() {
		return a.page(act, 0, 0)
	}
	return push(newOffer(a, act, func() tea.Cmd { return a.page(act, 0, 1) }))
}

// page is what follows the page before it: the next one that applies, given
// the answers so far, or the work. depth is how many of the action's pages are
// on the stack already.
func (a *app) page(act *spec.Action, next, depth int) tea.Cmd {
	for ; next < len(act.Vars); next++ {
		v := act.Vars[next]
		if !v.Applies(a.store.Get) {
			continue
		}
		after := func() tea.Cmd { return a.page(act, next+1, depth+1) }
		if v.Secret() {
			return push(newSecret(a, v, after))
		}
		return push(newField(a, v, after).under(act.Label()))
	}
	return push(&actionScreen{app: a, act: act, depth: depth + 1})
}

// offerScreen is the yes or no an action asks before anything else about it —
// for the one whose running is a decision of its own, like putting something
// online. It opens where the action says it does.
type offerScreen struct {
	app    *app
	act    *spec.Action
	next   func() tea.Cmd
	picker *picker
}

func newOffer(a *app, act *spec.Action, next func() tea.Cmd) *offerScreen {
	s := &offerScreen{app: a, act: act, next: next}
	s.picker = newPicker([]item{{title: labelYes(), key: keyYes}, {title: labelNo(), key: keyNo}})
	s.picker.describe(act.Question(a.store.Get))
	if act.Declines() {
		s.picker.focus(keyNo)
	}
	return s
}

func (s *offerScreen) Title() string { return s.act.Label() }
func (s *offerScreen) Hint() string  { return labelHintChoose() }

func (s *offerScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	s.picker.Update(msg)
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	switch {
	case confirms(key) && s.picker.selected() == keyYes:
		return s, s.next()
	case confirms(key), backs(key):
		return s, pop()
	}
	return s, nil
}

func (s *offerScreen) View(width, height int) string { return s.picker.View(width, height) }

// actionScreen is an action's script running.
//
// It goes the moment the script has worked, taking the action's pages with it,
// back to wherever the action was opened from — which looks again at whatever
// the action was about — or to the page it reports on first, where it has one.
// Where it did not work, it gives way to the page every failure opens on, and
// the way back from that is to the last page: the next thing to try is another
// go at the answers.
type actionScreen struct {
	app   *app
	act   *spec.Action
	depth int // the action's pages on the stack, this one included

	session *exec.Session
}

// actionRanMsg is the script coming back.
type actionRanMsg struct{ err error }

func (s *actionScreen) Title() string { return "" }

func (s *actionScreen) working() bool { return true }

func (s *actionScreen) Hint() string { return labelHintRunning() }

func (s *actionScreen) Init() tea.Cmd {
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
		return s, replace(newFailure(s.act.Label(), ran.err, pop))
	}
	// What the header keeps an eye on may be what this changed.
	done := tea.Batch(recheck(), back(s.depth))
	if !s.act.Reports() {
		return s, done
	}
	// A report shows what the script wrote down, so the answer file is read
	// back first — the same moment a task's report reads it.
	if err := s.app.runner.Imported(); err != nil {
		logging.Warn("%s: %s", s.act.Title, err)
	}
	headline, body := s.act.ReportText(s.app.store.Get)
	told := newReport(headline, body, s.app.store.Get(s.act.Shows))
	return s, replace(&toldScreen{page: told, done: func() tea.Cmd { return done }})
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
// as it says no: what it wrote on stderr is the page, it asks again by itself
// every few seconds, and enter opens the action it falls back on where this
// machine has that. Once every one says yes, the opening goes on.
type gateScreen struct {
	opening
	app  *app
	all  []*spec.Action
	at   int
	next func() screen

	checked bool
	why     string
	offered bool

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
		why     string
		refused bool
		offered bool
		round   int
	}
	gateDueMsg struct{ round int }
)

func (s *gateScreen) action() *spec.Action { return s.all[s.at] }

// fallback is the action the one standing now falls back on, or nil.
func (s *gateScreen) fallback() *spec.Action { return s.app.module.Action(s.action().Fallback) }

func (s *gateScreen) Title() string { return s.action().Label() }

// working is the first question to each action, while it runs. The ones after
// it leave the page standing as it is.
func (s *gateScreen) working() bool { return !s.checked }

func (s *gateScreen) Hint() string {
	switch {
	case !s.checked:
		return labelHintRunning()
	case s.offered:
		return labelHintOpen()
	}
	return labelHintRetry()
}

// Init asks again from the action standing now: when the page first comes up,
// and whenever it is back on top after its fallback was opened.
func (s *gateScreen) Init() tea.Cmd {
	s.checked = false
	return s.check()
}

// check asks the action standing now, as a new round, whether the work may go
// on — and whether this machine has its fallback to open.
func (s *gateScreen) check() tea.Cmd {
	s.round++
	round := s.round
	says := s.app.runner.Says(s.action())
	offered := func() bool { return false }
	if fb := s.fallback(); fb != nil {
		offered = s.app.runner.Offered(fb)
	}
	return func() tea.Msg {
		err := says()
		if err == nil {
			return gateMsg{round: round}
		}
		return gateMsg{why: err.Error(), refused: true, offered: offered(), round: round}
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
				return s, s.Init()
			}
			return s, reset(s.next())
		}
		s.checked, s.why, s.offered = true, msg.why, msg.offered
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
		case confirms(msg) && s.offered:
			return s, s.app.openAction(s.fallback())
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
	if s.offered {
		help = wrap(s.fallback().Help(), bodyWidth(width))
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

// back takes n pages off the stack at once.
func back(n int) tea.Cmd { return func() tea.Msg { return popScreenMsg{n: n} } }
