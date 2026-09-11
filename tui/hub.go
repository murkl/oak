package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// hub is where a machine that has answered everything waits: two things to do,
// and no way to get lost between them. Install, or go through the answers
// again.
//
// It has no description of its own and needs none — two rows, each with a
// sentence under it, say the whole of what this page is.
type hub struct {
	app    *app
	picker *picker
}

// The two rows. The NUL prefix cannot collide with anything the folder names.
const (
	keyInstall  = "\x00install"
	keySettings = "\x00settings"
)

func newHub(a *app) *hub {
	h := &hub{app: a}
	h.build()
	return h
}

func (h *hub) Refresh() {
	key := h.picker.selected()
	h.build()
	h.picker.focus(key)
}

// build names the top row after what opening the module does rather than after
// what it is called: the row is pressed, and what a reader wants off a row they
// are about to press is what will happen. The module says both words itself —
// the runtime has no guess of its own to offer.
func (h *hub) build() {
	h.picker = newPicker([]item{
		{title: h.app.module.Does(), detail: h.app.module.Help(), key: keyInstall},
		{title: labelSettings(), detail: labelSettingsHelp(h.app.module.Name()), key: keySettings},
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
