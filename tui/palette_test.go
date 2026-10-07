package tui

import (
	"regexp"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// A folder names one accent and cannot know which terminal it will be shown in.
// A green picked against a dark field would otherwise arrive on white paper as
// a pale smudge nobody can read.
func TestAnAccentIsTakenToWhereItCanBeRead(t *testing.T) {
	for _, field := range []lipgloss.Color{darkScheme.bezel, lightScheme.bezel} {
		for _, accent := range []string{"#1793d1", "#a3be8c", "#ffffff", "#000000"} {
			got := readable(lipgloss.Color(accent), field)
			if c := contrast(got, field); c < minContrast {
				t.Errorf("%s on %s has contrast %.2f, want at least %.1f", accent, field, c, minContrast)
			}
		}
	}
}

// An accent this cannot measure is still an accent somebody chose.
func TestAnAccentInSomeOtherNotationIsLeftAlone(t *testing.T) {
	if got := readable(lipgloss.Color("5"), darkScheme.bezel); got != lipgloss.Color("5") {
		t.Errorf("got %q, want it untouched", got)
	}
}

// Both schemes have to carry, not merely be legible: the light interface is the
// same design, not a second one.
func TestBothSchemesAreReadableOnTheirOwnField(t *testing.T) {
	for name, s := range map[string]scheme{"dark": darkScheme, "light": lightScheme} {
		for role, c := range map[string]lipgloss.Color{
			"soft": s.soft, "muted": s.muted,
			"info": s.info, "head": s.head, "warn": s.warn, "fail": s.fail, "accent": s.accent,
		} {
			if got := contrast(c, s.bezel); got < minContrast {
				t.Errorf("%s %s has contrast %.2f against its field", name, role, got)
			}
		}
		// The rule and the border only have to be seen, not read.
		if got := contrast(s.sunk, s.bezel); got < 1.6 {
			t.Errorf("%s rule has contrast %.2f, too close to the field to see", name, got)
		}
	}
}

func TestContrastAndLuminanceAgreeWithTheStandard(t *testing.T) {
	black, white := lipgloss.Color("#000000"), lipgloss.Color("#ffffff")
	if got := contrast(black, white); got < 20.9 || got > 21.1 {
		t.Errorf("black on white = %.2f, want 21", got)
	}
	if got := contrast(white, white); got != 1 {
		t.Errorf("white on white = %.2f, want 1", got)
	}
}

// sixteen puts the interface on a terminal of sixteen colours under the
// runtime's accent for one test, and back afterwards.
func sixteen(t *testing.T, accent string) {
	t.Helper()
	was := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI)
	SetAccent(accent)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(was)
		SetAccent("")
	})
}

// inSlot reports whether text is drawn with the SGR parameter code, alone or
// beside bold.
func inSlot(text, code string) bool {
	return regexp.MustCompile(`\x1b\[(\d+;)*` + code + `(;\d+)*m`).MatchString(text)
}

// On a terminal of sixteen colours every role takes the slot of its own hue,
// so a console dressed in any theme shows it in that theme's colour. The stock
// colour nearest Nord's green is yellow, and a check that passed would read as
// a warning.
func TestOnSixteenColoursEveryRoleTakesTheSlotOfItsHue(t *testing.T) {
	sixteen(t, "")
	for role, c := range map[string]struct {
		style lipgloss.Style
		code  string
	}{
		"good": {goodStyle, "32"}, "fail": {failStyle, "31"}, "warn": {alertStyle, "33"},
		"head": {headStyle, "34"}, "info": {infoStyle, "36"}, "soft": {softStyle, "37"},
		"muted": {mutedStyle, "90"}, "rule": {ruleStyle, "90"}, "accent": {accentStyle, "32"},
	} {
		if got := c.style.Render("x"); !inSlot(got, c.code) {
			t.Errorf("%s is drawn as %q, want SGR %s", role, got, c.code)
		}
	}
}

// A console draws bold as the bright slot, so a heading there is its colour
// alone: bold, it would land where a blue accent sits.
func TestOnSixteenColoursAHeadingIsNotBold(t *testing.T) {
	sixteen(t, "#1793d1")
	if got := headStyle.Render("x"); inSlot(got, "1") {
		t.Errorf("a heading is drawn as %q, bold", got)
	}
}

// The accent is the runtime's, and so is its hue: it goes to the stock colour
// nearest the one the runtime named, not the one it was lightened to.
func TestOnSixteenColoursTheAccentKeepsItsOwnHue(t *testing.T) {
	sixteen(t, "#1793d1")
	if got := accentStyle.Render("x"); !inSlot(got, "94") {
		t.Errorf("Arch blue is drawn as %q, want bright blue", got)
	}
}

// The wordmark settles in the accent's slot, like everything else drawn in the
// accent.
func TestTheSettledWordmarkIsInTheAccentsSlot(t *testing.T) {
	sixteen(t, "#1793d1")
	m := newSplash("Oak\n\nOAK", "")
	run(m)
	if got := m.mark()[2]; !inSlot(got, "94") {
		t.Errorf("the wordmark is not in the accent's slot:\n%q", got)
	}
}

// A terminal with no font of its own is a console of sixteen slots, whatever
// its environment claims.
func TestAConsoleIsHeldToSixteenColours(t *testing.T) {
	was := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(was) })

	lipgloss.SetColorProfile(termenv.ANSI256)
	adaptProfile(true)
	if got := lipgloss.ColorProfile(); got != termenv.ANSI {
		t.Errorf("a console is drawn in %s, want ANSI", got.Name())
	}

	lipgloss.SetColorProfile(termenv.ANSI256)
	adaptProfile(false)
	if got := lipgloss.ColorProfile(); got != termenv.ANSI256 {
		t.Errorf("a terminal with a font of its own is drawn in %s, want ANSI256", got.Name())
	}

	lipgloss.SetColorProfile(termenv.Ascii)
	adaptProfile(true)
	if got := lipgloss.ColorProfile(); got != termenv.Ascii {
		t.Errorf("a terminal without colour is drawn in %s, want Ascii", got.Name())
	}
}

// Which scheme is worn follows the terminal and nothing else: there is no
// setting for it, and the interface never paints a background of its own.
func TestTheSchemeFollowsTheTerminal(t *testing.T) {
	defer adapt(true)
	adapt(false)
	if colors.bezel != lightScheme.bezel {
		t.Error("a light terminal did not get the light scheme")
	}
	adapt(true)
	if colors.bezel != darkScheme.bezel {
		t.Error("a dark terminal did not get the dark scheme")
	}
}
