package tui

import tea "github.com/charmbracelet/bubbletea"

// fatalScreen is something the program cannot work around and the user cannot
// answer, drawn like a stopped run with the check's own words as its subject.
// The full failure is one key away, and after that there is only leaving.
type fatalScreen struct {
	app  *app
	err  error
	page *report
}

func newFatal(a *app, err error) *fatalScreen {
	return &fatalScreen{app: a, err: err, page: newReport(labelCannotContinue(), said(err), "").stop()}
}

func (s *fatalScreen) Title() string { return "" }
func (s *fatalScreen) Hint() string  { return labelHintContinue() }

func (s *fatalScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	// Deliberately answers, not any key: this page arrives while somebody is
	// still typing at the page before it, and a stray keystroke must not close
	// the one explanation they are going to get.
	if answers(key) {
		return s, push(newFailure(unitOf(s.err), s.err, leave).hinted(s.quit))
	}
	return s, nil
}

// quit is what the footer calls the way out of the page behind this one: there
// is nothing after it but leaving the program, or - where the module says how
// this machine is put down - the page that asks about that.
func (s *fatalScreen) quit() string { return s.app.hintEnd(labelHintQuit()) }

func (s *fatalScreen) View(width, height int) string { return s.page.View(width, height) }
