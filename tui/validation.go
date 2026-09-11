package tui

import (
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
)

// validationScreen is what a run proved about itself, read once it is over.
//
// A task says what it does and, where it can, how to tell that it took: a
// second script that reads the machine the work was done to and changes nothing
// on it. Those run as the work goes, and none of them can stop it — a check
// that disagrees with a task that succeeded is a thing to look at, not a reason
// to abandon an installation that is already on the disk.
//
// So this is where they are read: the count, and under it the tasks the machine
// disagreed with. Open one and the failure is laid out exactly as a failed
// run's is, because it is the same thing — a module, a task, a script, a line,
// a command and what it said.
//
// The page only exists where there is something on it to open. A run that
// agreed with itself has already said so in one line, under the words of
// whatever page it stopped on.
type validationScreen struct {
	app *app

	// Every test that ran, and the ones worth opening. Both, because the page
	// is a proportion: three failures mean something different out of four than
	// out of forty.
	tests  []testResult
	failed []testResult

	picker *picker
	done   func() tea.Cmd
}

// The row that leaves this page. It is a row rather than a key because leaving
// is the one thing here that cannot be taken back — nothing reopens this page,
// and what is on it is the only place the failures are laid out. The NUL prefix
// cannot collide with the numbers the other rows are keyed by.
const keyReviewed = "\x00reviewed"

func newValidation(a *app, tests []testResult, done func() tea.Cmd) *validationScreen {
	s := &validationScreen{app: a, tests: tests, done: done}
	items := []item{}
	for i, r := range tests {
		if !r.failed() {
			continue
		}
		s.failed = append(s.failed, r)
		items = append(items, item{title: r.task.Label(), key: strconv.Itoa(i)})
	}
	items = append(items, item{title: labelReviewed(), detail: labelReviewedHelp(), key: keyReviewed})
	s.picker = newPicker(items)
	return s
}

func (s *validationScreen) Title() string { return labelValidation() }

// Hint: enter opens the failure under the cursor, and the last row is the way
// on. Esc is not offered and does nothing here — see keyReviewed.
func (s *validationScreen) Hint() string { return labelHintChecks() }

func (s *validationScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	s.picker.Update(msg)
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	// Only the row that says so leaves. A key that means back everywhere else
	// would carry off the one page these failures are ever shown on, and the
	// reflex to press it is exactly what somebody who has just been told
	// something went wrong does.
	if !confirms(key) {
		return s, nil
	}
	if s.picker.selected() == keyReviewed {
		return s, s.done()
	}
	if r, found := s.at(s.picker.selected()); found {
		return s, push(newFailure(r.task.Label(), r.err, pop))
	}
	return s, nil
}

// at finds the test a row stands for.
func (s *validationScreen) at(key string) (testResult, bool) {
	i, err := strconv.Atoi(key)
	if err != nil || i < 0 || i >= len(s.tests) {
		return testResult{}, false
	}
	return s.tests[i], true
}

// With the sentence under the rows, because the last of them is the one row
// here that cannot be taken back and has to say so. The others carry none, and
// the space for it is held either way — a cursor moving must not shift the list
// above it.
func (s *validationScreen) View(width, height int) string {
	return s.headline() + "\n\n" + withDetail(s.picker, width, height-2)
}

// headline is the whole verdict in one line: the mark, and how many of how
// many.
func (s *validationScreen) headline() string {
	return failStyle.Render(glyphs.fail) + field(" ") +
		boldStyle.Render(labelTestsPassed(passed(s.tests), len(s.tests)))
}

// failureScreen is one failure, opened: everything there is to know about it,
// under the name of what it happened to.
//
// It is the same page wherever a failure comes from — a test the machine
// disagreed with, or the task that stopped the run — because they are the same
// thing and somebody reading either is after the same answer. What differs is
// the title over it and where leaving it goes, so those are what it is handed.
//
// Nothing is folded away and nothing is summarised. Somebody on this page is
// about to open a file, and what they need is which one and which line of it.
type failureScreen struct {
	title string
	err   error
	done  func() tea.Cmd

	// hint is what the footer calls leaving, kept as the label rather than the
	// word it renders to: the language can change while this page is up.
	hint func() string
}

func newFailure(title string, err error, done func() tea.Cmd) *failureScreen {
	return &failureScreen{title: title, err: err, done: done, hint: labelHintBack}
}

// hinted names the way out for a page there is no going back from — where what
// is behind it is leaving rather than the page it was opened from.
func (s *failureScreen) hinted(hint func() string) *failureScreen {
	s.hint = hint
	return s
}

func (s *failureScreen) Title() string { return s.title }
func (s *failureScreen) Hint() string  { return s.hint() }

// Closed by the one key that means yes, and by nothing else. Everywhere else a
// page that is only read answers to esc as well, because leaving it costs
// nothing; here it costs the only account of what went wrong that this run will
// ever give — and the page it is read on is reached by pressing enter, which
// makes a second enter the one keystroke nobody arrives here holding.
func (s *failureScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && confirms(key) {
		return s, s.done()
	}
	return s, nil
}

func (s *failureScreen) View(width, height int) string {
	return renderFailure(s.err, width)
}
