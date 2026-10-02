package tui

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// resultsScreen is what a finished run went on past: the optional tasks that
// failed and the tests the machine disagreed with. It is opened from the row of
// the same name under the run, as often as somebody wants, and each row opens
// the failure the way a failed run lays its own out.
type resultsScreen struct {
	// verdict is the count, because the page is a proportion: three failures
	// mean something else out of four than out of forty.
	verdict string
	failed  []outcome
	picker  *picker
}

func newResults(verdict string, failed []outcome) *resultsScreen {
	items := make([]item, 0, len(failed))
	for i, r := range failed {
		items = append(items, item{title: r.task.Label(), key: strconv.Itoa(i)})
	}
	return &resultsScreen{verdict: verdict, failed: failed, picker: newPicker(items)}
}

func (s *resultsScreen) Title() string { return labelResults() }

func (s *resultsScreen) Hint() string { return labelHintList() }

func (s *resultsScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	s.picker.Update(msg)
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	switch {
	case backs(key):
		return s, pop()
	case confirms(key):
		if r, found := s.at(s.picker.selected()); found {
			return s, push(newFailure(r.task.Label(), r.err, pop))
		}
	}
	return s, nil
}

// at finds the failure a row stands for.
func (s *resultsScreen) at(key string) (outcome, bool) {
	i, err := strconv.Atoi(key)
	if err != nil || i < 0 || i >= len(s.failed) {
		return outcome{}, false
	}
	return s.failed[i], true
}

func (s *resultsScreen) View(width, height int) string {
	return s.headline() + "\n\n" + s.picker.View(width, height-2)
}

// headline is the whole verdict in one line: the mark, and how many of how
// many.
func (s *resultsScreen) headline() string {
	return failStyle.Render(glyphs.fail) + field(" ") + boldStyle.Render(s.verdict)
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

	// said is what the failure means in the module's own words, over the
	// report: an action's fail. Empty where it says nothing.
	said string

	// hint is what the footer calls leaving, kept as the label rather than the
	// word it renders to: the language can change while this page is up.
	hint func() string

	// app and picker are the rows under the report where a module names actions
	// for a run that failed — see offering. Nil everywhere else.
	app    *app
	picker *picker
}

func newFailure(title string, err error, done func() tea.Cmd) *failureScreen {
	return &failureScreen{title: title, err: err, done: done, hint: labelHintBack}
}

// saying puts what a failure means, in the module's own words, over the report.
func (s *failureScreen) saying(text string) *failureScreen {
	s.said = text
	return s
}

// hinted names the way out for a page there is no going back from — where what
// is behind it is leaving rather than the page it was opened from.
func (s *failureScreen) hinted(hint func() string) *failureScreen {
	s.hint = hint
	return s
}

// offering puts the actions a module names for a run that failed under the
// report, as rows above the one that leaves it — sharing the log, say. Where
// the module names none, or this machine has none of them, the page is the
// report alone.
func (s *failureScreen) offering(a *app) *failureScreen {
	s.app, s.picker = a, a.offers(a.module.Rules.OnFailure)
	return s
}

// offers is the rows a place names that this machine has, after the runtime's
// own lead rows and above the one that goes on, or nil where there are none. It
// opens on that last row: an action is chosen on purpose, never by an enter
// meant for the page before.
func (a *app) offers(names []string, lead ...item) *picker {
	rows := a.rows(names)
	if len(lead)+len(rows) == 0 {
		return nil
	}
	items := append([]item{}, lead...)
	for _, act := range rows {
		items = append(items, actionRow(act))
	}
	items = append(items, item{title: labelGoOn(), key: keyGoOn})
	p := newPicker(items)
	p.focus(keyGoOn)
	return p
}

// keyGoOn is the row that goes on from a list of actions. The NUL prefix
// cannot collide with anything a module names.
const keyGoOn = "\x00on"

func (s *failureScreen) Title() string { return s.title }

func (s *failureScreen) Hint() string {
	if s.picker != nil {
		return labelHintChecks()
	}
	return s.hint()
}

// Closed by the one key that means yes, and by nothing else. Everywhere else a
// page that is only read answers to esc as well, because leaving it costs
// nothing; here it costs the only account of what went wrong that this run will
// ever give — and the page it is read on is reached by pressing enter, which
// makes a second enter the one keystroke nobody arrives here holding. With rows
// under it, that enter is on the row at the end of them.
func (s *failureScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	if s.picker == nil {
		if confirms(key) {
			return s, s.done()
		}
		return s, nil
	}
	s.picker.Update(msg)
	if !confirms(key) {
		return s, nil
	}
	if act := s.app.action(s.picker.selected()); act != nil {
		return s, s.app.openAction(act)
	}
	if s.picker.selected() == keyGoOn {
		return s, s.done()
	}
	return s, nil
}

// View is the report, and under it the rows where there are any. Only their
// names: the report is what this page is for, and the room a sentence under
// the rows would take is the room its last lines need.
func (s *failureScreen) View(width, height int) string {
	report := renderFailure(s.err, width)
	if s.said != "" {
		report = refusal(s.said, width) + "\n\n" + report
	}
	if s.picker == nil {
		return report
	}
	report = strings.TrimRight(report, "\n") + "\n\n"
	return report + s.picker.View(width, height-strings.Count(report, "\n"))
}
