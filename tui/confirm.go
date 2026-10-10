package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// confirmScreen is the runtime's yes or no before anything changes, after every
// password, where the module declares confirm. It opens on No, since the
// password before it was confirmed with enter.
type confirmScreen struct {
	app    *app
	picker *picker

	question string
}

func newConfirm(a *app) *confirmScreen {
	s := &confirmScreen{app: a, question: labelReallyStart()}
	s.picker = newPicker([]item{
		{title: labelYes(), key: keyYes},
		{title: labelNo(), key: keyNo},
	})
	s.picker.focus(keyNo)
	return s
}

func (s *confirmScreen) Title() string { return labelConfirmation() }

// crumbRoot: the passwords before this page are done with, and the one just
// typed is no place this page is inside of. It stands under the menu, like they
// do.
func (s *confirmScreen) crumbRoot() bool   { return true }
func (s *confirmScreen) crumbHead() string { return labelMenu() }

func (s *confirmScreen) Hint() string { return labelHintChoose() }

func (s *confirmScreen) Init() tea.Cmd { return nil }

func (s *confirmScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	s.picker.Update(key)
	switch {
	case confirms(key) && s.picker.selected() == keyYes:
		return s, push(startRun(s.app))
	case confirms(key), backs(key):
		return s, s.decline()
	}
	return s, nil
}

// decline goes back to the menu rather than to the password before this page,
// which has done its part, and forgets what was typed there.
func (s *confirmScreen) decline() tea.Cmd {
	s.app.store.Forget()
	return reset(newHub(s.app))
}

func (s *confirmScreen) View(width, height int) string {
	head := alertStyle.Render(paragraph(s.question, width))
	used := strings.Count(head, "\n") + 2 // the question, and the blank line under it
	return head + "\n\n" + s.picker.View(width, max(height-used, 1))
}

// startInstall is the way into the work: what the module asks each time, the
// secrets to type, then the confirmation where declared, or the run. Both kinds
// of question are asked here because neither is written down: each holds for
// this one run.
func startInstall(a *app, next int) screen {
	asked := append(a.store.Asked(), a.store.Secrets()...)
	if next < len(asked) {
		v := asked[next]
		then := func() tea.Cmd { return push(startInstall(a, next+1)) }
		if v.Secret() {
			return newSecret(a, v, then).under(labelMenu())
		}
		return newField(a, v, then).under(labelMenu())
	}
	if a.module.Confirm {
		return newConfirm(a)
	}
	return startRun(a)
}

// startRun is the work itself. A finished run ends on its result and the
// module's success rows, and a failed one lands back on the hub to correct an
// answer.
func startRun(a *app) screen {
	return newRun(a, a.runner.Tasks(), leave, func() tea.Cmd { return reset(newHub(a)) })
}
