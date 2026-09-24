package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The landing page: the first page of every run, and the one page in the
// program that is never translated.
//
// It is read before a language has been settled — settling one is what it is
// for — so there is no catalog to read it from and no point pretending
// otherwise. Nothing here goes through say(), which is also what keeps it out
// of the template: a string nobody can translate has no business in a
// translator's file.
//
// What that costs is paid back in how it is written: short sentences, plain
// English, and no word that needs another language to be understood.
const (
	landingChoose = "Please choose your language:"
	landingHint   = "↑↓ move · ⏎ confirm · q quit"
)

// landingScreen is that page. It stands on the field rather than in the frame:
// the splash's wordmark stays where it was, the question comes up under it, and
// answering is what opens the frame with everything in it.
//
// The choice itself is the language screen's, rows and keys alike — a second
// way of making it would be a second thing to keep right. What is its own is
// what stands around the rows.
type landingScreen struct {
	choice *languageScreen

	// splash is the one the page stands under, handed over before it starts —
	// see stager. Nil where the product draws no logo, and the page is then the
	// question alone.
	splash *splashModel

	// module is the one named on the way in, or the only one on offer: said
	// under the wordmark, because the page that would otherwise name it is
	// never drawn. In its own words rather than a catalog's, like everything
	// else here, and read when the page is built, so a module chosen after it
	// is not written back over the page that came before.
	module string
}

func newLanding(a *app, done func() tea.Cmd) *landingScreen {
	s := &landingScreen{choice: newLanguagePage(a, nil, done)}
	if a.module != nil {
		s.module = a.module.UI.Title
	}
	return s
}

func (s *landingScreen) stage(splash *splashModel) { s.splash = splash }

// Title is nothing: there is no breadcrumb here to carry it.
func (s *landingScreen) Title() string { return "" }
func (s *landingScreen) Hint() string  { return glyphs.spell.Replace(landingHint) }

func (s *landingScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	_, cmd := s.choice.Update(msg)
	return s, cmd
}

// View is the page centred across the terminal and raised to the golden section
// of its height. While the splash is still on, it is the splash's wordmark and
// sign-off with the rest of the page left blank — laid out on the rows the page
// will take, so the wordmark does not move when the question arrives.
func (s *landingScreen) View(width, height int) string {
	rows := s.page(width, height)
	if s.splash != nil && !s.splash.over() {
		intro := append(s.splash.mark(), make([]string, gapS)...)
		intro = append(intro, s.splash.signature())
		rows = append(intro, make([]string, max(len(rows)-len(intro), 0))...)
	}
	for i, r := range rows {
		rows[i] = lipgloss.PlaceHorizontal(width, lipgloss.Center, r)
	}
	return block(raised(rows, height))
}

// page is the wordmark, the module under it where one is named, and the
// question. On a terminal too small for all of it the wordmark gives way: a
// page that says whose it is but offers no answer asks nothing.
func (s *landingScreen) page(width, height int) []string {
	ask := s.ask(width, height)
	head := s.head(width)
	if len(head)+len(ask) > height || len(head) > 0 && lipgloss.Width(head[0]) > width {
		return ask
	}
	return append(head, ask...)
}

// head is what stands over the question: the wordmark, and under it the module
// where one is named, on the row the splash's sign-off had.
func (s *landingScreen) head(width int) []string {
	var rows []string
	if s.splash != nil {
		rows = s.splash.mark()
	}
	if s.module != "" {
		if len(rows) > 0 {
			rows = append(rows, make([]string, gapS)...)
		}
		rows = append(rows, softStyle.Render(truncate(s.module, width)))
	}
	if len(rows) == 0 {
		return nil
	}
	return append(rows, make([]string, gapM)...)
}

// ask is the question, the languages and the keys that choose one, in no more
// than room rows. On a terminal too short for the lot the keys go first, and
// then the list scrolls in what is left.
func (s *landingScreen) ask(width, room int) []string {
	rows := append(inked(landingChoose, width, textStyle), make([]string, gapS)...)
	keys := append(make([]string, gapM), mutedStyle.Render(truncate(s.Hint(), width)))

	langs := len(s.choice.picker.items)
	if len(rows)+langs+len(keys) > room {
		keys = nil
	}
	rows = append(rows, s.list(max(min(langs, room-len(rows)-len(keys)), 1))...)
	return append(rows, keys...)
}

// list is the rows, squared off to the widest of them so they centre as one
// column rather than each on its own. Only as wide as a name needs: the room a
// row keeps for a value is trimmed off again, since none of them has one.
func (s *landingScreen) list(height int) []string {
	p := s.choice.picker
	w := 0
	for _, it := range p.items {
		w = max(w, lipgloss.Width(it.title))
	}
	w += lipgloss.Width(glyphs.cursor) + gapS + valueGap
	if height < len(p.items) {
		w += scrollbarW
	}
	rows := strings.Split(p.View(w, height), "\n")
	for i, r := range rows {
		rows[i] = strings.TrimRight(r, " ")
	}
	return padLines(rows)
}
