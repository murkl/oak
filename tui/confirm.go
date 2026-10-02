package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// confirmScreen is the last page before anything is changed: a yes or no, the
// question in the warning colour over it. What is about to happen is every
// answer on the settings page, one esc away, so the page names none of them.
//
// It opens on No: the row that led here was chosen with enter, and an enter
// pressed once too often must not start the work.
//
// It is also the last moment an answer can still be put again, so every answer
// a list vouches for is read against that list once more on the way in - see
// Runner.Unoffered.
type confirmScreen struct {
	app    *app
	picker *picker

	// checking is whether the lists are still being read; pressed, whether
	// Yes came while they were. It starts the run the moment they are through
	// rather than being lost.
	checking, pressed bool
}

func newConfirm(a *app) *confirmScreen {
	s := &confirmScreen{app: a, checking: true}
	s.picker = newPicker([]item{
		{title: labelYes(), key: keyYes},
		{title: labelNo(), key: keyNo},
	})
	s.picker.describe(labelIrreversible())
	s.picker.focus(keyNo)
	return s
}

func (s *confirmScreen) Title() string { return "" }

func (s *confirmScreen) Hint() string { return labelHintChoose() }

// working puts the turning mark in the header while the lists are read.
func (s *confirmScreen) working() bool { return s.checking }

func (s *confirmScreen) Init() tea.Cmd {
	check := s.app.runner.Unoffered()
	return func() tea.Msg { return unofferedMsg{check()} }
}

// unofferedMsg is the answers the lists no longer offer.
type unofferedMsg struct{ names []string }

func (s *confirmScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case unofferedMsg:
		s.checking = false
		for _, name := range msg.names {
			s.app.store.Unoffer(name)
		}
		// Asked again in place of this page, as a run of questions that lands
		// on the hub once it is through: what is about to happen is worth
		// reading a second time with the new answer in it. The hub stays
		// underneath, so esc on the question is a step back and not the way out.
		if missing := s.app.store.Missing(); len(missing) > 0 {
			return s, replace(newWizard(s.app).screen(missing[0]))
		}
		if s.pressed {
			return s, push(startInstall(s.app, 0))
		}
	case tea.KeyMsg:
		s.picker.Update(msg)
		switch {
		case confirms(msg) && s.picker.selected() != keyYes:
			return s, pop()
		case confirms(msg) && s.checking:
			s.pressed = true
		case confirms(msg):
			return s, push(startInstall(s.app, 0))
		case backs(msg):
			return s, pop()
		}
	}
	return s, nil
}

func (s *confirmScreen) View(width, height int) string {
	return alertStyle.Render(labelContinue()) + "\n\n" + s.picker.View(width, max(height-2, 1))
}

// startInstall is the way into an installation: the secrets that have to be
// typed first, in order, and the run itself once there are none left.
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
	// A finished run ends on its result, under it whatever the module offers
	// once the work is done, and going on from there leaves. A failed one lands
	// back on the hub, which is where a wrong answer is corrected.
	return newRun(a, a.runner.Tasks(), leave, func() tea.Cmd { return reset(newHub(a)) })
}
