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

	// checking is whether the answers a list vouches for are being read against
	// that list again, between the row that starts the work and its first page.
	checking bool
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

// working puts the turning mark in the header while the lists are read.
func (h *hub) working() bool { return h.checking }

func (h *hub) Update(msg tea.Msg) (screen, tea.Cmd) {
	if msg, ok := msg.(unofferedMsg); ok {
		return h, h.unoffered(msg.names)
	}
	h.picker.Update(msg)
	key, ok := msg.(tea.KeyMsg)
	if !ok || h.checking {
		return h, nil
	}
	switch {
	case confirms(key):
		switch h.picker.selected() {
		case keyInstall:
			h.checking = true
			check := h.app.runner.Unoffered()
			return h, func() tea.Msg { return unofferedMsg{check()} }
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

// unofferedMsg is the answers the lists no longer offer.
type unofferedMsg struct{ names []string }

// unoffered asks again what a list no longer offers - an answer file from
// another machine names a disk this one does not have - before any password is
// typed, and starts the work once nothing is missing. Asked as a run of
// questions over the hub, so esc on one is a step back and not the way out.
func (h *hub) unoffered(names []string) tea.Cmd {
	h.checking = false
	for _, name := range names {
		h.app.store.Unoffer(name)
	}
	if missing := h.app.store.Missing(); len(missing) > 0 {
		return push(newWizard(h.app).screen(missing[0]))
	}
	return push(startInstall(h.app, 0))
}
