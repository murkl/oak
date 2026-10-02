package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// hub is where a machine that has answered everything waits: the work and the
// answers, and no way to get lost between them. What a module offers beside
// them is a row on the settings page — see settingsScreen.
//
// It has no description of its own and needs none — each row has a sentence
// under it, and together they say the whole of what this page is.
type hub struct {
	app    *app
	picker *picker
}

// The rows. The NUL prefix cannot collide with anything the folder names.
const (
	keyInstall  = "\x00install"
	keySettings = "\x00settings"
)

func newHub(a *app) *hub {
	h := &hub{app: a}
	h.build()
	return h
}

// Init asks again which actions this machine has, every time the page comes
// up: a card is a thing that gets plugged in, and the pages this one leads to
// — the way out, the end of a run — offer what the last look found.
func (h *hub) Init() tea.Cmd { return h.app.lookFor() }

func (h *hub) Refresh() {
	key := h.picker.selected()
	h.build()
	h.picker.focus(key)
}

// build names both rows after what pressing them does — in the module's own
// words for it where it has them — and not after the module they belong to:
// the frame overhead carries that name on every page, and a row repeating it
// would be the same word twice on one screen. What the module has to say for
// itself is the sentence under the first.
func (h *hub) build() {
	h.picker = newPicker([]item{
		{title: h.app.verb(), detail: h.app.module.Help(), key: keyInstall},
		{title: h.app.settingsTitle(), detail: labelSettingsSummary(), key: keySettings},
	})
}

func (h *hub) Title() string { return "" }
func (h *hub) Hint() string  { return labelHintMenu() }

// crumbRoot: the hub is home. Whatever run of pages ended on it is over, and
// none of it is behind this page any more.
func (h *hub) crumbRoot() bool { return true }

func (h *hub) Update(msg tea.Msg) (screen, tea.Cmd) {
	h.picker.Update(msg)
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return h, nil
	}
	switch {
	case confirms(key):
		switch h.picker.selected() {
		case keyInstall:
			return h, push(newConfirm(h.app))
		case keySettings:
			return h, push(newSettings(h.app))
		}
	case backs(key):
		// Nothing is behind the hub: the run of questions that led here is
		// spent, and the model turns backing off the last page there is into
		// the question of how to leave.
		return h, pop()
	}
	return h, nil
}

func (h *hub) View(width, height int) string { return withDetail(h.picker, width, height) }
