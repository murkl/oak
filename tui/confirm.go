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
type confirmScreen struct {
	app *app
}

func newConfirm(a *app) *confirmScreen { return &confirmScreen{app: a} }

func (s *confirmScreen) Title() string { return s.app.module.Name() }

func (s *confirmScreen) Hint() string { return labelHintStart() }

func (s *confirmScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	switch {
	case confirms(key):
		return s, push(startInstall(s.app, 0))
	case backs(key):
		return s, pop()
	}
	return s, nil
}

func (s *confirmScreen) View(width, height int) string {
	start := labelStartNamed(s.app.module.Name())
	var b strings.Builder
	b.WriteString(alertStyle.Render(labelReadyToStart()) + "\n\n")
	if text := s.app.module.ConfirmText(s.app.store.Get); text != "" {
		b.WriteString(paragraph(text, width) + "\n")
	}
	return b.String() + "\n" + accentBold.Render(glyphs.cursor+start)
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
