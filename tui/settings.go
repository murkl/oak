package tui

import (
	"github.com/murkl/oak/internal/i18n"
	"github.com/murkl/oak/internal/spec"
	"github.com/murkl/oak/internal/store"

	tea "github.com/charmbracelet/bubbletea"
)

// settingsScreen is every answer this installer holds, on one page, each beside
// what it is currently set to.
//
// Not a hand-written page: it is the folder's own list of variables, in the
// order the module declared them, grouped by the headings it named. A variable
// added to the declaration is on this page the moment it exists, and there is
// no second list anywhere that could fall out of step with the first.
//
// A row shows a name and a value and nothing else. What the value is *for* is a
// sentence, and a sentence belongs on the page that asks the question — which
// is one keypress away, and is the same page the opening run of questions used.
//
// It narrows like any other long list: press / and type. A module of any size
// puts more answers on this page than fit in a frame, and hunting for one by
// scrolling is the thing the box exists to spare.
type settingsScreen struct {
	app *app

	// The rows as the answers stand, flat, and the box that narrows them. Kept
	// apart from the picker because what a query hides has to still be there to
	// come back, and because the headings are laid out around whatever survived
	// rather than fixed to the rows.
	rows   []settingRow
	filter *filter

	picker *picker
}

// The groups the runtime's own rows stand in, so that the layout puts a blank
// line where one changes. The NUL prefix cannot collide with a group the folder
// named.
//
// Only the second carries a heading. The language row is the page's own first
// line and reads as what it is; the validation row is about the work rather
// than about a value, and a word over it is what says so.
const (
	groupLanguage = "\x00language"
	groupValidate = "\x00validate"
)

// settingRow is one setting beside the heading it sits under. Both the module's
// own key and the words it reads as: the key is what marks the end of a group,
// and the words are what a query is matched against.
type settingRow struct {
	item
	group string
	label string
}

func newSettings(a *app) *settingsScreen {
	s := &settingsScreen{app: a, filter: newFilter(false)}
	s.build()
	return s
}

// Refresh rebuilds on the way back from a value that was just changed: the row
// shows the new value, and a group whose condition that answer just flipped
// appears or goes. Whatever was typed into the box stays typed — the query is
// how this row was found, and changing it is no reason to go looking again.
func (s *settingsScreen) Refresh() { s.build() }

// build reads the answers afresh, then lays the page out for the query now in
// the box.
func (s *settingsScreen) build() {
	s.rows = s.collect()
	s.layout()
}

// collect is every setting this page shows, in the order it shows them.
func (s *settingsScreen) collect() []settingRow {
	rows := []settingRow{}
	// The runtime's own two rows stand above the folder's, and each in a group
	// of its own: they have nothing in common but who they belong to — one is
	// about the words on screen, the other about what a run does with itself —
	// and read as a pair the second would be taken for more of the first.
	//
	// Language leads. It changes this page itself, so it is found without having
	// to read anything — unless the module tied it to one of its own answers,
	// see `language:`, in which case that answer is the row a few lines further
	// down and a second one above it would only be the same setting able to
	// disagree with itself.
	if s.app.module.Language == "" && len(s.app.langs) > 1 {
		rows = append(rows, settingRow{item: item{
			title: labelLanguage(),
			value: s.languageName(),
			key:   store.LangVar,
		}, group: groupLanguage})
	}
	// And it is offered only where this module has something to check: a switch
	// for a thing that would never happen is a row that reads as a promise
	// nothing keeps.
	if s.app.module.Checks() {
		rows = append(rows, settingRow{item: item{
			title: labelValidatingScripts(),
			value: store.Label(truth(s.app.prefs.Validates())),
			key:   store.ValidateVar,
		}, group: groupValidate, label: labelValidating()})
	}
	for _, v := range s.app.store.Visible() {
		rows = append(rows, settingRow{
			item: item{
				title: v.Label(),
				value: s.app.store.Display(v),
				key:   v.Name,
			},
			group: v.Group,
			label: v.GroupLabel(),
		})
	}
	return rows
}

// list is the page as it stands: the rows the box has left of them, with the
// headings put back around those. A heading marks where the group changes, in
// either direction: into one, so the rows under it read as a set, and back out
// of one, so the rows that follow do not read as still being in it.
func (s *settingsScreen) list() []item {
	items := []item{}
	group := ""
	for _, r := range s.rows {
		if !s.keeps(r) {
			continue
		}
		if r.group != group {
			group = r.group
			if len(items) > 0 {
				items = append(items, heading(""))
			}
			if r.label != "" {
				items = append(items, heading(r.label))
			}
		}
		items = append(items, r.item)
	}
	if len(items) == 0 {
		items = append(items, item{title: labelNoMatch(), disabled: true})
	}
	return items
}

// keeps reports whether a row survives the query. Its heading counts as well as
// its name: the heading is on screen above it and is half of how the row reads,
// so somebody who remembers a setting as "one of the storage ones" is looking
// at something the page actually says.
func (s *settingsScreen) keeps(r settingRow) bool {
	q := s.filter.query()
	return matches(r.title, q) || matches(r.label, q)
}

// layout rebuilds the picker, leaving the cursor on the row it was on wherever
// that row survived the query.
func (s *settingsScreen) layout() {
	key := ""
	if s.picker != nil {
		key = s.picker.selected()
	}
	s.picker = newPicker(s.list())
	s.picker.describe(labelSettingsHelp(s.app.module.Name()))
	s.picker.focus(key)
}

// languageName is what the current language calls itself, which is how it is
// listed and so how it should read here.
func (s *settingsScreen) languageName() string {
	code := i18n.Current()
	for _, l := range s.app.langs {
		if l.Code == code {
			return l.Name
		}
	}
	return code
}

// takesText: the narrowing box, while it is open. A letter is a character
// being typed into it rather than a key of this page's.
func (s *settingsScreen) takesText() bool { return s.filter.active() }

func (s *settingsScreen) Title() string { return labelSettings() }
func (s *settingsScreen) Hint() string  { return filterHint(labelHintList(), s.filter) }

func (s *settingsScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	// The box gets the key before the page does: / opens it, typing narrows,
	// esc closes it — which is why esc only leaves the page once there is no
	// box left to close.
	if took, cmd := s.filter.Update(key); took {
		s.layout()
		return s, cmd
	}
	s.picker.Update(msg)
	switch {
	case backs(key):
		return s, s.leave()
	case confirms(key):
		return s, s.open(s.picker.selected())
	}
	return s, nil
}

// leave is what backing out of the page does — ordinarily straight to the hub,
// but through whatever answer is now missing first. Turning a setting on can
// call for values nothing has asked for yet — dual boot names two partitions
// only once it is on — and letting the hub come up before those are answered
// would leave an install one enter key away from running without them.
func (s *settingsScreen) leave() tea.Cmd {
	if missing := s.app.store.Missing(); len(missing) > 0 {
		return push(newWizard(s.app).screen(missing[0]))
	}
	return pop()
}

// open is the page behind a row. A secret has none: it is not stored, so there
// is nothing here to change — it is asked for on the way into an installation
// and forgotten again afterwards.
func (s *settingsScreen) open(name string) tea.Cmd {
	switch {
	case name == "":
		return nil
	case name == store.LangVar:
		return push(newLanguage(s.app, pop))
	case name == store.ValidateVar:
		return push(newSwitch(s.app, labelValidatingScripts(), labelValidatingHelp(), s.app.prefs.Validates(), s.app.validate))
	}
	v := s.app.module.Var(name)
	if v == nil || v.Secret() {
		return nil
	}
	return push(newField(s.app, v, pop))
}

func (s *settingsScreen) View(width, height int) string {
	return s.filter.View() + s.picker.View(width, height-s.filter.rows())
}

// truth is a bool as the answer file spells one, so that the runtime's own rows
// read exactly as a module's do.
func truth(on bool) string {
	if on {
		return spec.BoolTrue
	}
	return spec.BoolFalse
}

// switchScreen is a yes or no that belongs to the runtime rather than to a
// module. A module's own bool has a page already — the one every question is
// asked on — and it is built out of a declaration this answer does not have, so
// the two rows get a page of their own rather than a fake variable to hang off.
type switchScreen struct {
	app    *app
	title  string
	help   string
	set    func(bool) tea.Cmd
	picker *picker
}

func newSwitch(a *app, title, help string, on bool, set func(bool) tea.Cmd) *switchScreen {
	s := &switchScreen{app: a, title: title, help: help, set: set}
	s.picker = newPicker([]item{
		{title: labelYes(), key: keyYes},
		{title: labelNo(), key: keyNo},
	})
	s.picker.describe(help)
	if !on {
		s.picker.focus(keyNo)
	}
	return s
}

func (s *switchScreen) Title() string { return s.title }
func (s *switchScreen) Hint() string  { return labelHintChoose() }

func (s *switchScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	s.picker.Update(msg)
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	switch {
	case confirms(key):
		return s, tea.Batch(s.set(s.picker.selected() == keyYes), pop())
	case backs(key):
		return s, pop()
	}
	return s, nil
}

func (s *switchScreen) View(width, height int) string { return s.picker.View(width, height) }
