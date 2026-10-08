package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// hub is where a machine that has answered everything waits: the work and the
// answers, and no way to get lost between them. What a module offers beside
// them is a row on the settings page — see settingsScreen.
//
// Over the two rows stands the module's icon and nothing else. The page only
// comes up once every check the work waits for has said yes and every question
// has an answer, so there is nothing left to say on it.
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

// build names both rows after what pressing them does, in the same words in
// every module, and not after the module they belong to: the frame overhead
// carries that name on every page, and a row repeating it would be the same
// word twice on one screen.
func (h *hub) build() {
	h.picker = newPicker([]item{
		{title: rowStart, key: keyInstall},
		{title: rowSetup, key: keySettings},
	})
}

func (h *hub) Title() string { return labelMenu() }
func (h *hub) Hint() string  { return labelHintMenu() }

// crumbRoot: the hub is home. Whatever run of pages ended on it is over, and
// none of it is behind this page any more.
func (h *hub) crumbRoot() bool { return true }

// home: a trail on this page would name nothing but the page itself.
func (h *hub) home() bool { return true }

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

// View is the icon over the rows, a blank line between them, the two centred
// as one block in the frame. A frame too small for the icon keeps the rows,
// since they are what the page is for.
func (h *hub) View(width, height int) string {
	rows := h.picker.column(min(len(h.picker.items), height))
	if picture := h.picture(); len(picture)+gapS+len(rows) <= height && lipgloss.Width(picture[0]) <= width {
		rows = append(append(picture, make([]string, gapS)...), rows...)
	}
	for i, r := range rows {
		rows[i] = lipgloss.PlaceHorizontal(width, lipgloss.Center, r)
	}
	// Centred between the frame's rules, so the blank line the frame keeps under
	// the top one counts as a row above. A row left over goes under the block.
	lead := max((height+1-len(rows))/2-1, 0)
	return block(append(make([]string, lead), rows...))
}

// picture is the module's icon, the runtime's, or the tick, squared off so it
// centres as one block. Five rows stand to the blank line and the two rows
// under them in the golden ratio, which is the size the tick is drawn at.
func (h *hub) picture() []string {
	picture, ink := padLines(glyphTick), goodStyle
	if icon := h.app.icon(); icon != "" {
		picture, ink = logoLines(icon), accentStyle
	}
	out := make([]string, len(picture))
	for i, line := range picture {
		out[i] = ink.Render(line)
	}
	return out
}

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
