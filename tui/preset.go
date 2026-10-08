package tui

import (
	"strconv"

	"github.com/murkl/oak/internal/spec"

	tea "github.com/charmbracelet/bubbletea"
)

// presetScreen asks a fresh machine where to start from, the module's rows on
// the runtime's page. A preset only fills in answers that stay ordinary values,
// so one keypress undoes nothing hard.
type presetScreen struct {
	opening
	app     *app
	presets []*spec.Preset
	done    func() tea.Cmd
	picker  *picker
}

func newPreset(a *app, presets []*spec.Preset, done func() tea.Cmd) *presetScreen {
	s := &presetScreen{app: a, presets: presets, done: done}
	items := make([]item, 0, len(presets))
	// Keyed by where the row sits, because that is the whole of a starting
	// point's identity: it is a set of answers, and nothing points at one.
	for i, o := range presets {
		items = append(items, item{title: o.Label(), detail: o.Help(), key: strconv.Itoa(i)})
	}
	s.picker = newPicker(items)
	s.picker.describe(labelPresetsHelp())
	return s
}

func (s *presetScreen) Title() string { return labelPresets() }
func (s *presetScreen) Hint() string  { return labelHintChoose() }

func (s *presetScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	s.picker.Update(msg)
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	switch {
	case confirms(key):
		at, err := strconv.Atoi(s.picker.selected())
		if err != nil || at >= len(s.presets) {
			return s, nil
		}
		return s, s.take(s.presets[at])
	case backs(key):
		return s, pop()
	}
	return s, nil
}

// take is a starting point being chosen: its values become answers, and the
// page after it is the next one.
//
// A row that opens an action is the same idea reached the long way round: a
// starting point somebody was handed rather than picked off this page - a
// configuration shared after another installation - is an action whose script
// fetches the answers, since only the module knows where such a thing is kept.
func (s *presetScreen) take(o *spec.Preset) tea.Cmd {
	if !o.Fetches() {
		return tea.Batch(s.app.adopt(o), s.done())
	}
	return s.app.openFrom(s.app.module.Action(o.Action), func() tea.Cmd {
		return tea.Batch(s.app.fetched(), s.done())
	})
}

func (s *presetScreen) View(width, height int) string {
	return withDetail(s.picker, width, height)
}
