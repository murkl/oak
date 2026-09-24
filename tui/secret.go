package tui

import (
	"strings"

	"github.com/murkl/oak/internal/spec"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// secretScreen asks for a value that is never written down.
//
// Twice, where the password is being chosen: nothing can check one that does
// not exist yet, and a typo in it is discovered at the first boot of a system
// that took twenty minutes to build. The second entry costs four seconds.
// Once, where it already exists and is only being handed over — see
// Variable.Existing — because the thing it is handed to answers within seconds
// and says which of the two it was.
//
// Where the module declares a check, the password is tried on what it opens
// before it is taken, and a wrong one is refused here rather than halfway
// through the run that needed it — see Variable.Check.
//
// What is typed reaches the environment of the bash process that runs the
// stages, and of the check before it where there is one. Not the answer file,
// not the log, not the argument list of anything, and not the screen, which
// shows one dot per character.
type secretScreen struct {
	app  *app
	v    *spec.Variable
	done func() tea.Cmd

	input    textinput.Model
	first    string
	again    bool
	problem  string
	checking bool
}

// triedMsg is what the module's check made of the password it was handed.
type triedMsg struct {
	value string
	ok    bool
}

func newSecret(a *app, v *spec.Variable, done func() tea.Cmd) *secretScreen {
	s := &secretScreen{app: a, v: v, done: done}
	s.box()
	return s
}

// box is a fresh, empty entry field. Fresh every time rather than cleared: the
// repeat has to be typed, never edited from what the first entry left behind.
func (s *secretScreen) box() {
	s.input = textinput.New()
	s.input.EchoMode = textinput.EchoPassword
	s.input.EchoCharacter = []rune(glyphs.secret)[0]
	s.input.CharLimit = 128
	styleInput(&s.input)
	s.input.Focus()
}

func (s *secretScreen) Init() tea.Cmd { return textinput.Blink }

// takesText: the whole page is a box being typed into, and a password is
// allowed every letter there is — q included.
func (s *secretScreen) takesText() bool { return true }

// working is the check, while it runs: the mark turns in the header, and the
// box takes nothing until the answer is in.
func (s *secretScreen) working() bool { return s.checking }

func (s *secretScreen) Title() string { return s.v.Label() }

func (s *secretScreen) Hint() string {
	if s.checking {
		return labelHintRunning()
	}
	return labelHintInput()
}

func (s *secretScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	if msg, ok := msg.(triedMsg); ok {
		return s.tried(msg)
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok || s.checking {
		return s, nil
	}
	switch {
	// Esc alone: the whole page is a box, so backspace is the delete key here
	// and never the way out.
	case cancels(key):
		// Backing out of the repeat goes back to the first entry, not out of the
		// page: the two are one question.
		if s.again {
			s.again, s.problem = false, ""
			s.first = ""
			s.box()
			return s, nil
		}
		return s, pop()
	case confirms(key):
		return s.commit()
	}
	var cmd tea.Cmd
	s.input, cmd = s.input.Update(key)
	return s, cmd
}

func (s *secretScreen) commit() (screen, tea.Cmd) {
	value := s.input.Value()
	switch {
	case !s.again && value == "" && s.v.Required:
		s.problem = s.v.Why()
		return s, nil

	case !s.again && s.v.Repeats():
		s.first, s.again, s.problem = value, true, ""
		s.box()
		return s, nil

	case s.again && value != s.first:
		// Back to the beginning rather than asking for the repeat again: one of
		// the two was wrong and there is no telling which.
		s.first, s.again = "", false
		s.problem = labelPasswordMismatch()
		s.box()
		return s, nil
	}
	if check := s.app.runner.Check(s.v, value); check != nil {
		s.checking, s.problem = true, ""
		return s, func() tea.Msg { return triedMsg{value: value, ok: check()} }
	}
	s.app.store.Set(s.v.Name, value)
	return s, s.done()
}

// tried takes the password the check accepted, or starts the page over under
// the reason it was refused: one of the two entries was the wrong one, and
// there is no telling which.
func (s *secretScreen) tried(msg triedMsg) (screen, tea.Cmd) {
	s.checking = false
	if !msg.ok {
		s.first, s.again, s.problem = "", false, s.v.WhyRefused()
		s.box()
		return s, nil
	}
	s.app.store.Set(s.v.Name, msg.value)
	return s, s.done()
}

func (s *secretScreen) View(width, height int) string {
	var b strings.Builder
	if help := s.v.Help(); help != "" {
		b.WriteString(paragraph(help, width) + "\n\n")
	}
	label := s.v.Label()
	if s.again {
		label = labelPasswordRepeat()
	}
	b.WriteString(softStyle.Render(label) + "\n")
	b.WriteString(cursorStyle.Render(glyphs.cursor) + s.input.View())
	if s.problem != "" {
		b.WriteString("\n\n" + failStyle.Render(truncate(s.problem, width)))
	}
	return b.String()
}
