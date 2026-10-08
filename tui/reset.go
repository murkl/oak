package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/murkl/oak/internal/logging"
)

// resetScreen asks before dropping every answer of the module, since the answer
// file goes with no way back. It opens on No, since an enter meant for the
// setting above must not land here.
type resetScreen struct {
	app    *app
	picker *picker
}

func newReset(a *app) *resetScreen {
	s := &resetScreen{app: a}
	s.picker = newPicker([]item{
		{title: labelYes(), key: keyYes},
		{title: labelNo(), key: keyNo},
	})
	s.picker.describe(labelResetHelp())
	s.picker.focus(keyNo)
	return s
}

func (s *resetScreen) Title() string { return labelReset() }
func (s *resetScreen) Hint() string  { return labelHintChoose() }

func (s *resetScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
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
		return s, s.startOver()
	case backs(key):
		return s, pop()
	}
	return s, nil
}

func (s *resetScreen) View(width, height int) string { return s.picker.View(width, height) }

// startOver forgets the answers and opens the module where a machine that
// answered nothing opens it, starting points included. The whole stack goes
// too, a trail through answers that no longer exist.
func (s *resetScreen) startOver() tea.Cmd {
	if err := s.app.store.Reset(); err != nil {
		logging.Error("%s", err)
		return flashBad(err.Error())
	}
	s.app.first = true
	return reset(s.app.upfront())
}
