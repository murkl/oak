package tui

import (
	"maps"
	"os"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/lipgloss"
)

// Every glyph the interface draws, in one place, so the set can be checked
// against a font. The reduced set is for a Linux console font of at most 512
// glyphs and keeps to ASCII, box drawing, blocks and codepage 437, each mark
// chosen to read on its own.
type glyphSet struct {
	// cursor and its blank are the same width, so a row never shifts as the
	// selection moves over it.
	cursor string
	crumb  string
	rule   string
	dash   string

	// The scrollbar's track and thumb, one cell per row. A line and a block
	// rather than two weights of the same line: two line-drawing characters a
	// pixel apart in thickness read as one continuous rule with a fault in it,
	// which is worse than no scrollbar at all.
	scrollTrack string
	scrollThumb string

	ok   string
	fail string

	// What a task that asks first is marked with, and what a row that was
	// declined keeps instead of a tick.
	ask  string
	skip string

	// The row a list-editing screen appends after its items, structural rather
	// than one of the items themselves.
	add string

	// The two marks the header's status is shown with: its check said yes, or
	// it said no. Filled and hollow, so the difference is a shape and not only
	// a colour.
	on  string
	off string

	// What a secret looks like while it is being typed. One cell wide in every
	// font, unlike the asterisk, which is drawn high in most of them and makes
	// a password field look like a footnote.
	secret string

	// focus are the density steps a block pixel passes through as the splash's
	// sweep trail cools past it: hollow at the front, filling in to solid - so
	// a fresh letter looks like it is coming into focus rather than switching
	// on at full weight in one step.
	focus []string

	// spinner turns while something runs.
	spinner []string

	// spell rewrites the marks a set has no glyph for but that turn up inside a
	// sentence rather than beside one: the return symbol a key hint names, and
	// the ellipsis a line that is still going ends on. They arrive as words
	// rather than as marks - out of the code, and out of a catalog that
	// translates them - so this is a rewriting of what is said and not one more
	// entry above.
	spell *strings.Replacer
}

// glyphBlank is the cursor's own width in empty cells. The same in either set,
// because a space is a space.
const glyphBlank = "  "

// The four cells every picture is built from: a full block, its halves and
// nothing, so a cell twice as tall as wide carries two square dots. All four
// are in codepage 437 and every lat* font, so the tick and the code look the
// same everywhere.
const (
	blockFull  = "█"
	blockUpper = "▀"
	blockLower = "▄"
	blockNone  = " "
)

// cell is the one of those four that says what a pair of stacked pixels does.
func cell(upper, lower bool) string {
	switch {
	case upper && lower:
		return blockFull
	case upper:
		return blockUpper
	case lower:
		return blockLower
	}
	return blockNone
}

// glyphTick is the mark a finished run is headed by, at the size of something
// worth stopping for. The same tick every finished row carries, drawn out of
// blocks instead of asked of the font - a font's own tick is one cell tall
// whatever it is set beside, and this one has a whole page under it.
var glyphTick = []string{
	"           ▄█▀",
	"         ▄█▀",
	"█▄     ▄█▀",
	" ▀█▄ ▄█▀",
	"   ▀██▀",
}

// glyphCross is the other one: the mark over a run that could not go on, drawn
// out of the same blocks and at the same size, so the two pages are one page
// with one thing different about them.
var glyphCross = []string{
	"▀█▄     ▄█▀",
	"  ▀█▄ ▄█▀",
	"    ███",
	"  ▄█▀ ▀█▄",
	"▄█▀     ▀█▄",
}

// glyphBlockPixel is the one rune the shipped block-letter wordmark draws with -
// every "on" cell of a letter is this and nothing else. The splash checks for it
// by name rather than by literal, so a custom logo built from something else
// entirely is left exactly as it is.
const glyphBlockPixel = '█'

// fullGlyphs is the interface as it is meant to look, on a terminal that can
// draw it. All of these are plain Unicode, present in any monospace font that
// can render a box.
var fullGlyphs = glyphSet{
	cursor:      "▸ ",
	crumb:       "›",
	rule:        "─",
	dash:        "·",
	scrollTrack: "│",
	scrollThumb: "█",
	ok:          "✓",
	fail:        "✕",
	ask:         "?",
	skip:        "·",
	add:         "+",
	on:          "●",
	off:         "○",
	secret:      "•",
	focus:       []string{"░", "▒", "▓", string(glyphBlockPixel)},

	// A filled quadrant sweeping round a circle: it reads as one round thing
	// rotating in place and stays inside a single cell.
	spinner: []string{"◐", "◓", "◑", "◒"},

	// Nothing to rewrite: this is the set every mark was written in.
	spell: strings.NewReplacer(),
}

// plainGlyphs is the same interface on a virtual console. Every entry here is
// in codepage 437 and in the lat* console fonts alike, which between them is
// every font a Linux console is realistically wearing - including the one the
// kernel falls back to when nothing loaded a font at all.
var plainGlyphs = glyphSet{
	cursor: "» ",
	crumb:  ">",
	rule:   "─",
	dash:   "·",

	scrollTrack: "│",
	scrollThumb: "█",

	// No console font has a tick or a cross, so a finished row takes the bullet
	// and a failed one an x. Neither is a block, which on the line would read
	// as a stuck cursor.
	ok:     "•",
	fail:   "x",
	ask:    "?",
	skip:   "·",
	add:    "+",
	secret: "•",
	focus:  []string{"░", "▒", string(glyphBlockPixel)},

	// No console font has a circle, filled or not. The small square codepage
	// 437 has is the nearest thing to a lit lamp, and the middle dot beside it
	// reads as one that is out.
	on:  "■",
	off: "·",

	// One stroke turning on the spot, at the weight of the interface's rules,
	// since a console has no circle and a blinking block reads as a stuck
	// cursor.
	spinner: []string{"│", "/", "─", "\\"},

	// The return symbol is in no console font at all, and the ellipsis is
	// missing from the one the kernel falls back to; both are spelled out.
	spell: strings.NewReplacer("⏎", "enter", "…", "..."),
}

// glyphs is the set showing right now, read at draw time from everywhere the
// interface renders - exactly as the palette is, and for the same reason: which
// terminal this is cannot be known until it has been asked.
var glyphs = fullGlyphs

// adaptGlyphs dresses the interface in the set the terminal can actually draw.
// Separate from the asking so that what the answer does can be checked without
// a terminal to answer.
func adaptGlyphs(plain bool) {
	glyphs = fullGlyphs
	if plain {
		glyphs = plainGlyphs
	}
}

// terminalIsPlain reports whether the terminal draws from a console font: TERM
// `linux`, or none or dumb, which promises nothing. Everything else has a font
// behind it.
func terminalIsPlain() bool {
	switch term := os.Getenv("TERM"); {
	case term == "", term == "dumb":
		return true
	default:
		return strings.HasPrefix(term, "linux")
	}
}

// spinEvery is how fast the working mark turns.
const spinEvery = 100 * time.Millisecond

// spinFrame is the frame every working mark is on now, read off the clock so
// the header's and a page's marks turn in step. A tick still asks for the
// redraw.
func spinFrame() string {
	return glyphs.spinner[int(time.Now().UnixNano()/int64(spinEvery))%len(glyphs.spinner)]
}

// ConsoleGlyphs is every character outside ASCII this interface can put on a
// Linux virtual console, in code point order: the marks of the reduced set, the
// pictures, the frame, and every word handed in as the reduced set spells it.
//
// A product that loads a console font holds that font to this rather than to a
// copy of it - a copy is the one list here that nothing would keep in step.
func ConsoleGlyphs(words ...string) string {
	g := plainGlyphs
	marks := []string{
		g.cursor, g.crumb, g.rule, g.dash, g.scrollTrack, g.scrollThumb,
		g.ok, g.fail, g.ask, g.skip, g.add, g.on, g.off, g.secret,
		blockFull, blockUpper, blockLower, blockNone,
	}
	marks = append(marks, g.focus...)
	marks = append(marks, g.spinner...)
	marks = append(marks, glyphTick...)
	marks = append(marks, glyphCross...)

	border := lipgloss.NormalBorder()
	marks = append(marks, border.Top, border.Bottom, border.Left, border.Right,
		border.TopLeft, border.TopRight, border.BottomLeft, border.BottomRight)

	// The landing page's own words are in no catalog, so nobody hands them in.
	for _, word := range append([]string{landingChoose, landingHint}, words...) {
		marks = append(marks, g.spell.Replace(word))
	}

	seen := map[rune]bool{}
	for _, mark := range marks {
		for _, r := range mark {
			if r > unicode.MaxASCII {
				seen[r] = true
			}
		}
	}
	out := slices.Sorted(maps.Keys(seen))
	return string(out)
}
