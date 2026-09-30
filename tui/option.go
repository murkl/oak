package tui

import (
	"strings"
	"time"

	"github.com/murkl/oak/internal/exec"
	"github.com/murkl/oak/internal/logging"
	"github.com/murkl/oak/internal/spec"

	tea "github.com/charmbracelet/bubbletea"
)

// An option is opened from a row — on the menu, or on the page every way out
// arrives at — or stands in front of the work while its start says no. Opening
// one is its pages, one to a screen and asked the way every question is, and
// then its script, on a page of its own that goes the moment it has worked.
// What the option is for is the module's business: this file only walks
// through what option.yaml declares.

// offeredMsg is what an option's requires said about this machine. The model
// takes it, whichever page is in front when it lands.
type offeredMsg struct {
	o   *spec.Option
	yes bool
}

// lookFor asks whether this machine has each of the options that say what they
// require, beside the drawing rather than in it. The one that says nothing is
// known without asking — see has.
func (a *app) lookFor() tea.Cmd {
	var cmds []tea.Cmd
	for _, o := range a.module.Options {
		if o.Requires.Empty() {
			continue
		}
		ask := a.runner.Offered(o)
		cmds = append(cmds, func() tea.Msg { return offeredMsg{o: o, yes: ask()} })
	}
	return tea.Batch(cmds...)
}

// has reports whether this machine has an option, as its requires last said.
// Until it has said, the row is not shown: a row that would be taken away
// again is worse than one that lands a moment late.
func (a *app) has(o *spec.Option) bool { return o.Requires.Empty() || a.offered[o] }

// rows is the options standing as a row on one page right now.
func (a *app) rows(where string) []*spec.Option {
	var out []*spec.Option
	for _, o := range a.module.Menu(where) {
		if a.has(o) {
			out = append(out, o)
		}
	}
	return out
}

// keyOption is what a row standing for an option is keyed by, in front of the
// option's folder. The NUL prefix cannot collide with anything a module names.
const keyOption = "\x00option:"

// optionRow is the row an option stands as: its title, and what it does under
// it.
func optionRow(o *spec.Option) item {
	return item{title: o.Label(), detail: o.Help(), key: keyOption + o.ID()}
}

// option is the option a row stands for, or nil where the row stands for
// something else.
func (a *app) option(key string) *spec.Option {
	id, ok := strings.CutPrefix(key, keyOption)
	if !ok {
		return nil
	}
	for _, o := range a.module.Options {
		if o.ID() == id {
			return o
		}
	}
	return nil
}

// openOption is an option's first page, or its work where it asks nothing. Every page
// is pushed onto the one before it, so esc goes back a page the way it does
// through any run of questions, and the work page knows how many to take away
// again once it has worked.
func (a *app) openOption(o *spec.Option) tea.Cmd { return a.page(o, 0, 0) }

// page is what follows the page before it: the next one that applies, given
// the answers so far, or the work. depth is how many of the option's pages are
// on the stack already.
func (a *app) page(o *spec.Option, next, depth int) tea.Cmd {
	for ; next < len(o.Vars); next++ {
		v := o.Vars[next]
		if !v.Applies(a.store.Get) {
			continue
		}
		after := func() tea.Cmd { return a.page(o, next+1, depth+1) }
		if v.Secret() {
			return push(newSecret(a, v, after))
		}
		return push(newField(a, v, after).under(o.Label()))
	}
	return push(&optionScreen{app: a, o: o, depth: depth + 1})
}

// optionScreen is an option's script running, and what it said where it did
// not work.
//
// It goes the moment the script has worked, taking the option's pages with it,
// back to wherever the option was opened from — which looks again at whatever
// the option was about. Where it did not, the report stays up, and the way back
// is to the last page: the next thing to try is another go at the answers.
type optionScreen struct {
	app   *app
	o     *spec.Option
	depth int // the option's pages on the stack, this one included

	session *exec.Session
	busy    bool
	err     error
}

// optionRanMsg is the script coming back.
type optionRanMsg struct{ err error }

func (s *optionScreen) Title() string { return "" }

func (s *optionScreen) working() bool { return s.busy }

func (s *optionScreen) Hint() string {
	if s.busy {
		return labelHintRunning()
	}
	return labelHintBack()
}

func (s *optionScreen) Init() tea.Cmd {
	s.busy, s.err = true, nil
	session, err := s.app.runner.Open(s.o)
	if err != nil || session == nil {
		return func() tea.Msg { return optionRanMsg{err} }
	}
	s.session = session
	return func() tea.Msg {
		<-session.Done()
		return optionRanMsg{session.Err()}
	}
}

func (s *optionScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case optionRanMsg:
		s.busy, s.session = false, nil
		// Whatever the pages were given is gone the moment it is used, whether
		// it worked or not.
		s.app.store.Forget()
		if msg.err != nil {
			logging.Error("%s", msg.err)
			s.err = msg.err
			return s, nil
		}
		// What the header keeps an eye on may be what this changed.
		return s, tea.Batch(recheck(), back(s.depth))
	case tea.KeyMsg:
		if !s.busy && (answers(msg) || backs(msg)) {
			return s, pop()
		}
	}
	return s, nil
}

// stop kills the script, where somebody chose to leave while it ran.
func (s *optionScreen) stop() {
	if s.session != nil {
		s.session.Kill()
		s.session = nil
	}
}

func (s *optionScreen) View(width, height int) string {
	if s.busy {
		return accentStyle.Render(spinFrame()) + field(" ") + boldStyle.Render(s.o.Label())
	}
	head := failStyle.Render(glyphs.fail) + field(" ") + boldStyle.Render(labelRunFailed())
	return head + "\n\n" + renderFailure(s.err, width)
}

// gateScreen is the options the work waits for, one after another, in their
// order. It stands on the first whose start says no, for as long as it says no:
// what that start wrote on stderr is the page, it looks again by itself every
// few seconds, and enter opens the option where this machine has it and it has
// something to open. Once every one says yes, the opening goes on.
type gateScreen struct {
	opening
	app  *app
	all  []*spec.Option
	at   int
	next func() screen

	checked bool
	why     string
	offered bool

	// round is which look still counts. Every new one starts a round, so a clock
	// set before the page moved on is ignored rather than answered twice.
	round int
}

// gateEvery is how often the page looks again by itself: often enough that a
// cable plugged in carries on before anybody reaches for r, rarely enough that
// a check going out to the network is not a load of its own.
const gateEvery = 5 * time.Second

func newGate(a *app, all []*spec.Option, next func() screen) *gateScreen {
	return &gateScreen{app: a, all: all, next: next}
}

type (
	gateMsg struct {
		why     string
		waiting bool
		offered bool
		round   int
	}
	gateDueMsg struct{ round int }
)

func (s *gateScreen) option() *spec.Option { return s.all[s.at] }

func (s *gateScreen) Title() string { return s.option().Label() }

// working is the first look at each option, while it runs. The looks after it
// leave the page standing as it is.
func (s *gateScreen) working() bool { return !s.checked }

func (s *gateScreen) Hint() string {
	switch {
	case !s.checked:
		return labelHintRunning()
	case s.opens():
		return labelHintOpen()
	}
	return labelHintRetry()
}

// opens reports whether enter opens the option: this machine has it, and it has
// something to open.
func (s *gateScreen) opens() bool { return s.offered && !s.option().Work.Empty() }

// Init looks again from the start of the option standing now: when the page
// first comes up, and whenever it is back on top after its option was opened.
func (s *gateScreen) Init() tea.Cmd {
	s.checked = false
	return s.check()
}

// check asks the option standing now, as a new round, whether the work may go
// on — and whether this machine has it to open.
func (s *gateScreen) check() tea.Cmd {
	s.round++
	o, round := s.option(), s.round
	waiting, offered := s.app.runner.Waiting(o), s.app.runner.Offered(o)
	return func() tea.Msg {
		err := waiting()
		if err == nil {
			return gateMsg{round: round}
		}
		return gateMsg{why: err.Error(), waiting: true, offered: offered(), round: round}
	}
}

// wait sets the clock for the next look, as a new round.
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
		if !msg.waiting {
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
		case confirms(msg) && s.opens():
			return s, s.app.openOption(s.option())
		case msg.String() == "r":
			return s, s.check()
		case backs(msg):
			return s, pop()
		}
	}
	return s, nil
}

func (s *gateScreen) View(width, height int) string {
	if !s.checked {
		return accentStyle.Render(spinFrame()) + field(" ") + boldStyle.Render(s.option().Label())
	}
	lines := wrap(s.why, max(width-2, 1))
	if len(lines) == 0 {
		lines = []string{""}
	}
	var b strings.Builder
	b.WriteString(failStyle.Render(glyphs.fail) + field(" ") + boldStyle.Render(lines[0]))
	for _, line := range lines[1:] {
		b.WriteString("\n" + field("  ") + boldStyle.Render(line))
	}
	if s.opens() {
		if help := s.option().Help(); help != "" {
			b.WriteString("\n\n" + paragraph(help, width))
		}
	}
	return b.String()
}

// back takes n pages off the stack at once.
func back(n int) tea.Cmd { return func() tea.Msg { return popScreenMsg{n: n} } }
