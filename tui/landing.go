package tui

import (
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

// landingText is the greeting and what the rows underneath are for, and nothing
// else. Whoever is reading it came here to start something and is one keypress
// from the page that says what — a paragraph about what happens next is read
// twice by nobody and stands between them and the only thing on this page.
func landingText(product string) string {
	return "Welcome to " + product + ".\n\nPlease choose the language."
}

// newLanding is that page: the greeting, and under it the languages on offer.
//
// It is the language screen with something else standing over its rows rather
// than a page of its own — the choice is the same choice, and a second way of
// making it would be a second thing to keep right.
func newLanding(a *app, done func() tea.Cmd) *languageScreen {
	return newLanguagePage(a,
		func() string { return landingTitle },
		func() string { return landingText(a.brand()) },
		done)
}
