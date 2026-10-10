package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/murkl/oak/internal/logging"
)

// yesNoScreen asks before a step there is no way back from, in a sentence that
// says what it does. It opens on No, since an enter meant for the page before
// must not land here, and No or esc goes back to that page.
type yesNoScreen struct {
	title  string
	yes    func() tea.Cmd
	picker *picker
}

func newYesNo(title, help string, yes func() tea.Cmd) *yesNoScreen {
	s := &yesNoScreen{title: title, yes: yes}
	s.picker = newPicker([]item{
		{title: labelYes(), key: keyYes},
		{title: labelNo(), key: keyNo},
	})
	s.picker.describe(help)
	s.picker.focus(keyNo)
	return s
}

func (s *yesNoScreen) Title() string { return s.title }
func (s *yesNoScreen) Hint() string  { return labelHintChoose() }

func (s *yesNoScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	s.picker.Update(msg)
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	switch {
	case confirms(key):
		if s.picker.selected() != keyYes {
			return s, pop()
		}
		return s, s.yes()
	case backs(key):
		return s, pop()
	}
	return s, nil
}

func (s *yesNoScreen) View(width, height int) string { return s.picker.View(width, height) }

// newReset asks before dropping every answer of the module, since the answer
// file goes with no way back.
func newReset(a *app) *yesNoScreen {
	return newYesNo(labelReset(), labelResetHelp(), func() tea.Cmd { return startOver(a) })
}

// startOver forgets the answers and opens the module where a machine that
// answered nothing opens it, starting points included. The whole stack goes
// too, a trail through answers that no longer exist.
func startOver(a *app) tea.Cmd {
	if err := a.store.Reset(); err != nil {
		logging.Error("%s", err)
		return flashBad(err.Error())
	}
	a.first = true
	return reset(a.upfront())
}
