package tui

import (
	"strings"

	"github.com/murkl/oak/internal/logging"
	"github.com/murkl/oak/internal/spec"

	tea "github.com/charmbracelet/bubbletea"
)

// leaveScreen is the way out where the module says the machine booted to run
// it: what to leave it in, and under that closing the interface, which a kiosk
// starts again. Drawn over what was happening, it stops nothing until a row is
// chosen, and under --debug a row that does not simulate itself only closes the
// program.
type leaveScreen struct {
	app    *app
	halt   func()
	picker *picker

	// doing is the action being carried out, nil while nothing is. A restart
	// takes a moment to arrive and the frame has to say something in it, or the
	// last thing anybody sees is a page that ignored their keystroke.
	doing *spec.Action
	err   error
}

// The runtime's own row. The NUL prefix cannot collide with anything a module
// names.
const keyConsole = "\x00console"

func newLeave(a *app, halt func(), running bool) *leaveScreen {
	s := &leaveScreen{app: a, halt: halt}
	var items []item
	for _, act := range a.rows(a.module.Rules.OnLeave) {
		items = append(items, actionRow(act))
	}
	// Last, since it only ends the program while the rows above end the
	// machine's session. A kiosk has nothing behind the program, so whatever
	// keeps it running starts it again.
	help := labelConsoleHelp()
	if a.kiosk {
		help = labelConsoleKioskHelp()
	}
	items = append(items, item{title: labelConsole(), detail: help, key: keyConsole})
	s.picker = newPicker(items)
	// Said out loud only while there is something to say it about: whoever
	// reached this page in the middle of a run is owed both halves of it - that
	// the run did not stop, and that every row here stops it.
	if running {
		s.picker.describe(labelLeaveRunning())
	}
	return s
}

func (s *leaveScreen) Title() string { return labelLeave() }

func (s *leaveScreen) Hint() string {
	if s.doing != nil {
		return labelHintRunning()
	}
	return labelHintChoose()
}

// working is what puts the turning mark in the header while a machine is on its
// way down.
func (s *leaveScreen) working() bool { return s.doing != nil }

// leftMsg is the option coming back, which on a machine that is genuinely going
// down never happens: the machine takes the program with it long before. It
// arrives when something went wrong, and when nothing was really done - a
// module being tried out.
type leftMsg struct{ err error }

func (s *leaveScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case leftMsg:
		s.doing = nil
		if msg.err != nil {
			// Nothing else to do but say so and stand here: the machine is
			// still running, and the other row may still work.
			logging.Error("%s", msg.err)
			s.err = msg.err
			return s, nil
		}
		return s, quit()

	case tea.KeyMsg:
		// Nothing means anything while the machine is going down. It is about to
		// stop answering, and a keystroke that started a second command into
		// that would be the one thing that could still go wrong.
		if s.doing != nil {
			return s, nil
		}
		s.picker.Update(msg)
		switch {
		case confirms(msg):
			return s, s.carryOut(s.picker.selected())
		case backs(msg):
			// Back to whatever this was drawn over - the hub, a question, an
			// installation that has been running the whole time this page was
			// up. Nothing was stopped to get here, so there is nothing to
			// restore.
			return s, dismiss()
		}
	}
	return s, nil
}

// carryOut does what a row stands for and reports back. An option is not a task
// and does not belong in a run: nothing follows it, there is nothing to report
// to a list, and the log is the only place a failure could be written down
// anyway.
func (s *leaveScreen) carryOut(key string) tea.Cmd {
	if key == "" {
		return nil
	}
	// From here on this is a decision rather than a question, so whatever was
	// running behind this page is put down first - every row ends it.
	s.halt()
	// The console is not a command and nothing is waiting for it: the program
	// closes, and whatever started it is back. The answer file stays, so a kiosk
	// started again opens on every answer it had.
	if key == keyConsole {
		return quit()
	}
	act := s.app.action(key)
	if act == nil {
		return nil
	}
	s.doing, s.err = act, nil
	session, err := s.app.runner.Open(act)
	if err != nil || session == nil {
		return func() tea.Msg { return leftMsg{err} }
	}
	return func() tea.Msg {
		<-session.Done()
		return leftMsg{session.Err()}
	}
}

func (s *leaveScreen) View(width, height int) string {
	if s.doing != nil {
		return accentStyle.Render(spinFrame()) + field(" ") + boldStyle.Render(s.doing.Label())
	}
	var b strings.Builder
	if s.err != nil {
		head := failStyle.Render(glyphs.fail) + field(" ") + boldStyle.Render(labelRunFailed())
		room := height - 2 - 1 - s.picker.height(width)
		b.WriteString(head + "\n\n" + renderFailure(s.err, width, room) + "\n\n")
		height -= strings.Count(b.String(), "\n")
	}
	return b.String() + withDetail(s.picker, width, height)
}
