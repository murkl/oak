package tui

import (
	"github.com/murkl/oak/internal/spec"

	tea "github.com/charmbracelet/bubbletea"
)

// choiceScreen is which of the runtime's modules this run is.
//
// It comes before everything the module itself does, because all of that
// belongs to whichever it settles — the questions, the answer file, the log.
// An installer that can also repair what it installed is not one program with a
// switch in it: the two ask different questions, do different work and are
// dangerous in different ways, so they are two modules and this is the one
// moment they are told apart.
//
// Which is also why it stands under the wordmark with the welcome page rather
// than in the frame: the frame is titled after the module, and before one has
// been chosen there is nothing yet for it to be about. It is read in the
// language just chosen, since that is settled by now.
//
// What is on offer is each module's own name and its own sentence about
// itself, so the runtime never learns what any of them is for. A runtime
// offering one module, or one named on the command line, never draws this
// page.
type choiceScreen struct {
	stand
	app    *app
	picker *picker
	done   func() tea.Cmd

	// first is whether nothing stands behind this page — no language was
	// asked for — so leaving it is leaving the program rather than a step back.
	first bool
}

func newChoice(a *app, first bool, done func() tea.Cmd) *choiceScreen {
	s := &choiceScreen{app: a, first: first, done: done}
	items := make([]item, 0, len(a.modules))
	for _, mod := range a.modules {
		items = append(items, item{title: mod.Name(), detail: mod.Help(), key: mod.ID()})
	}
	s.picker = newPicker(items)
	return s
}

// Title is nothing: there is no breadcrumb here to carry it.
func (s *choiceScreen) Title() string { return "" }

func (s *choiceScreen) Hint() string {
	if s.first {
		return labelHintMenu()
	}
	return labelHintChoose()
}

func (s *choiceScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	s.picker.Update(msg)
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	if backs(key) {
		return s, pop()
	}
	if !confirms(key) {
		return s, nil
	}
	id, ok := s.picker.chosen()
	if !ok {
		return s, nil
	}
	// A module that will not open is the end of the road rather than a row that
	// does nothing: it was read and checked at startup, so anything failing here
	// is the machine refusing to keep the answers or the log.
	if err := s.app.enter(s.app.byID(id)); err != nil {
		return s, push(newFatal(s.app, err))
	}
	return s, s.done()
}

func (s *choiceScreen) View(width, height int) string {
	return s.view(width, height, question{text: labelChoice(), list: s.picker, keys: s.Hint()})
}

// byID is the module a row on that page stands for.
func (a *app) byID(id string) *spec.Module {
	for _, mod := range a.modules {
		if mod.ID() == id {
			return mod
		}
	}
	return nil
}
