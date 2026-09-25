package tui

import (
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The way in: the pages that stand on the field under the wordmark rather than
// in the frame. The welcome page every run opens on, and after it — where the
// product offers more than one module — the question of which to open. The
// splash's wordmark stays where it was, each question comes up under it, and
// answering the last of them is what opens the frame with everything in it.

// The welcome page is the one page in the program that is never translated.
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

// stand is what such a page draws around its rows. The choice itself is the
// page's own — the language page's rows and keys, or the fork's — and what
// stands around it is the same on both, so the two read as one way in.
type stand struct {
	// splash is the one the page stands under, handed over before it starts —
	// see stager. Nil where the product draws no logo, and the page is then the
	// question alone.
	splash *splashModel

	// caption is said under the wordmark: on the welcome page, the module named
	// on the way in or the only one on offer. Empty everywhere else.
	caption string
}

func (s *stand) stage(splash *splashModel) { s.splash = splash }

// question is what a standing page asks: the sentence, the rows that answer it
// and the keys that choose one.
type question struct {
	text string
	list *picker
	keys string
}

// view is the page centred across the terminal and raised to the golden
// section of its height. While the splash is still on, it is the splash's
// wordmark and sign-off with the rest of the page left blank — laid out on the
// rows the page will take, so the wordmark does not move when the question
// arrives.
func (s *stand) view(width, height int, q question) string {
	rows := s.page(width, height, q)
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

// page is the wordmark, the caption under it where there is one, and the
// question. On a terminal too small for all of it the wordmark gives way: a
// page that says whose it is but offers no answer asks nothing.
func (s *stand) page(width, height int, q question) []string {
	body := q.lines(width, height)
	head := s.head(width)
	if len(head)+len(body) > height || len(head) > 0 && lipgloss.Width(head[0]) > width {
		return body
	}
	return append(head, body...)
}

// head is what stands over the question: the wordmark, and under it the
// caption, on the row the splash's sign-off had.
func (s *stand) head(width int) []string {
	var rows []string
	if s.splash != nil {
		rows = s.splash.mark()
	}
	if s.caption != "" {
		if len(rows) > 0 {
			rows = append(rows, make([]string, gapS)...)
		}
		rows = append(rows, softStyle.Render(truncate(s.caption, width)))
	}
	if len(rows) == 0 {
		return nil
	}
	return append(rows, make([]string, gapM)...)
}

// lines is the question, the rows, the sentence belonging to the one under
// the cursor and the keys that choose one, in no more than room rows. On a
// terminal too short for the lot the keys go first, then the sentence, and
// then the list scrolls in what is left.
func (q question) lines(width, room int) []string {
	rows := append(inked(q.text, width, textStyle), make([]string, gapS)...)
	detail := q.detail(width)
	keys := append(make([]string, gapM), mutedStyle.Render(truncate(q.keys, width)))

	count := len(q.list.items)
	if len(rows)+count+len(detail)+len(keys) > room {
		keys = nil
	}
	if len(rows)+count+len(detail) > room {
		detail = nil
	}
	rows = append(rows, q.column(max(min(count, room-len(rows)-len(detail)-len(keys)), 1))...)
	rows = append(rows, detail...)
	return append(rows, keys...)
}

// column is the rows, squared off to the widest of them so they centre as one
// column rather than each on its own. Only as wide as a name needs: the room a
// row keeps for a value is trimmed off again, since none of them has one.
func (q question) column(height int) []string {
	p := q.list
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

// detail is the sentence belonging to the row under the cursor, on the rows
// a list keeps for one whether or not this row has anything to say: moving the
// cursor must not move the keys under it. Wrapped to the frame's reading width
// rather than the terminal's, so a wide terminal does not stretch it into a
// line nobody reads to the end. Nothing at all where no row has a sentence,
// which is the list of languages.
func (q question) detail(width int) []string {
	if !slices.ContainsFunc(q.list.items, func(it item) bool { return it.detail != "" }) {
		return nil
	}
	lines := append(wrap(q.list.detail(), bodyWidth(min(width, frameW))), make([]string, detailRows)...)
	rows := make([]string, gapS, gapS+detailRows)
	for _, line := range lines[:detailRows] {
		rows = append(rows, softStyle.Render(line))
	}
	return rows
}

// landingScreen is the welcome page: the words the rest of the run is read in.
// The choice itself is the language screen's, rows and keys alike — a second
// way of making it would be a second thing to keep right.
type landingScreen struct {
	stand
	choice *languageScreen
}

// newLanding is that page. A module named on the way in, or the only one on
// offer, is said under the wordmark, because the page that would otherwise
// name it is never drawn. In its own words rather than a catalog's, like
// everything else here, and read when the page is built, so a module chosen
// after it is not written back over the page that came before.
func newLanding(a *app, done func() tea.Cmd) *landingScreen {
	s := &landingScreen{choice: newLanguagePage(a, nil, done)}
	if a.module != nil {
		s.caption = a.module.UI.Title
	}
	return s
}

// Title is nothing: there is no breadcrumb here to carry it.
func (s *landingScreen) Title() string { return "" }
func (s *landingScreen) Hint() string  { return glyphs.spell.Replace(landingHint) }

func (s *landingScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	_, cmd := s.choice.Update(msg)
	return s, cmd
}

func (s *landingScreen) View(width, height int) string {
	return s.view(width, height, question{text: landingChoose, list: s.choice.picker, keys: s.Hint()})
}
