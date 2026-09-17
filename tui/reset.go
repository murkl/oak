package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/murkl/oak/internal/logging"
)

// resetScreen is the last row of the settings page, asked before it acts: every
// answer this module holds is dropped and the module opens again at its first
// question.
//
// A page rather than a row that acts the moment it is pressed, because there is
// no taking it back: the answer file is deleted, and with it a run of questions
// that may have taken somebody ten minutes. It opens on No for the same reason
// — the row above it is an ordinary setting, and an enter meant for that one
// must not land here.
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

// startOver forgets the answers and opens the module where a machine that has
// answered nothing opens it: at the questions it wants settled before anything
// else, and from there through the network, the check and the starting points —
// which are offered again, because being offered once is what a starting point
// is for and this machine has just become one that has never started.
//
// The whole stack goes with it. What led here — the hub, this page — is a trail
// through answers that no longer exist.
func (s *resetScreen) startOver() tea.Cmd {
	if err := s.app.store.Reset(); err != nil {
		logging.Error("%s", err)
		return flashBad(err.Error())
	}
	s.app.first = true
	return reset(s.app.upfront())
}
