package tui

import tea "github.com/charmbracelet/bubbletea"

// What may answer a question, in one place, since a mouse wheel arrives as
// arrow keys: arrows only move a cursor, enter says yes, esc and backspace go
// back, and q and ctrl+c ask to leave. In front of a box being typed into
// backspace only deletes, and the model takes the two that leave before any
// screen does (see Model.wayOut).

// confirms is the one key that means yes.
func confirms(k tea.KeyMsg) bool { return k.String() == "enter" }

// cancels is the one key that always means back.
func cancels(k tea.KeyMsg) bool { return k.String() == "esc" }

// erases is the delete key, and the second way to say back where there is
// nothing to delete it for. A page that can be either - a list with a text box
// under it - asks which of the two it is; a page that is only a box reaches for
// cancels alone.
func erases(k tea.KeyMsg) bool { return k.String() == "backspace" }

// backs is either way back, for the pages holding no text at all.
func backs(k tea.KeyMsg) bool { return cancels(k) || erases(k) }

// answers is yes or back. Used by the pages that are only there to be read and
// closed, where the two mean the same thing.
func answers(k tea.KeyMsg) bool { return confirms(k) || backs(k) }

// quits asks to leave the program. A letter, so it is only ever read as one
// where nothing is being typed - see takesText.
func quits(k tea.KeyMsg) bool { return k.String() == "q" }

// aborts asks the same thing from everywhere at all: out of a text box, out of
// a run that answers no other key, out of the logo. It is the one way out that
// nothing can be holding.
func aborts(k tea.KeyMsg) bool { return k.String() == "ctrl+c" }

// moves reports the keys that move a list's cursor and nothing else. This is the
// line a page with a text box on it draws: these go to the list, everything else
// is a character being typed - so a query and a cursor share one keyboard
// without either having to know about the other.
func moves(k tea.KeyMsg) bool {
	switch k.String() {
	case "up", "down", "pgup", "pgdown":
		return true
	}
	return false
}

// scrolls reports the keys a wheel produces. Kept as its own list rather than
// as "not enter and not esc": a page that ignores scrolling still has to let
// every other key through to a text field.
func scrolls(k tea.KeyMsg) bool {
	switch k.String() {
	case "up", "down", "left", "right", "pgup", "pgdown":
		return true
	}
	return false
}
