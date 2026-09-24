package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// hub is where a machine that has answered everything waits: two things to do,
// and no way to get lost between them. Install, or go through the answers
// again — and, for a module that can join a wireless network but can do
// without one, a third: joining one, whenever somebody wants it.
//
// It has no description of its own and needs none — a row or three, each with
// a sentence under it, say the whole of what this page is.
type hub struct {
	app    *app
	picker *picker
}

// The rows. The NUL prefix cannot collide with anything the folder names.
const (
	keyInstall  = "\x00install"
	keySettings = "\x00settings"
	keyNetwork  = "\x00network"
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

// build names the top row after what pressing it does and not after the module
// it belongs to: the frame overhead carries that name on every page, and a row
// repeating it would be the same word twice on one screen. What the module has
// to say for itself is the sentence under the row.
//
// The network is offered where the module can join one and does not ask for
// the internet on the way in: where it does, the opening has already put that
// page in front of it.
func (h *hub) build() {
	items := []item{
		{title: labelOpening(), detail: h.app.module.Help(), key: keyInstall},
		{title: labelSettings(), detail: labelSettingsSummary(), key: keySettings},
	}
	if r := h.app.runner.Radio(); r != nil && r.Joinable() && !r.Checks() {
		items = append(items, item{title: labelNetwork(), detail: labelNetworkJoin(), key: keyNetwork})
	}
	h.picker = newPicker(items)
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
		case keyNetwork:
			return h, push(newJoin(h.app, h.app.runner.Radio()))
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
