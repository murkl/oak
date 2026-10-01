package tui

import (
	"strings"

	"github.com/murkl/oak/internal/logging"
	"github.com/murkl/oak/internal/spec"

	tea "github.com/charmbracelet/bubbletea"
)

// leaveScreen is the way out, on a machine where leaving is not quitting a
// program.
//
// A module that names actions for this page is saying that this machine booted
// to run it: quitting into whatever is behind it is not an exit unless there is
// something there. So every way out of the interface arrives here instead, and
// what is offered is what the module says this machine can be left in —
// switched off, started again — and, under those, running with the interface
// closed. In a kiosk there is no console to go back to, and that last row
// starts the program over instead.
//
// It is drawn over whatever was happening rather than in place of it, and
// nothing is stopped by its appearing: a run carries on behind it and the
// header keeps counting. Choosing a row is what stops it — that is what halt
// is, and it is called before any row does anything else.
//
// The actions are the module's, which is also what makes them harmless while
// one is being tried out: under --debug an action is not run unless it
// simulates itself, and the program simply closes.
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

// The runtime's own rows. The NUL prefix cannot collide with anything a module
// names.
const (
	keyConsole   = "\x00console"
	keyStartOver = "\x00start-over"
)

func newLeave(a *app, halt func(), running bool) *leaveScreen {
	s := &leaveScreen{app: a, halt: halt}
	var items []item
	for _, act := range a.rows(a.module.Rules.OnLeave) {
		items = append(items, actionRow(act))
	}
	// Last: the rows above end this machine's session, and this one only ends
	// the program. It reads as the smallest of them and belongs under them. A
	// kiosk has nothing to go back to, so the same place holds the one thing it
	// can do instead.
	if a.kiosk {
		items = append(items, item{title: labelStartOver(), detail: labelStartOverHelp(), key: keyStartOver})
	} else {
		items = append(items, item{title: labelConsole(), detail: labelConsoleHelp(), key: keyConsole})
	}
	s.picker = newPicker(items)
	// Said out loud only while there is something to say it about: whoever
	// reached this page in the middle of a run is owed both halves of it — that
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
// arrives when something went wrong, and when nothing was really done — a
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
			// Back to whatever this was drawn over — the hub, a question, an
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
	// running behind this page is put down first — every row ends it.
	s.halt()
	// The console is not a command and nothing is waiting for it: the program
	// closes, and whatever started it is back.
	if key == keyConsole {
		return quit()
	}
	// Starting over is every answer forgotten and the program closed, for
	// whatever keeps a kiosk running to start it again: a new process is the
	// one start that owes nothing to the run before it. Answers that will not
	// go are said the way the settings page says so, while the program is still
	// standing to say it.
	if key == keyStartOver {
		if err := s.app.store.Reset(); err != nil {
			logging.Error("%s", err)
			return flashBad(err.Error())
		}
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
		b.WriteString(head + "\n\n" + renderFailure(s.err, width) + "\n\n")
		height -= strings.Count(b.String(), "\n")
	}
	return b.String() + withDetail(s.picker, width, height)
}
