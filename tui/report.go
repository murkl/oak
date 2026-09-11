package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// report is the page a run holds still on when a task has something to say: the
// work is done, and here is what came of it.
//
// It exists because a list of task names filling in from the top cannot say
// this. Every row on it looks like every other, so the moment a machine stops
// being a disk being written to and starts being a system somebody owns goes
// past unmarked — and the questions that follow it ("open a shell in there?",
// "restart now?") arrive as though the work were still going on. So the run
// stops, once, and says so at the size the thing deserves.
//
// It is also the page a run that could not go on stops at, under the other mark
// and in the other colour. The two are one page with one thing different about
// them — a run saying where it got to, at the size that deserves — so they are
// one type rather than two that would drift apart.
//
// The words are the module's where the module has any, because what has just
// happened is its own business. What is the runtime's is the mark, the colour,
// and the code — a value nobody is going to copy off a screen by hand.
type report struct {
	headline string
	body     string

	// code is the value drawn to be scanned, and printed underneath as itself.
	// Empty where the module named none, or where what it named came back empty —
	// a link that could not be made is not a page that cannot be shown.
	code string

	// note is one line under the words: what the run has proved about itself by
	// the time this page went up. It is here rather than only at the end of the
	// run because this is the page somebody actually reads — a run that offers a
	// restart is a run most people never see the end of.
	//
	// Empty where nothing has been tested. alarm is whether it is something to
	// look at rather than something to note.
	note  string
	alarm bool

	// stopped is whether this is the page a run ended badly on: the cross
	// instead of the tick, and the fail colour instead of the good one.
	stopped bool
}

// The page divides in the golden ratio, and which way round it divides is
// decided by the code: the words are the major part and the code the minor,
// because the words are what is read and the code is what is done afterwards.
//
// Beside rather than under, which is the one place this page departs from what
// a poster of it would look like. The frame is 89 by 21 — wide, and no taller
// than the terminal it is guaranteed to have — and a code stacked under a
// paragraph wants some 30 rows. Side by side, the same two blocks sit in the
// proportion the rest of the program is built in, and neither is cut.
const (
	// reportWordsMin is the narrowest the words may be squeezed to before the
	// code is dropped instead. Below it a sentence breaks every three words and
	// reads as a column of fragments.
	reportWordsMin = 34

	// reportGap is the channel between the two. Wide enough that the white of
	// the code reads as a thing on the page rather than as the page's edge.
	reportGap = gapL
)

func newReport(headline, body, code string) *report {
	return &report{headline: headline, body: body, code: code}
}

// says adds the line about the tests to this page.
func (r *report) says(note string, alarm bool) *report {
	r.note, r.alarm = note, alarm
	return r
}

// stop turns this page into the one a run that could not go on draws.
func (r *report) stop() *report {
	r.stopped = true
	return r
}

// mark and ink are the two things that differ between the page a run finished
// on and the page it stopped on.
func (r *report) mark() []string {
	if r.stopped {
		return glyphCross
	}
	return glyphTick
}

func (r *report) ink() lipgloss.Style {
	if r.stopped {
		return failStyle.Bold(true)
	}
	return goodStyle
}

func (r *report) Hint() string { return labelHintContinue() }

func (r *report) View(width, height int) string {
	code := qrCode(r.code, width-reportWordsMin-reportGap, height)
	if len(code) == 0 {
		// Nothing beside it, so the margin has to come from somewhere: the same
		// golden one every other page's running text keeps off the frame's edge.
		// A page whose whole content is a headline and a sentence would otherwise
		// set that sentence in a single line the full width of the frame, which is
		// half again the measure the rest of the interface reads at.
		return block(centred(r.words(bodyWidth(width), height), height))
	}
	words := r.words(width-lipgloss.Width(code[0])-reportGap, height)
	return block(beside(words, code, reportGap))
}

// words is everything there is to read, in the order it is read: the mark, what
// happened, what it means, and the value it produced.
//
// Every gap between them is one blank line and no more. The mark is five rows
// tall and carries the whole hierarchy of the page on its own; spacing the rest
// out to match would leave a page of islands.
//
// What a frame too short for all of it loses is what carries least: the tail of
// the paragraph first, and only once there is none of it left, the mark. The
// headline, the line about the tests and the value are what this page is, and
// none of them is ever what goes.
func (r *report) words(width, height int) []string {
	ink := r.ink()
	head := inked(r.headline, width, ink)
	body := inked(r.body, width, textStyle)

	// Rendered row by row rather than through inked: the mark is a picture, and
	// what wraps a paragraph would treat its spacing as words to be closed up.
	picture := r.mark()
	mark := make([]string, 0, len(picture))
	for _, line := range picture {
		mark = append(mark, ink.Render(line))
	}

	build := func() []string {
		out := make([]string, 0, len(mark)+len(head)+len(body)+4)
		if len(mark) > 0 {
			out = append(append(out, mark...), "")
		}
		out = append(out, head...)
		if len(body) > 0 {
			out = append(append(out, ""), body...)
		}
		if r.note != "" {
			ink := mutedStyle
			if r.alarm {
				ink = alertStyle
			}
			out = append(out, "", ink.Render(truncate(r.note, width)))
		}
		if r.code != "" {
			out = append(out, "", infoStyle.Render(truncate(r.code, width)))
		}
		return out
	}

	for {
		if out := build(); len(out) <= height {
			return out
		}
		switch {
		case len(body) > 0:
			body = shortened(body)
		case len(mark) > 0:
			mark = nil
		default:
			return build()
		}
	}
}

// shortened drops the last whole paragraph. A paragraph either fits or it is
// not there: half a sentence with the edge of the frame after it reads as a
// fault rather than as something left out on purpose.
func shortened(lines []string) []string {
	for i := len(lines) - 1; i >= 0; i-- {
		if lines[i] == "" {
			return lines[:i]
		}
	}
	return nil
}

// inked renders one paragraph as rows in a style, at a width already short of
// the frame — the channel beside it is the margin, so there is no second one to
// take off here.
func inked(text string, width int, ink lipgloss.Style) []string {
	lines := wrap(text, width)
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = ink.Render(line)
	}
	return out
}

// beside lays two blocks side by side, each centred against the other, with a
// channel between them. The taller decides the height, so whichever of the two
// it is, the shorter sits in the middle of it rather than hanging off the top.
func beside(left, right []string, gap int) []string {
	rows := max(len(left), len(right))
	leftW := 0
	for _, line := range left {
		leftW = max(leftW, lipgloss.Width(line))
	}
	lead, trail := (rows-len(left))/2, (rows-len(right))/2

	out := make([]string, rows)
	for i := range out {
		line := ""
		if j := i - lead; j >= 0 && j < len(left) {
			line = left[j]
		}
		line += field(strings.Repeat(" ", leftW-lipgloss.Width(line)+gap))
		if j := i - trail; j >= 0 && j < len(right) {
			line += right[j]
		}
		out[i] = strings.TrimRight(line, " ")
	}
	return out
}

// centred sinks a block to the middle of the height it has, for the page with
// nothing beside it to line up against.
func centred(rows []string, height int) []string {
	if len(rows) >= height {
		return rows
	}
	return append(make([]string, (height-len(rows))/2), rows...)
}

func block(rows []string) string { return strings.Join(rows, "\n") }
