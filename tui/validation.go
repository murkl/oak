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
// So this is where they are read, all at once, on the far side of the run. The
// page is one line where everything passed, and where something did not it is
// that line and the tasks it went wrong in — open one and the failure is laid
// out exactly as a failed run's is, because it is the same thing: a script, a
// line, a command and what it said.
type validationScreen struct {
	app *app

	// Every check that ran, and the ones worth opening. Both, because the page
	// is a proportion: three failures mean something different out of four than
	// out of forty.
	checks []check
	failed []check

	picker *picker
	done   func() tea.Cmd
}

func newValidation(a *app, checks []check, done func() tea.Cmd) *validationScreen {
	s := &validationScreen{app: a, checks: checks, done: done}
	items := []item{}
	for i, c := range checks {
		if !c.failed() {
			continue
		}
		s.failed = append(s.failed, c)
		items = append(items, item{title: c.task.Label(), key: strconv.Itoa(i)})
	}
	s.picker = newPicker(items)
	return s
}

func (s *validationScreen) Title() string { return labelValidation() }

// Hint: enter is what the page offers — a failure to open, or, where there is
// none to open, the way on. Esc is the way on either way, which is what a page
// with nothing behind it can make of a key that means back everywhere else.
func (s *validationScreen) Hint() string {
	if len(s.failed) == 0 {
		return labelHintContinue()
	}
	return labelHintChecks()
}

func (s *validationScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	s.picker.Update(msg)
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	switch {
	case backs(key):
		return s, s.done()
	case confirms(key):
		if c, found := s.at(s.picker.selected()); found {
			return s, push(newFailure(c))
		}
		return s, s.done()
	}
	return s, nil
}

// at finds the check a row stands for.
func (s *validationScreen) at(key string) (check, bool) {
	i, err := strconv.Atoi(key)
	if err != nil || i < 0 || i >= len(s.checks) {
		return check{}, false
	}
	return s.checks[i], true
}

func (s *validationScreen) View(width, height int) string {
	head := s.headline()
	if len(s.failed) == 0 {
		return head
	}
	return head + "\n\n" + s.picker.View(width, height-2)
}

// headline is the whole verdict in one line: the mark, and how many of how
// many. The mark carries which of the two it is, so the words never have to say
// it twice.
func (s *validationScreen) headline() string {
	passed := len(s.checks) - len(s.failed)
	words := boldStyle.Render(labelChecksPassed(passed, len(s.checks)))
	if len(s.failed) == 0 {
		return accentBold.Render(glyphs.ok) + field(" ") + words
	}
	return failStyle.Render(glyphs.fail) + field(" ") + words
}

// failureScreen is one of them, opened: the task it belongs to over the failure
// itself, laid out the way every other failure in this program is.
//
// Nothing is folded away and nothing is summarised. Somebody on this page is
// about to open a file, and what they need is which one and which line of it.
type failureScreen struct{ c check }

func newFailure(c check) *failureScreen { return &failureScreen{c: c} }

func (s *failureScreen) Title() string { return s.c.task.Label() }
func (s *failureScreen) Hint() string  { return labelHintBack() }

func (s *failureScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && answers(key) {
		return s, pop()
	}
	return s, nil
}

func (s *failureScreen) View(width, height int) string {
	return renderFailure(s.c.err, width)
}
