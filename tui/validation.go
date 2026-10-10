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

// failureScreen is one failure opened in full under the name of what it
// happened to, the same page for a failed test and a stopped run. Nothing is
// folded away, since the reader is about to open a file at a line.
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
	// for a run that failed - see offering. Nil everywhere else.
	app    *app
	picker *picker

	// sure is whether leaving asks first: the page a run stopped on is the
	// only one offering what to do about it.
	sure bool
}

func newFailure(title string, err error, done func() tea.Cmd) *failureScreen {
	return &failureScreen{title: title, err: err, done: done, hint: labelHintBack}
}

// saying puts what a failure means, in the module's own words, over the report.
func (s *failureScreen) saying(text string) *failureScreen {
	s.said = text
	return s
}

// hinted names the way out for a page there is no going back from - where what
// is behind it is leaving rather than the page it was opened from.
func (s *failureScreen) hinted(hint func() string) *failureScreen {
	s.hint = hint
	return s
}

// offering puts the actions a module names for a run that failed under the
// report, as rows above the one that leaves it - sharing the log, say. Where
// the module names none, or this machine has none of them, the page is the
// report alone.
func (s *failureScreen) offering(a *app) *failureScreen {
	s.app, s.picker = a, a.offers(a.module.Rules.OnFailure)
	return s
}

// asking makes leaving this page a question that opens on No.
func (s *failureScreen) asking() *failureScreen {
	s.sure = true
	return s
}

// leave goes where the page leads, asked first where it says so.
func (s *failureScreen) leave() tea.Cmd {
	if !s.sure {
		return s.done()
	}
	return push(newYesNo(labelBackToMenu(), labelBackToMenuHelp(), s.done))
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

// Closed by enter alone, not esc, since leaving costs the run's only account of
// what went wrong and enter is the key nobody arrives here holding. With rows
// under it, that enter is on the last row.
func (s *failureScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	if s.picker == nil {
		if confirms(key) {
			return s, s.leave()
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
		return s, s.leave()
	}
	return s, nil
}

// View is the report, and under it the rows where there are any. Only their
// names: the report is what this page is for, and the room a sentence under
// the rows would take is the room its last lines need. The rows always fit,
// and the report gives up the front of what the tool said for them.
func (s *failureScreen) View(width, height int) string {
	head := ""
	if s.said != "" {
		head = refusal(s.said, width) + "\n\n"
	}
	room := height - strings.Count(head, "\n")
	if s.picker != nil {
		room -= s.picker.height(width) + 1
	}
	report := head + renderFailure(s.err, width, room)
	if s.picker == nil {
		return report
	}
	report = strings.TrimRight(report, "\n") + "\n\n"
	return report + s.picker.View(width, height-strings.Count(report, "\n"))
}
