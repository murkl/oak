package tui

import (
	"github.com/murkl/oak/internal/i18n"

	tea "github.com/charmbracelet/bubbletea"
)

// languageScreen picks the language the whole interface speaks.
//
// It is deliberately not one of a module's variables: which words the questions
// are asked in has to be settled before any question can be read — before the
// question of which module to open is read — and it belongs to the runtime
// rather than to the thing being installed. It is also the one setting whose
// effect is immediate and total: every word on the next frame is in the new
// language, including the one on the row that was just chosen.
//
// Two pages are built out of it: the row in the settings that changes the
// choice, and the landing page every run opens on, which stands the same list
// under the wordmark rather than in the frame (see landing.go).
type languageScreen struct {
	opening
	app *app

	// What the page says above its rows, where it says anything. Read afresh
	// every time the rows are built rather than kept as text, because choosing
	// a row here is what changes the language it is written in.
	lead func() string

	picker *picker
	done   func() tea.Cmd
}

// newLanguage is the page behind the language row in the settings: the choice
// on its own, under the name it has there.
func newLanguage(a *app, done func() tea.Cmd) *languageScreen {
	return newLanguagePage(a, func() string { return labelLanguageHelp(a.brand()) }, done)
}

func newLanguagePage(a *app, lead func() string, done func() tea.Cmd) *languageScreen {
	s := &languageScreen{app: a, lead: lead, done: done}
	s.build()
	return s
}

func (s *languageScreen) build() {
	items := make([]item, 0, len(s.app.langs))
	for _, l := range s.app.langs {
		items = append(items, item{title: l.Name, key: l.Code})
	}
	s.picker = newPicker(items)
	s.picker.focus(i18n.Current())
	if s.lead != nil {
		s.picker.describe(s.lead())
	}
}

func (s *languageScreen) Title() string { return labelLanguage() }
func (s *languageScreen) Hint() string  { return labelHintChoose() }

func (s *languageScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	s.picker.Update(msg)
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	switch {
	case confirms(key):
		code := s.picker.selected()
		if code == "" {
			return s, nil
		}
		// The rows are rebuilt before leaving, so a page that stays on screen
		// for one more frame is already in the language just chosen. The names
		// themselves do not change — a language is always listed in its own
		// words — but the sentence above them does.
		cmd := s.app.speak(code)
		s.build()
		return s, tea.Batch(cmd, s.done())
	case backs(key):
		return s, pop()
	}
	return s, nil
}

func (s *languageScreen) View(width, height int) string { return s.picker.View(width, height) }
