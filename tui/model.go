package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Model owns the terminal, the screen stack and the chrome around whatever is
// on top of it. A screen never draws its own frame, so there is exactly one
// place that decides what the program looks like — and exactly one place that
// can promise nothing ever escapes it.
type Model struct {
	app    *app
	stack  []screen
	width  int
	height int

	// leaving is the way out, drawn over the page underneath rather than in
	// place of it. It is not part of the stack because it is not somewhere you
	// navigated to: it is a question put over whatever was happening, and
	// whatever was happening carries on behind it until one of its rows is
	// chosen. Nil while nobody is asking to leave.
	leaving screen

	// The opening. splash is the logo while it is up and nil once it has gone;
	// arrived is how far the interface is into coming up behind it.
	splash  *splashModel
	arrived time.Duration

	// wordmark is the same logo, kept once the splash is over: every page of
	// the way in stands under it, the one pushed after the splash has gone as
	// much as the one the splash handed over to. Nil where there is no logo.
	wordmark *splashModel

	// spinning guards the clock against a second chain being started while one
	// is already running; which frame the working mark is on comes from
	// spinFrame, off the wall clock, so two marks on screen at once turn
	// together.
	spinning bool

	// The header's status: whether the module's check has answered yet, and
	// what it said last. reading and waiting are the one chain of reads — a
	// read out, or the clock until the next — the way spinning guards the
	// mark's, and round is which of them still counts: see recheckMsg.
	known, passes    bool
	reading, waiting bool
	round            int

	status    string
	statusBad bool
	quitting  bool
}

// A terminal that has not reported its size yet still has to render something,
// and 80x24 is the size every terminal is at least.
const (
	defaultWidth  = 80
	defaultHeight = 24
)

// statuser is a screen with something to say in the header — the stage counter
// during an install. Optional, like every other screen extra.
type statuser interface{ status() string }

func newModel(a *app, logo string) *Model {
	m := &Model{app: a, width: defaultWidth, height: defaultHeight}
	m.stack = []screen{a.start()}
	if logo == "" {
		// Nothing to come up out of: the interface is simply there.
		m.arrived = fadeFor
		return m
	}
	m.splash = newSplash(logo, a.oak)
	m.wordmark = m.splash
	if framed(m.top()) {
		return m
	}
	m.stage(m.top())
	m.splash.stays = true
	return m
}

// stage hands a page that stands under the wordmark the one it stands under.
func (m *Model) stage(s screen) {
	if st, ok := s.(stager); ok {
		st.stage(m.wordmark)
	}
}

func (m *Model) Init() tea.Cmd {
	cmd := tea.Batch(initOf(m.top()), m.turn(), m.poll())
	if m.splash == nil {
		return cmd
	}
	return tea.Batch(cmd, animTick())
}

func (m *Model) top() screen { return m.stack[len(m.stack)-1] }

// turn keeps the working mark moving, and keeps exactly one chain of ticks
// doing it — a second would turn the mark at twice the rate and outlive the
// work it stands for. It answers nil once there is nothing left to say, which
// is what stops the clock.
func (m *Model) turn() tea.Cmd {
	if m.spinning || m.busy() == nil {
		return nil
	}
	m.spinning = true
	return spinTick()
}

// spinMsg is one turn of the working mark. Its own clock rather than the
// opening's: that one is a fixed animation with an end, this one runs for as
// long as something is happening and has to be startable again afterwards.
type spinMsg struct{}

func spinTick() tea.Cmd {
	return after(spinEvery, func(time.Time) tea.Msg { return spinMsg{} })
}

// animate runs the one clock the opening has: the light going round and the
// logo dimming out, then the interface rising out of the background it left.
// It stops asking for frames the moment nothing is moving any more.
//
// A page that stands under the wordmark is already all there once the splash
// is over, so nothing rises behind it: the frame comes up out of the field when
// that page is answered — see arrive.
func (m *Model) animate() tea.Cmd {
	if m.splash != nil {
		if done := m.splash.advance(); !done {
			setFade(m.splash.light())
			return animTick()
		}
		m.splash = nil
		if !framed(m.front()) {
			m.arrived = fadeFor
		}
	}
	if m.arrived >= fadeFor {
		setFade(1)
		return nil
	}
	m.arrived += animEvery
	setFade(float64(m.arrived) / float64(fadeFor))
	return animTick()
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	was := m.front()
	next, cmd := m.step(msg)
	return next, tea.Batch(cmd, m.arrive(was), m.poll())
}

// statusMsg is what the header's status check answered; statusDueMsg is the
// clock saying it is time to ask it again. Both carry the round they belong to.
//
// recheckMsg is a page saying it changed what the status is about — a network
// just joined — so the next read is asked for now rather than after the
// interval, and whatever the read already out there says is not waited for: it
// was taken before the change.
type (
	statusMsg struct {
		pass  bool
		round int
	}
	statusDueMsg struct{ round int }
	recheckMsg   struct{}
)

func recheck() tea.Cmd { return func() tea.Msg { return recheckMsg{} } }

// poll keeps the header's status current from the moment a module is open,
// one read at a time: the next is asked for once the last has answered and its
// interval has gone by, so a check slower than its interval is never run twice
// at once.
func (m *Model) poll() tea.Cmd {
	if m.reading || m.waiting || m.app.runner == nil {
		return nil
	}
	read := m.app.runner.Status()
	if read == nil {
		return nil
	}
	m.reading = true
	round := m.round
	return func() tea.Msg { return statusMsg{pass: read(), round: round} }
}

// arrive brings the frame up out of the field the moment it appears over a
// page that stood on the field without one, the way it comes up after the
// splash. A fade already under way starts again from nothing rather than
// being joined by a second clock, which would run it at twice the rate.
func (m *Model) arrive(was screen) tea.Cmd {
	if m.splash != nil || framed(was) || !framed(m.front()) {
		return nil
	}
	fading := m.arrived < fadeFor
	m.arrived = 0
	setFade(0)
	if fading {
		return nil
	}
	return animTick()
}

func (m *Model) step(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// A zero-size report happens and would collapse the layout for good.
		if msg.Width > 0 && msg.Height > 0 {
			m.width, m.height = msg.Width, msg.Height
		}
		return m, nil

	case animMsg:
		return m, m.animate()

	case spinMsg:
		m.spinning = false
		return m, m.turn()

	// Each is asked again on the way out of Update, where nothing is out any
	// more — see poll.
	case statusMsg:
		m.reading = false
		if msg.round != m.round {
			return m, nil
		}
		m.known, m.passes, m.waiting = true, msg.pass, true
		return m, after(m.app.module.Status.Interval(), func(time.Time) tea.Msg { return statusDueMsg{msg.round} })

	case statusDueMsg:
		if msg.round == m.round {
			m.waiting = false
		}
		return m, nil

	case recheckMsg:
		m.round++
		m.waiting = false
		return m, nil

	case tea.KeyMsg:
		// The splash answers to one thing only, and swallows the key that says
		// it — otherwise dismissing the logo would also press whatever the page
		// underneath has under the cursor. Arrows are excepted: they are what
		// this terminal makes of a mouse wheel, and are nobody saying anything.
		// So is ctrl+c: it means leave wherever it is pressed, and the logo
		// goes out of its way rather than standing in front of the answer.
		if m.splash != nil {
			if !scrolls(msg) {
				m.splash.skip()
			}
			if !aborts(msg) {
				return m, nil
			}
		}
		m.status = "" // any keystroke clears a flash
		if m.wayOut(msg) {
			return m, m.exit()
		}

	case pushScreenMsg:
		m.stage(msg.s)
		m.stack = append(m.stack, msg.s)
		// A page that runs something starts it in its own Init, so the clock for
		// the mark has to be offered again here: it stopped when the last thing
		// finished, and nothing else would wake it.
		return m, tea.Batch(initOf(msg.s), m.turn())

	case popScreenMsg:
		n := max(msg.n, 1)
		if n >= len(m.stack) {
			// Backing off the last page there is. Nothing is behind it, so this
			// is the same thing as asking to leave.
			return m, m.exit()
		}
		m.stack = m.stack[:len(m.stack)-n]
		if r, ok := m.top().(refresher); ok {
			r.Refresh()
		}
		return m, tea.Batch(initOf(m.top()), m.turn())

	case replaceScreenMsg:
		m.stack[len(m.stack)-1] = msg.s
		return m, tea.Batch(initOf(msg.s), m.turn())

	case resetStackMsg:
		m.stack = []screen{msg.s}
		return m, tea.Batch(initOf(msg.s), m.turn())

	case flashMsg:
		m.status, m.statusBad = msg.text, msg.bad
		return m, nil

	case leaveMsg:
		return m, m.exit()

	case dismissMsg:
		m.leaving = nil
		return m, m.turn()

	case quitMsg:
		return m, m.stopAndQuit()
	}

	// A keystroke belongs to whatever is in front of the user. Everything else
	// belongs to the page that started it — which may be a run going on behind
	// the way out, and which is the whole reason opening that page does not
	// stop one. The way out itself has nothing in flight except while it is
	// carrying a row out.
	_, typed := msg.(tea.KeyMsg)
	if m.leaving != nil && (typed || working(m.leaving)) {
		next, cmd := m.leaving.Update(msg)
		m.leaving = next
		return m, tea.Batch(cmd, m.turn())
	}
	next, cmd := m.top().Update(msg)
	m.stack[len(m.stack)-1] = next
	return m, tea.Batch(cmd, m.turn())
}

// wayOut reports whether a keystroke is somebody asking to leave the program
// rather than answering the page in front of them. It is decided here, once,
// so that every page of every module answers these keys alike and no page has
// to remember to.
//
// ctrl+c always is. q is, wherever a letter is not a character being typed.
// And so are esc and backspace on a page there is nothing behind — a run, and
// the questions it stops to ask — where the way out is what going back means.
// Everywhere else those two are the page's own, and mean one step back.
func (m *Model) wayOut(k tea.KeyMsg) bool {
	if aborts(k) {
		return true
	}
	front := m.front()
	if takesText(front) {
		return false
	}
	return quits(k) || backs(k) && held(front)
}

// front is the page in front of the user: the way out while it is being asked,
// and otherwise whatever is on top of the stack.
func (m *Model) front() screen {
	if m.leaving != nil {
		return m.leaving
	}
	return m.top()
}

// busy is the page with something running, wherever it is: the way out while it
// is putting the machine down, and otherwise the topmost page on the stack that
// is working. Nil when nothing at all is happening.
//
// It is looked for below the top because a run carries on behind the page that
// asks whether to leave it, and the header has to keep saying so.
func (m *Model) busy() screen {
	if m.leaving != nil && working(m.leaving) {
		return m.leaving
	}
	for i := len(m.stack) - 1; i >= 0; i-- {
		if working(m.stack[i]) {
			return m.stack[i]
		}
	}
	return nil
}

func (m *Model) View() string {
	// An empty view on the way out hands the terminal back clean, without the
	// last frame left painted on it.
	if m.quitting {
		return ""
	}
	front := m.front()
	if m.splash != nil && framed(front) {
		return m.splash.View(m.width, m.height)
	}
	if !framed(front) {
		return front.View(m.width, m.height)
	}
	w, h := frameSize(m.width, m.height)

	crumbs := crumbTrail(m.stack)
	// The way out adds its own segment: it is over the page underneath rather
	// than instead of it, and the line above says which of the two is being
	// read.
	if m.leaving != nil {
		crumbs = append(crumbs, m.leaving.Title())
	}

	// A flash stands where the page's own status usually is, and how it is inked
	// says which of the two it is: what went wrong reads as a failure, what
	// merely happened reads like the status it replaced.
	status, alarm := m.pageStatus(), false
	if m.status != "" {
		status, alarm = m.status, m.statusBad
	}
	mark := m.indicator()
	if mark == "" && status == "" {
		mark, status = m.state()
	}

	return renderFrame(m.width, m.height, chrome{
		brand:   m.app.heading(),
		status:  status,
		alarm:   alarm,
		mark:    mark,
		crumb:   breadcrumb(crumbs, w),
		body:    front.View(w, h-headerRows(len(crumbs))),
		hint:    front.Hint(),
		version: m.app.version,
	})
}

// exit is what every way out of the interface goes through — ctrl+c, q, esc out
// of a run, backing off the last page, the end of an installation.
//
// Where the module said how this machine is put down, that is a question rather
// than an exit: the machine booted to run this and there is nothing behind it to
// quit into, so the page offering a restart or a shutdown is what happens next.
// Where it said nothing, the program ends, which is right for something somebody
// started from a shell they are still sitting in.
//
// Nothing is stopped by asking. Wondering how to leave an installation is not
// leaving one, and a package transaction is not interrupted by a keystroke that
// only opened a page: the work carries on behind it and the header keeps saying
// so, until one of the rows on that page is actually chosen — see leave.go.
func (m *Model) exit() tea.Cmd {
	if !m.app.leaves() {
		return m.stopAndQuit()
	}
	if m.leaving == nil {
		m.leaving = newLeave(m.app, m.halt, m.busy() != nil)
	}
	return m.turn()
}

// halt puts down everything that is still running, the moment leaving stops
// being a question and becomes a decision. Leaving a package transaction
// writing to a disk nobody is watching any more is worse than an interrupted
// one.
func (m *Model) halt() {
	for _, s := range m.stack {
		stop(s)
	}
}

// stopAndQuit ends the program, after telling everything still running to put
// down whatever it is holding — so a run can never outlive the interface that
// started it.
func (m *Model) stopAndQuit() tea.Cmd {
	m.halt()
	m.quitting = true
	return tea.Quit
}

// pageStatus is whatever has something to say about itself in the header: what
// is running, wherever it is, and otherwise the page being read. Most pages
// have nothing to say.
func (m *Model) pageStatus() string {
	for _, s := range []screen{m.busy(), m.front()} {
		if t, ok := s.(statuser); ok && t.status() != "" {
			return t.status()
		}
	}
	return ""
}

// indicator is the working mark: the spinner while anything at all is running,
// and nothing otherwise. One slot, one meaning.
func (m *Model) indicator() string {
	if m.busy() != nil {
		return accentStyle.Render(spinFrame())
	}
	return ""
}

// state is the header's status as the module's check last left it: Oak's mark
// for yes or no, and the words the module gave each. Nothing until the check
// has answered once — a mark shown before then would claim a state nothing has
// read.
func (m *Model) state() (mark, words string) {
	if !m.known {
		return "", ""
	}
	st := m.app.module.Status
	if m.passes {
		return goodStyle.Render(glyphs.on), st.Words(true)
	}
	return mutedStyle.Render(glyphs.off), st.Words(false)
}

// headerRows is how much of the frame the chrome takes, so a screen is told the
// height it actually has.
func headerRows(crumbs int) int {
	rows := 4 // brand, rule, rule, footer
	if crumbs > 0 {
		rows += 2 // breadcrumb and the blank line under it
	}
	return rows
}
