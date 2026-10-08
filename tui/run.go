package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/murkl/oak/internal/exec"
	"github.com/murkl/oak/internal/logging"
	"github.com/murkl/oak/internal/spec"

	tea "github.com/charmbracelet/bubbletea"
)

// runScreen runs the tasks one after another and shows each name with a mark,
// never a line of what they print, which goes to the log. A task that declared
// its output its progress gets the last line it drew dimmed under its name, and
// a task that asks first interrupts the list.
type runScreen struct {
	app *app

	steps []*spec.Task
	state []mark

	// Where the two outcomes lead. Both are given by whoever started the run,
	// because only they know what a success means next and where a failure is
	// fixed - an installation that fails belongs back among the answers.
	then func() tea.Cmd
	back func() tea.Cmd

	at      int   // the task running now, or len(steps) once they all have
	stage   phase // how far the one at the cursor has got through what it declared
	ask     *ask
	asking  *picker
	told    *report
	session *exec.Session
	err     error
	done    bool

	// after is the rows a finished run ends on - its test results where
	// something failed, and what the module offers once the work is done - and
	// nil where there are none.
	after *picker

	// tests is what the run proved about itself as it went: one entry per task
	// that declared a test and got as far as running it. A failed one does not
	// stop anything - the work itself said it worked - so they are counted
	// under the run and read behind its test results row.
	tests []outcome

	// optional is every task that declared the result stands without it, and
	// what its work came to. A failed one is gone past rather than stopped at,
	// so it is counted and read beside the tests.
	optional []outcome

	// settled is whether a keystroke means anything yet. It is false while
	// something is running and for a moment after every question and every
	// result - see settleFor.
	settled bool

	// When the run began, and how long it turned out to take. Read from the wall
	// clock rather than counted off the frames: the frames stop while a task
	// asks and while one has the terminal to itself, and an installation is not
	// shorter for having waited for somebody.
	started time.Time
	took    time.Duration
}

// mark is what a row has against it, which is also the whole of a run's state.
type mark int

const (
	pending mark = iota
	ran
	skipped
	broken
)

// phase is how far the task at the cursor has got through what it declared
// about itself: a value it has to ask for, then the offer, then the work, then
// the check that the work took, then whatever it has to report of what it came
// to. Each is skipped by a task that declared none, and the order is the useful
// one - an offer can name what was just chosen, a check runs while what it
// looks at is freshest, and a report can name what the work produced.
type phase int

const (
	phaseAsk phase = iota
	phaseConfirm
	phaseRun
	phaseCheck
	phaseReport
)

// outcome is what something the run went on past came to - a task's own test,
// or the work of a task the result stands without: the task it belongs to, and
// the failure where there was one.
type outcome struct {
	task *spec.Task
	err  error
}

// failed reports whether this one is worth reading about afterwards.
func (r outcome) failed() bool { return r.err != nil }

// passed is how many of them came out well.
func passed(outcomes []outcome) int {
	n := 0
	for _, r := range outcomes {
		if !r.failed() {
			n++
		}
	}
	return n
}

// settleFor is the pause before a keystroke counts, after a question appears
// and after the run ends. Keys pressed while waiting arrive the instant the
// screen changes and would otherwise answer what nobody has read.
const settleFor = 618 * time.Millisecond

func newRun(a *app, steps []*spec.Task, then, back func() tea.Cmd) *runScreen {
	return &runScreen{app: a, steps: steps, state: make([]mark, len(steps)), then: then, back: back}
}

func (s *runScreen) Title() string { return "" }

// crumbRoot: a run is something happening rather than a place, so the pages
// that led here are spent and the trail is left to the list.
func (s *runScreen) crumbRoot() bool { return true }

// working is what puts the turning mark in the header: something is running,
// which a question waiting for an answer is not, and neither is a page being
// read. Fetching the answers to one still is - that is a command of the module's,
// running like any other.
func (s *runScreen) working() bool {
	switch {
	case s.done, s.told != nil:
		return false
	case s.ask != nil:
		return s.ask.loading
	}
	return s.asking == nil
}

// holds: nothing in a run has a page behind it, since the work and the question
// it stopped on are already under way, so esc and backspace mean what ctrl+c
// means. Once over, the run is a page closed like any other.
func (s *runScreen) holds() bool { return !s.done && s.told == nil }

// takesText: the narrowing box over a question the run stopped for. It has the
// first claim on esc - the box is closed before the question is left - and
// while it is open a letter is a character being typed.
func (s *runScreen) takesText() bool { return s.ask != nil && s.ask.filter.active() }

// status is the counter beside it: which step of how many. A run stopped on
// something it has to report is not counting: what that page says is that a
// thing is finished, and a number beside it saying how much is left would take
// it straight back.
func (s *runScreen) status() string {
	if s.done || s.told != nil {
		return ""
	}
	return labelCounter(min(s.at+1, len(s.steps)), len(s.steps))
}

func (s *runScreen) Hint() string {
	switch {
	case s.ask != nil:
		return s.ask.Hint()
	case s.asking != nil:
		return labelHintAnswer()
	case !s.settled:
		return labelHintRunning()
	case s.told != nil:
		return s.told.Hint()
	case s.after != nil:
		return labelHintChecks()
	}
	return s.app.hintEnd(labelHintClose())
}

func (s *runScreen) Init() tea.Cmd {
	// Asked again on the way back from a page drawn over the run. The run is
	// already going by then, and starting it twice would reset the clock and
	// repeat whatever step it was on.
	if !s.started.IsZero() {
		return nil
	}
	s.started = time.Now()
	// The answers this run is started with are written out whole first. A task
	// that copies the file or shares it then hands on exactly those, not a line
	// appended twice by hand or a key an older release asked and this one does
	// not.
	return tea.Batch(s.app.save(), s.step())
}

// elapsed is how long this run has been going, and how long it went for once it
// is over - one answer, so the headline reads the same number before and after.
func (s *runScreen) elapsed() time.Duration {
	if s.done {
		return s.took
	}
	return time.Since(s.started)
}

// clock renders a duration the way a clock does: minutes and seconds, with
// hours in front of them once there are any. An installation is measured in
// minutes, so that is the unit it is read in - and one that runs past an hour
// has to say so rather than counting up to 74 minutes.
func clock(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	secs := int(d.Seconds())
	if hours := secs / 3600; hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, secs/60%60, secs%60)
	}
	return fmt.Sprintf("%02d:%02d", secs/60, secs%60)
}

type (
	stepDoneMsg  struct{ err error }
	simulatedMsg struct{}
	testedMsg    struct{ err error }
	settleMsg    struct{}
)

// simulateFor is how long a simulated task is shown running. Long enough for
// the row to be read as it passes, which is what makes a simulated run
// something to watch and to photograph.
const simulateFor = time.Second

// step takes on the task at the cursor: asks what it needs, offers it if it is
// an offer, runs it, and ends the run when none are left. It is called again
// after each, so the frame is redrawn between them.
func (s *runScreen) step() tea.Cmd {
	if s.at >= len(s.steps) {
		return s.finish(nil)
	}
	e := s.steps[s.at]
	if s.stage == phaseAsk {
		s.stage = phaseConfirm
		if e.Asks != "" {
			s.ask = newAsk(s.app.module.Var(e.Asks))
			return tea.Batch(s.ask.Init(s.app), s.settle())
		}
	}
	if s.stage == phaseConfirm {
		s.stage = phaseRun
		if e.Confirms() {
			s.asking = newPicker([]item{
				{title: labelYes(), key: keyYes},
				{title: labelNo(), key: keyNo},
			})
			// No first, like every confirm: the page before it was left with
			// enter. Yes only after a task this one follows from has run.
			if !s.follows(e) {
				s.asking.focus(keyNo)
			}
			return s.settle()
		}
	}
	if s.stage == phaseCheck {
		s.stage = phaseReport
		if cmd := s.prove(e); cmd != nil {
			return cmd
		}
	}
	if s.stage == phaseReport {
		return s.tell(e)
	}
	return s.start()
}

// follows reports whether a task e names under yes-after ran earlier in this
// run. Passed over or declined is not ran.
func (s *runScreen) follows(e *spec.Task) bool {
	for i, t := range s.steps[:s.at] {
		if s.state[i] == ran && slices.Contains(e.YesAfter, t.ID()) {
			return true
		}
	}
	return false
}

// prove runs a task's test: read the machine, change nothing, say whether the
// work took. Nil where there is none or validation is off, and a test that will
// not start is a failed test rather than a failed run.
func (s *runScreen) prove(e *spec.Task) tea.Cmd {
	if !e.Checks() || !s.app.prefs.Validates() {
		return nil
	}
	session, err := s.app.runner.Test(e)
	if err != nil {
		s.tests = append(s.tests, outcome{task: e, err: err})
		return nil
	}
	s.settled = false
	s.session = session
	return proved(session)
}

func proved(session *exec.Session) tea.Cmd {
	return func() tea.Msg {
		<-session.Done()
		return testedMsg{session.Err()}
	}
}

// tell puts up what a task had to report of what it just did, and holds the run
// there until it has been read.
//
// A task with nothing to report - which is nearly all of them - passes straight
// through.
func (s *runScreen) tell(e *spec.Task) tea.Cmd {
	if !e.Reports() {
		return s.advance()
	}
	headline, body := e.ReportText(s.app.store.Get)
	note, alarm := s.tally()
	s.told = newReport(headline, body, "").says(note, alarm)
	return tea.Batch(s.app.save(), s.settle())
}

// advance moves the cursor to the next task, which starts over at the first
// phase - the one after it has its own questions to be asked.
func (s *runScreen) advance() tea.Cmd {
	s.at++
	s.stage = phaseAsk
	return s.step()
}

// start runs the task at the cursor in the background.
func (s *runScreen) start() tea.Cmd {
	s.settled = false
	e := s.steps[s.at]
	if s.app.runner.Simulated(e) {
		logging.Info("%s: simulated", e.Title)
		return after(simulateFor, func(time.Time) tea.Msg { return simulatedMsg{} })
	}
	session, err := s.app.runner.Start(e)
	if err != nil {
		return s.fell(err)
	}
	s.session = session
	// No clock of its own: the frame already repaints while a page reports it is
	// working, which is what makes the clock in the headline count rather than
	// sit at the second the run began.
	return waitFor(session)
}

func waitFor(session *exec.Session) tea.Cmd {
	return func() tea.Msg {
		<-session.Done()
		return stepDoneMsg{session.Err()}
	}
}

// fell is what becomes of a task whose work failed: one the result stands
// without is noted and passed, without test or report, and any other ends the
// run.
func (s *runScreen) fell(err error) tea.Cmd {
	e := s.steps[s.at]
	if !e.AllowFailure {
		return s.finish(err)
	}
	logging.Warn("%s: %s", e.Title, err)
	s.session = nil
	s.state[s.at] = broken
	s.optional = append(s.optional, outcome{task: e, err: err})
	return s.advance()
}

// finish ends the run, one way or the other.
//
// A run that could not go on stops on the same page a run that finished stops
// on, under the other mark: it is the same thing being said, and everything
// there is to know about the failure is one keystroke behind it.
func (s *runScreen) finish(err error) tea.Cmd {
	s.took = time.Since(s.started)
	s.done, s.err, s.session, s.asking, s.ask = true, err, nil, nil, nil
	if err != nil {
		logging.Error("%s", err)
		s.told = newReport(s.failed(), labelRunStopped(s.stoppedAt()), "").stop()
	} else {
		logging.Info("run: ok")
		s.after = s.app.offers(s.app.module.Rules.OnSuccess, s.resultsRow()...)
	}
	// Whatever a run was given is gone the moment it is over, whether it worked
	// or not: a failed installation is one that gets looked at, and nothing
	// typed in confidence should still be in memory while that happens.
	s.app.store.Forget()
	return s.settle()
}

// settle starts the clock that makes what is on screen answerable.
func (s *runScreen) settle() tea.Cmd {
	s.settled = false
	return after(settleFor, func(time.Time) tea.Msg { return settleMsg{} })
}

// stop kills the task that is running, and everything it started. Reached only
// once somebody has chosen a row on the way out - asking to leave a run does
// not stop it, saying so does.
func (s *runScreen) stop() {
	if s.session == nil {
		return
	}
	logging.Warn("run: stopped")
	s.session.Kill()
	// Let go of it, so a second way out asking the same thing of this page says
	// so once. Whoever is waiting on the run holds its own reference.
	s.session = nil
}

func (s *runScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case stepDoneMsg:
		if msg.err != nil {
			return s, s.fell(msg.err)
		}
		s.state[s.at] = ran
		if s.steps[s.at].AllowFailure {
			s.optional = append(s.optional, outcome{task: s.steps[s.at]})
		}
		s.stage = phaseCheck
		return s, s.step()

	case simulatedMsg:
		s.state[s.at] = ran
		// Straight to what it has to report: there is nothing on the machine
		// for its test to read.
		s.stage = phaseReport
		return s, s.step()

	case testedMsg:
		e := s.steps[s.at]
		s.session = nil
		s.tests = append(s.tests, outcome{task: e, err: msg.err})
		if msg.err != nil {
			// Not a failed run: the work said it worked, and something looking
			// at the machine afterwards disagreed. The run carries on and the
			// page after it is where the two are put side by side.
			logging.Warn("%s: %s", e.Title, msg.err)
		}
		return s, s.step()

	case askedMsg:
		skip, err := s.ask.fill(msg, s.app.store.Get(s.ask.v.Name))
		if err != nil {
			return s, s.finish(err)
		}
		if skip {
			logging.Info("%s: %s", s.steps[s.at].Title, "nothing to choose from")
			s.ask = nil
			s.state[s.at] = skipped
			return s, s.advance()
		}
		return s, nil

	case settleMsg:
		s.settled = true
		return s, nil

	case tea.KeyMsg:
		// The keys that ask to leave never reach this page - the model takes
		// them, and asking is not stopping: the page it opens is drawn over
		// this one and the run carries on behind it. See holds and leave.go.
		if !s.settled {
			// Killing a half-finished package transaction is worse than waiting
			// for it, so nothing else means anything while a task runs, and
			// nothing at all for a moment after a question or a result appears.
			return s, nil
		}
		if s.ask != nil {
			return s, s.answerAsk(msg)
		}
		if s.asking != nil {
			return s, s.answer(msg)
		}
		// A page that only had to be read is closed the way the result of a run
		// is: deliberately, with enter or esc, and by nothing else.
		if s.told != nil {
			if answers(msg) {
				// The page a run stopped on: everything about the failure is
				// behind it, and the way on from there is back to the answers.
				if s.err != nil {
					return s, push(newFailure(s.stoppedAt(), s.err, s.back).offering(s.app))
				}
				s.told = nil
				return s, s.advance()
			}
			return s, nil
		}
		if s.after != nil {
			return s, s.choose(msg)
		}
		// The result is dismissed deliberately or not at all: enter and esc,
		// nothing else. Every other key - and every scroll, which arrives here
		// as an arrow - leaves the report on screen.
		if answers(msg) {
			return s, s.then()
		}
	}
	return s, nil
}

// choose is a row under a finished run being chosen: its test results, an
// action the module offers once the work is done, or the last row, which goes
// where the run leads.
func (s *runScreen) choose(key tea.KeyMsg) tea.Cmd {
	s.after.Update(key)
	if !confirms(key) {
		return nil
	}
	selected := s.after.selected()
	if selected == keyResults {
		note, _ := s.tally()
		return push(newResults(note, s.failures()))
	}
	if act := s.app.action(selected); act != nil {
		return s.app.openAction(act)
	}
	return s.then()
}

// keyResults is the row that opens the test results. The NUL prefix cannot
// collide with anything a module names.
const keyResults = "\x00results"

// resultsRow is that row where the run went on past something, and nothing
// where it did not: a run that agreed with itself has said so in one line.
func (s *runScreen) resultsRow() []item {
	if len(s.failures()) == 0 {
		return nil
	}
	return []item{{title: labelResults(), key: keyResults}}
}

// answerAsk takes the value a task asked for and carries on into whatever else
// that task declared. There is no way past the question but answering it: the
// work it belongs to has already started, and esc has nothing behind it to go
// back to.
func (s *runScreen) answerAsk(key tea.KeyMsg) tea.Cmd {
	cmd, given := s.ask.Update(key, s.app)
	if !given {
		return cmd
	}
	logging.Info("%s: %s", s.ask.v.Name, s.app.store.Get(s.ask.v.Name))
	s.ask = nil
	return tea.Batch(cmd, s.step())
}

// answer takes the yes or no to a task that asked. No is an answer like any
// other: the task is skipped, and the run carries on.
func (s *runScreen) answer(key tea.KeyMsg) tea.Cmd {
	s.asking.Update(key)
	if !confirms(key) {
		return nil
	}
	yes := s.asking.selected() == keyYes
	s.asking = nil
	if yes {
		return s.start()
	}
	logging.Info("%s: declined", s.steps[s.at].Title)
	s.state[s.at] = skipped
	return s.advance()
}

// The two rows of a question. The NUL prefix cannot collide with anything a
// module names.
const (
	keyYes = "\x00yes"
	keyNo  = "\x00no"
)

func (s *runScreen) View(width, height int) string {
	// A report is the whole page. The line that says what is running is what it
	// is standing in for: the run has stopped, and there is nothing above the
	// mark it draws for that line to be about.
	if s.told != nil {
		return s.told.View(width, height)
	}
	var b strings.Builder
	b.WriteString(s.headline() + "\n")
	used := 2 // the headline, and the blank line under it
	if verdict := s.verdict(width); verdict != "" {
		b.WriteString(verdict + "\n")
		used++
	}
	b.WriteString("\n")
	switch {
	case s.ask != nil:
		return b.String() + s.ask.View(width, height-used)
	case s.asking != nil:
		return b.String() + s.question(width, height-used)
	case s.after != nil:
		return b.String() + s.after.View(width, height-used)
	}
	return b.String() + s.list(width, height-used)
}

// verdict is what the tests came to, under the line that says the run is over,
// inked where some disagreed. On the success page only, since a failed run's
// tests are not worth counting.
func (s *runScreen) verdict(width int) string {
	note, alarm := s.tally()
	if !s.done || s.err != nil || note == "" {
		return ""
	}
	ink := mutedStyle
	if alarm {
		ink = alertStyle
	}
	return field(glyphBlank) + ink.Render(truncate(note, width-markW))
}

// failures is everything the run went on past, in the order it is read: the
// work that did not happen before the checks that disagreed with work that did.
func (s *runScreen) failures() []outcome {
	var out []outcome
	for _, r := range slices.Concat(s.optional, s.tests) {
		if r.failed() {
			out = append(out, r)
		}
	}
	return out
}

// tally is what the run has come to beyond its list, the optional tasks that
// failed and the tests that ran, and whether it is worth a look. Worked out in
// one place, since a page mid-run and the end both read it.
func (s *runScreen) tally() (string, bool) {
	var said []string
	alarm := false
	if missed := len(s.optional) - passed(s.optional); missed > 0 {
		said = append(said, labelOptionalFailed(missed, len(s.optional)))
		alarm = true
	}
	if len(s.tests) > 0 {
		ok := passed(s.tests)
		said = append(said, labelTestsPassed(ok, len(s.tests)))
		alarm = alarm || ok < len(s.tests)
	}
	return strings.Join(said, " · "), alarm
}

// headline is the run itself: what is happening, or what happened - and, either
// way, the clock on it. An installation is minutes of a list filling in with
// nothing to judge it against; how long it has been going is the one thing
// somebody watching it cannot work out for themselves, and how long it took is
// the same answer once it is over.
func (s *runScreen) headline() string {
	// A task that has stopped the run to ask something is named in place of the
	// run itself: what is on screen is that one question, and the clock has
	// nothing to do with how long somebody takes to answer it.
	switch {
	case s.asking != nil, s.ask != nil && !s.ask.loading:
		return accentBold.Render(glyphs.ask) + field(" ") + boldStyle.Render(s.steps[s.at].Label())
	case !s.done:
		return accentStyle.Render(spinFrame()) + field(" ") + boldStyle.Render(s.running())
	}
	return accentBold.Render(glyphs.ok) + field(" ") + boldStyle.Render(s.succeeded())
}

// The three things a run says about itself. None of them names the module: the
// frame above says which one this is on every page, and the only thing the line
// adds is how far the work has got.
func (s *runScreen) running() string {
	return labelRunningFor(clock(s.elapsed()))
}

func (s *runScreen) succeeded() string { return labelRunDone(clock(s.took)) }
func (s *runScreen) failed() string    { return labelRunFailed() }

// stoppedAt is what the run was doing when it could not go on, which is the one
// thing the page saying so has to name. Past the last step there is no step to
// name, and what is left is the module the run belonged to.
func (s *runScreen) stoppedAt() string {
	if s.at < len(s.steps) {
		return s.steps[s.at].Label()
	}
	return s.app.module.Name()
}

// question is what a task asked, with the answers filled into it, and the
// two ways to answer under it.
func (s *runScreen) question(width, height int) string {
	text := paragraph(s.steps[s.at].Question(s.app.store.Get), width)
	used := strings.Count(text, "\n") + 3 // the text itself, and the blank line under it
	return text + "\n\n" + s.asking.View(width, height-used)
}

// list is every task with its mark, the window following the cursor down a run
// too long to fit. A task's progress line costs a row under its name and is cut
// at its start, since a bar says how far it got at its end.
func (s *runScreen) list(width, height int) string {
	drawn := s.progress()
	rows := height
	if drawn != "" {
		rows--
	}
	if rows < 1 {
		return ""
	}
	top := 0
	if s.at >= rows {
		top = min(s.at-rows+1, len(s.steps)-rows)
	}
	var b strings.Builder
	for i := top; i < min(top+rows, len(s.steps)); i++ {
		if i > top {
			b.WriteByte('\n')
		}
		b.WriteString(s.line(i, width))
		if i == s.at && drawn != "" {
			b.WriteString("\n" + field(glyphBlank) + mutedStyle.Render(truncateStart(drawn, width-markW)))
		}
	}
	return b.String()
}

// progress is the line the running task drew last, where it declared its
// output its progress, and nothing everywhere else - its test included, which
// reads rather than works.
func (s *runScreen) progress() string {
	if s.done || s.at >= len(s.steps) || s.stage != phaseRun || s.session == nil || !s.steps[s.at].Progress {
		return ""
	}
	return s.session.Latest()
}

func (s *runScreen) line(i, width int) string {
	title := truncate(s.steps[i].Label(), width-markW)
	switch {
	case s.state[i] == ran:
		return accentStyle.Render(glyphs.ok) + field(" ") + softStyle.Render(title)
	case s.state[i] == skipped:
		return mutedStyle.Render(glyphs.skip) + field(" ") + mutedStyle.Render(title)
	case s.state[i] == broken:
		return failStyle.Render(glyphs.fail) + field(" ") + softStyle.Render(title)
	case i == s.at && !s.done:
		return accentStyle.Render(spinFrame()) + field(" ") + boldStyle.Render(title)
	}
	// Still to come: no mark at all, so the column reads as a checklist filling
	// in from the top rather than as a row of empty boxes.
	return field(glyphBlank) + mutedStyle.Render(title)
}

// markW is what the mark column costs a title: the glyph and the space after it.
const markW = 2
