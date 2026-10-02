package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// confirmScreen is the last page before anything is changed, after every
// password: a yes or no, the question in the warning colour over it. The module
// says in its own words what is about to happen; one that says nothing gets the
// runtime's warning that it cannot be undone.
//
// It opens on No: the password before it was confirmed with enter, and an enter
// pressed once too often must not start the work.
type confirmScreen struct {
	app    *app
	picker *picker

	// question is what the page asks: the module's, or the runtime's own.
	question string
}

func newConfirm(a *app) *confirmScreen {
	s := &confirmScreen{app: a}
	s.picker = newPicker([]item{
		{title: labelYes(), key: keyYes},
		{title: labelNo(), key: keyNo},
	})
	question, body := a.module.Confirm(a.store.Get)
	if question == "" {
		question, body = labelContinue(), labelIrreversible()
	}
	s.question = question
	if body != "" {
		s.picker.describe(body)
	}
	s.picker.focus(keyNo)
	return s
}

func (s *confirmScreen) Title() string { return "" }

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

// startInstall is the way into an installation: the secrets that have to be
// typed first, in order, and the last page once there are none left.
//
// Asked here rather than among the other questions, because a secret is never
// written down: it would be missing again at every start, and no machine could
// ever be finished answering. Here it is typed once, used, and forgotten.
func startInstall(a *app, next int) screen {
	secrets := a.store.Secrets()
	if next < len(secrets) {
		return newSecret(a, secrets[next], func() tea.Cmd {
			return push(startInstall(a, next+1))
		})
	}
	return newConfirm(a)
}

// startRun is the work itself. A finished run ends on its result, under it
// whatever the module offers once the work is done, and going on from there
// leaves. A failed one lands back on the hub, which is where a wrong answer is
// corrected.
func startRun(a *app) screen {
	return newRun(a, a.runner.Tasks(), leave, func() tea.Cmd { return reset(newHub(a)) })
}
