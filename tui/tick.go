package tui

import (
	"time"

	"github.com/charmbracelet/bubbles/cursor"

	tea "github.com/charmbracelet/bubbletea"
)

// after is every clock in the interface: the message a page asks for once a
// stretch of time has gone by. They all go through one name so a test can run
// the interface on no time at all — that a page acts when its clock goes off is
// behaviour and is tested, how long it waited first is not, and a suite that
// waits those out spends its run waiting.
var after = tea.Tick

// cursorMode is how a text cursor behaves. It is the other clock in the
// interface and the only one bubbles owns rather than this package, so it is
// switched here rather than through after — and it is switched at the one place
// every text input in the program is made, styleInput.
var cursorMode = cursor.CursorBlink

// pollEvery is how often a page with something running asks to be redrawn.
// Fast enough for the mark to look alive, slow enough that a long install does
// not spend its time painting frames.
const pollEvery = 100 * time.Millisecond

// tickMsg is one of those redraws. A page that has work in flight asks for the
// next one for as long as it has any, and stops asking the moment it does not —
// so an idle interface costs nothing at all.
type tickMsg struct{}

func tick() tea.Cmd {
	return after(pollEvery, func(time.Time) tea.Msg { return tickMsg{} })
}
