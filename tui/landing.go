package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
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
const landingTitle = "Welcome"

// Three lines, and they are three because each says a different thing: what
// this is, where the rest of it lives, and what the rows underneath are for.
// Whoever is reading them came here to start something and is one keypress from
// the page that says what — a paragraph about what happens next is read twice
// by nobody and stands between them and the only thing on this page.
const (
	landingGreeting = "Welcome to %s."
	landingLink     = "More about this project:"
	landingChoose   = "Please choose the language."
)

// landingWords is that greeting, the product's address where it declared one,
// and the line that leads the rows — each in the ink that says what it is. The
// greeting carries the weight, the address the colour a value is written in
// everywhere else in the program, and what is left is body text.
func landingWords(product, link string, width int) []string {
	out := inked(fmt.Sprintf(landingGreeting, product), width, boldStyle)
	if link != "" {
		out = append(out, "")
		out = append(out, inked(landingLink, width, softStyle)...)
		out = append(out, infoStyle.Render(truncate(link, width)))
	}
	return append(append(out, ""), inked(landingChoose, width, textStyle)...)
}

// newLanding is that page: the greeting, the address, and under them the
// languages on offer.
//
// It is the language screen with something else standing over its rows rather
// than a page of its own — the choice is the same choice, and a second way of
// making it would be a second thing to keep right.
func newLanding(a *app, done func() tea.Cmd) *languageScreen {
	s := newLanguagePage(a, func() string { return landingTitle }, nil, done)
	s.words = func(width int) []string { return landingWords(a.brand(), a.link(), width) }
	return s
}
