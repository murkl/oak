package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"rsc.io/qr"
)

// A value drawn as a code to scan, for a link wanted on the phone in somebody's
// hand. A code slightly wrong is no code, so colours, margin and whether it is
// drawn at all follow what a scanner needs.

// Two stacked modules to a cell, which is what makes a module square: a
// terminal cell is about twice as tall as it is wide.
const qrRows = 2

// The light margin around the code, in modules: four by the specification, two
// as readers manage. The widest that fits is used, so a short frame still
// carries a scannable code.
const (
	qrQuietWant = 4
	qrQuietMin  = 2
)

// Black on white, said outright rather than taken from the palette, since a
// camera looks for dark squares on light. Inverted or tinted codes read on some
// scanners and not others.
var qrInk = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#000000")).
	Background(lipgloss.Color("#ffffff"))

// qrCode is a value drawn as a code, one string per row, or nil where it will
// not fit or encode. Never cropped or squeezed past its margin, since the page
// shows the value as writing either way.
func qrCode(text string, width, height int) []string {
	if text == "" {
		return nil
	}
	code, err := qr.Encode(text, qr.L)
	if err != nil {
		return nil
	}
	for quiet := qrQuietWant; quiet >= qrQuietMin; quiet-- {
		side := code.Size + 2*quiet
		if side <= width && (side+qrRows-1)/qrRows <= height {
			return draw(code, quiet)
		}
	}
	return nil
}

// draw renders the modules, two rows of them to a line of cells.
func draw(code *qr.Code, quiet int) []string {
	side := code.Size + 2*quiet
	dark := func(x, y int) bool {
		x, y = x-quiet, y-quiet
		if x < 0 || y < 0 || x >= code.Size || y >= code.Size {
			return false
		}
		return code.Black(x, y)
	}
	rows := make([]string, 0, (side+qrRows-1)/qrRows)
	for y := 0; y < side; y += qrRows {
		var b strings.Builder
		for x := range side {
			b.WriteString(cell(dark(x, y), dark(x, y+1)))
		}
		// One style over the whole row rather than one per cell: the light
		// modules are the background, and a run of them has to be painted as
		// surely as the dark ones - otherwise the terminal's own field shows
		// through the very margin the scanner is looking for.
		rows = append(rows, qrInk.Render(b.String()))
	}
	return rows
}
