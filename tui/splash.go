package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The splash is a moment before the interface, without frame, trail or keys, in
// the same program so the shell never blinks through. The wordmark sweeps in
// and the version stands under it, and where the welcome page follows it keeps
// the wordmark (see landing.go).

const (
	// animEvery is one animation frame, the rate the wordmark sweeps in.
	animEvery = time.Second / 24

	// splashFor is how long the splash stays. 1.618 seconds, because everything
	// else here is proportioned that way too and it is long enough to read a
	// wordmark.
	splashFor = 1618 * time.Millisecond

	// sweepFor is how long the wordmark takes to come in - the splash divided
	// by φ³, long enough to read as a light passing through it and short enough
	// that nobody waits through it.
	sweepFor = 382 * time.Millisecond

	// trailCols is how many columns behind the sweep's front a letter still
	// carries a trace of it - cooling from the accent back to its row's own
	// resting colour, and, for a block pixel, filling in from hollow to solid -
	// long enough to read as a light passing through the word and coming
	// into focus behind it, short enough that most of a settled wordmark is
	// already at rest.
	trailCols = gapXXL
)

type animMsg struct{}

func animTick() tea.Cmd {
	return after(animEvery, func(time.Time) tea.Msg { return animMsg{} })
}

// logoLines squares off a block of text. Trailing spaces do not survive every
// editor, and a ragged block would be centred line by line - which would pull
// the wordmark apart.
func logoLines(logo string) []string {
	return padLines(strings.Split(strings.TrimRight(logo, "\n"), "\n"))
}

// padLines squares a block off to its own widest line, right-padding every
// other one out to match.
func padLines(lines []string) []string {
	w := 0
	for _, l := range lines {
		if n := lipgloss.Width(l); n > w {
			w = n
		}
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = l + strings.Repeat(" ", w-lipgloss.Width(l))
	}
	return out
}

type splashModel struct {
	// rows is the logo, squared off and split in two: the eyebrow - the OS
	// name, dim, standing over the wordmark - and the headline, the one line
	// this program actually is, lit in the accent. Everything from the first
	// blank line on is the headline; a logo with none is headline only.
	rows      []string
	titleRows int

	// version is the product's, as its oak.yaml names it, which stands under
	// the wordmark. Empty where the product names none.
	version string
	elapsed time.Duration
}

func newSplash(logo, version string) *splashModel {
	rows := logoLines(logo)
	title := 0
	for title < len(rows) && strings.TrimSpace(rows[title]) != "" {
		title++
	}
	return &splashModel{rows: rows, titleRows: title, version: version}
}

// advance moves the splash on one frame and reports whether it is over.
func (m *splashModel) advance() (done bool) {
	m.elapsed += animEvery
	return m.over()
}

func (m *splashModel) over() bool { return m.elapsed >= splashFor }

// skip cuts the logo short: nobody should have to sit through a wordmark twice.
func (m *splashModel) skip() { m.elapsed = splashFor }

// revealed is how far the wordmark has swept into view, left to right: 0 at
// the very start, 1 once the build is done, and held there for the rest of
// the splash - the letters do not sweep in twice.
func (m *splashModel) revealed() float64 {
	if m.elapsed >= sweepFor {
		return 1
	}
	return float64(m.elapsed) / float64(sweepFor)
}

func (m *splashModel) View(width, height int) string {
	return placeOnField(width, height, m.sign(strings.Join(m.mark(), "\n")))
}

// mark draws the wordmark, row by row, as far as it has swept into view.
func (m *splashModel) mark() []string {
	revealed := m.revealed()
	front := revealed * float64(lipgloss.Width(m.rows[0]))

	rows := make([]string, len(m.rows))
	for dy, line := range m.rows {
		resting, rest := colors.accent, accentBold
		if dy < m.titleRows {
			resting, rest = colors.soft, softStyle
		}
		rows[dy] = sweepLine(line, front, revealed >= 1, resting, rest)
	}
	return rows
}

// sweepLine renders one row up to front: a letter in the trail cools from the
// accent to resting and fills in, anything beyond front is unlit, and settled
// pins the trail at 1 once the sweep is done. An arrived letter is drawn in
// rest, so it takes the same colour slot on sixteen colours.
func sweepLine(line string, front float64, settled bool, resting lipgloss.Color, rest lipgloss.Style) string {
	trail := rest.UnsetForeground()
	var b strings.Builder
	for dx, r := range []rune(line) {
		if r == ' ' || float64(dx) >= front {
			b.WriteString(field(" "))
			continue
		}
		t := 1.0
		if !settled {
			t = trailProgress(dx, front)
		}
		glyph := sweepGlyph(r, t)
		if t >= 1 {
			b.WriteString(rest.Render(glyph))
			continue
		}
		b.WriteString(trail.Foreground(sweepColor(t, resting)).Render(glyph))
	}
	return b.String()
}

// trailProgress is how far a column at dx sits behind the sweep's front, as a
// fraction of trailCols: 0 right at the front, 1 a full trailCols behind it
// or further. Colour and density both move on this one scale, so a letter's
// shading and its sharpness resolve together rather than on two clocks.
func trailProgress(dx int, front float64) float64 {
	switch t := (front - float64(dx)) / trailCols; {
	case t < 0:
		return 0
	case t > 1:
		return 1
	default:
		return t
	}
}

// sweepColor is the foreground a letter takes at a given point in the trail:
// full accent at the front (t=0), cooling to resting by the time the trail
// runs out (t=1).
func sweepColor(t float64, resting lipgloss.Color) lipgloss.Color {
	return blend(colors.accent, resting, t)
}

// sweepGlyph is what a rune renders as at a given point in the trail: a solid
// block pixel comes into focus through glyphs.focus' density steps as t goes
// from the front to fully resolved. Anything else - a runtime's own logo may
// draw with letters or icons rather than block pixels - renders as itself
// throughout; only the shipped block font resolves like this.
func sweepGlyph(r rune, t float64) string {
	if r != glyphBlockPixel {
		return string(r)
	}
	return glyphs.focus[int(t*float64(len(glyphs.focus)-1)+0.5)]
}

// sign puts the version under a block, centred, with gapS blank lines of its
// own - the same single line the logo itself uses to part the eyebrow from
// the wordmark, so it reads as its own caption underneath rather than a third
// row of the mark itself.
func (m *splashModel) sign(block string) string {
	// Wide enough for whichever of the two is wider: a wordmark of a few
	// letters would otherwise wrap the line underneath it mid-word.
	line := baseStyle.Width(max(lipgloss.Width(block), lipgloss.Width(m.version))).Align(lipgloss.Center)

	parts := make([]string, 0, gapS+2)
	parts = append(parts, block)
	for range gapS {
		parts = append(parts, line.Render(""))
	}
	parts = append(parts, line.Render(m.signature()))

	return lipgloss.JoinVertical(lipgloss.Center, parts...)
}

// signature is the version, put up once the wordmark has settled: the last
// word, not a caption reading along beside it. Its row is held empty until
// then, so nothing moves when it arrives.
func (m *splashModel) signature() string {
	if m.elapsed < sweepFor {
		return ""
	}
	return mutedStyle.Render(m.version)
}
