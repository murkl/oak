package tui

import (
	"strings"

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
// Two pages are built out of it and they differ in what stands over the rows:
// the landing page every run opens on (see landing.go), and the row in the
// settings that changes the choice afterwards.
type languageScreen struct {
	opening
	app *app

	// What the page is called and what it says above its rows. Read afresh
	// every frame rather than kept as text, because choosing a row here is what
	// changes the language they are written in.
	//
	// The two pages say it in two ways. The settings row hands the list one
	// sentence, which scrolls with the rows the way every other description in
	// the program does; the landing page draws a block of its own instead —
	// several lines, in several inks, standing still — so `lead` and `words`
	// are one field each and exactly one of them is ever set.
	title func() string
	lead  func() string
	words func(width int) []string

	// link is the product's address, drawn beside the rows as a code to scan.
	// Empty on every page but the landing, and on a product that declared none.
	link string

	picker *picker
	done   func() tea.Cmd
}

// newLanguage is the page behind the language row in the settings: the choice
// on its own, under the name it has there.
func newLanguage(a *app, done func() tea.Cmd) *languageScreen {
	return newLanguagePage(a, labelLanguage, func() string { return labelLanguageHelp(a.brand()) }, done)
}

func newLanguagePage(a *app, title, lead func() string, done func() tea.Cmd) *languageScreen {
	s := &languageScreen{app: a, title: title, lead: lead, done: done}
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

func (s *languageScreen) Title() string { return s.title() }
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

// landingGap is the channel between the words and the code, wide enough that
// the white of the code reads as a thing on the page rather than as its edge —
// the same one the report page holds, for the same reason.
const landingGap = gapL

// View is the list, or — on the landing page — the page that list is part of:
// the words, the rows under them, and the code beside both.
//
// The page divides in the golden ratio twice over. Across: the words and the
// rows take the major part, the code the minor — that way round because the
// words are what is read and the rows are what is pressed, while the code is
// for a second machine entirely. It is drawn in a column of its own rather than
// against the longest line beside it, so it stands in the same place whatever
// the product is called and however many languages it was translated into.
//
// Down: the rows may claim up to the major part of the height, and whatever
// they do not need is the words'. On the frame this page is drawn in that costs
// nothing — two languages leave the words all the room they want — and it is
// what decides who gives way on a terminal too short for both, or in a product
// translated into twenty languages. The words do: they are what stands over the
// page, and a greeting with no answer under it asks a question it does not
// offer a way to answer.
func (s *languageScreen) View(width, height int) string {
	if s.words == nil {
		return s.picker.View(width, height)
	}

	major, minor := split(width)
	code := qrCode(s.link, minor, height)
	if len(code) == 0 {
		// Nothing beside the words, so there is no column to keep clear for it:
		// the page is the words and the rows, at the width the frame gives them.
		major = width
	}

	rowsWant, _ := split(height)
	room := height - min(len(s.picker.items), rowsWant) - 1
	words := s.words(major)
	// A paragraph at a time, from the end: the greeting is what is left last,
	// and half of one read as the frame's edge would be worse than neither.
	for len(words) > room {
		words = shortened(words)
	}

	rows := words
	if len(rows) > 0 {
		rows = append(rows, "")
	}
	rows = append(rows, strings.Split(s.picker.View(major, max(height-len(rows), 1)), "\n")...)
	if len(code) == 0 {
		return block(rows)
	}
	return block(beside(rows, major, code, landingGap))
}
