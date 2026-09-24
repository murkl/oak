package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// confirmScreen is the last page before anything is changed.
//
// What it says comes entirely from the module, with the answers filled into
// it, because only the module knows what is about to happen — which disk, whether
// it is erased or shared, what that costs. The runtime supplies the moment, not
// the warning.
//
// It answers to enter and nothing else. Every other key, and every scroll — a
// wheel arrives here as an arrow — leaves the page exactly where it is.
//
// It is also the last moment an answer can still be put again, so every answer
// a list vouches for is read against that list once more on the way in — see
// Runner.Unoffered. One the list no longer offers sends the run of questions
// back to it rather than on into the work.
type confirmScreen struct {
	app *app

	// checking is whether the lists are still being read; pressed, whether
	// enter came while they were. Enter is the cheapest key, and it starts the
	// run the moment they are through rather than being lost.
	checking, pressed bool
}

func newConfirm(a *app) *confirmScreen { return &confirmScreen{app: a, checking: true} }

func (s *confirmScreen) Title() string { return s.app.module.Name() }

func (s *confirmScreen) Hint() string { return labelHintStart() }

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
		switch {
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
	var b strings.Builder
	b.WriteString(alertStyle.Render(labelReadyToStart()) + "\n\n")
	if text := s.app.module.ConfirmText(s.app.store.Get); text != "" {
		b.WriteString(paragraph(text, width) + "\n")
	}
	return b.String() + "\n" + accentBold.Render(glyphs.cursor+labelOpening())
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
	// Nothing follows a finished run: everything the module had to offer once the
	// system was installed was a task of the last stage and has been offered.
	// Enter on the result leaves. A failed one lands back on the hub, which is
	// where a wrong answer is corrected.
	return newRun(a, a.runner.Tasks(), leave, func() tea.Cmd { return reset(newHub(a)) })
}
