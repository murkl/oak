package tui

import (
	"github.com/murkl/oak/internal/spec"

	tea "github.com/charmbracelet/bubbletea"
)

// choiceScreen is which module this run is, asked before anything of a module
// since everything after belongs to it, under the wordmark since the frame is
// titled after the module. It offers each module's own name, and is never drawn
// for one module or one named outright.
type choiceScreen struct {
	stand
	app    *app
	picker *picker
	done   func() tea.Cmd

	// first is whether nothing stands behind this page - no language was
	// asked for - so leaving it is leaving the program rather than a step back.
	first bool
}

func newChoice(a *app, first bool, done func() tea.Cmd) *choiceScreen {
	s := &choiceScreen{app: a, first: first, done: done}
	items := make([]item, 0, len(a.modules))
	for _, mod := range a.modules {
		items = append(items, item{title: mod.Name(), key: mod.ID()})
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
