package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// filter is the narrowing box a long list puts in front of itself: press / and
// type, the same box as the search page. Closed it draws nothing, and every
// list that can have one has one, so the key works the same everywhere.
type filter struct {
	input textinput.Model
	open  bool

	// permanent is a box that is simply there rather than waiting for the key:
	// up from the first frame and never closed. It costs the page every letter,
	// q and backspace included, so nothing here decides it: the question says
	// so, or it is one asked before the keyboard layout has been settled.
	permanent bool
}

// filterKey opens the box. Slash rather than a letter, because a list already
// spends its letters on moving through it - h, j, k, l, g, G - and because it is
// what a terminal has meant by "narrow this" since long before this program.
const filterKey = "/"

// newFilter builds the box closed, or open and focused where permanent. The
// list's length plays no part, so the page looks the same on every machine.
func newFilter(permanent bool) *filter {
	f := &filter{}
	f.input = textinput.New()
	f.input.Placeholder = labelFilterPlaceholder()
	f.input.CharLimit = 64
	styleInput(&f.input)
	if permanent {
		f.permanent = true
		f.open = true
		f.input.Focus()
	}
	return f
}

// query is what has been typed so far.
func (f *filter) query() string { return f.input.Value() }

// active reports whether the box is open and taking what is typed. Asked by
// the page holding it, which has to say whether a letter is a character or a
// key of its own - see takesText.
func (f *filter) active() bool { return f != nil && f.open }

// Update offers a key to the box and reports whether it took it: arrows and
// enter stay the list's, every other key is typed. Esc closes the box, and only
// a closed box lets esc mean back.
func (f *filter) Update(key tea.KeyMsg) (took bool, cmd tea.Cmd) {
	if !f.open {
		if key.String() != filterKey {
			return false, nil
		}
		f.open = true
		f.input.Focus()
		return true, textinput.Blink
	}
	switch {
	// The permanent box has nothing to close, so what would close it instead
	// leaves the question the same way it would have if the box had never been
	// there: esc is handed on rather than acted on here.
	case f.permanent && cancels(key):
		return false, nil
	// Esc closes it. Backspace does not: while this box is open it is the
	// delete key, so holding it down to clear a query cannot close the box and
	// then leave the page behind it.
	case cancels(key):
		f.close()
		return true, nil
	case confirms(key), moves(key):
		return false, nil
	}
	f.input, cmd = f.input.Update(key)
	return true, cmd
}

// close puts the box away and clears it: opening it again asks a new question,
// not the last one half-answered.
func (f *filter) close() {
	f.open = false
	f.input.Blur()
	f.input.SetValue("")
}

// View is the box as it sits over a list: the same cursor-and-field the search
// page draws, and the blank line that holds it off the rows. Empty while closed.
func (f *filter) View() string {
	if !f.open {
		return ""
	}
	return cursorStyle.Render(glyphs.cursor) + f.input.View() + "\n\n"
}

// rows is what the box costs the list below it, read off what it actually draws
// so the two can never drift apart.
func (f *filter) rows() int { return strings.Count(f.View(), "\n") }

// matches is the program's one definition of matching: a case-folding substring
// of the text as it reads on screen. An empty query matches everything.
func matches(text, query string) bool {
	return strings.Contains(strings.ToLower(text), strings.ToLower(strings.TrimSpace(query)))
}

// narrow keeps the rows a query matches, on the title - which is all a row of
// answers has. A page whose rows read as more than that - a name under a
// heading - asks matches about each part itself.
func narrow(items []item, query string) []item {
	out := make([]item, 0, len(items))
	for _, it := range items {
		if matches(it.title, query) {
			out = append(out, it)
		}
	}
	return out
}

// filterHint folds the box's keys into a page's own help: what the box does
// while up, and the key that opens it while not. Nil is a page whose list has
// not resolved yet.
func filterHint(base string, f *filter) string {
	switch {
	case f == nil:
		return base
	case f.permanent:
		return labelHintFilterPermanent()
	case f.open:
		return labelHintFilter()
	}
	return base + " · " + labelHintFilterKey()
}
