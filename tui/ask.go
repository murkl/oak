package tui

import (
	"fmt"
	"strings"

	"github.com/murkl/oak/internal/runner"
	"github.com/murkl/oak/internal/spec"

	tea "github.com/charmbracelet/bubbletea"
)

// ask is a question in the middle of a run, for a value only known once some
// work is done, standing where the task list was. It is answered or not got
// past, since the task after it needs the value.
type ask struct {
	v      *spec.Variable
	values []item
	picker *picker
	filter *filter

	loading bool
	problem string
}

func newAsk(v *spec.Variable) *ask {
	open := v.FilterMode() == spec.FilterOpen
	return &ask{v: v, filter: newFilter(open), loading: true}
}

// askedMsg is the answers coming back. Loaded off the frame like every other
// options command: reading them may mean walking a file system that has only
// just been mounted.
type askedMsg struct {
	values []runner.Option
	err    error
}

func (a *ask) Init(app *app) tea.Cmd {
	v := a.v
	return func() tea.Msg {
		values, err := app.runner.Options(v)
		return askedMsg{values: values, err: err}
	}
}

// fill takes the answers and says what to do with the task behind them. An
// empty list skips the task, since there is nothing to choose, while a command
// that failed stops the run rather than hide a typo.
func (a *ask) fill(msg askedMsg, current string) (skip bool, err error) {
	a.loading = false
	if msg.err != nil {
		return false, fmt.Errorf("%s: %w", a.v.Label(), msg.err)
	}
	if len(msg.values) == 0 {
		return true, nil
	}
	a.values = make([]item, 0, len(msg.values))
	for _, o := range msg.values {
		a.values = append(a.values, item{title: o.Label, key: o.Value})
	}
	a.picker = newPicker(a.rows())
	a.picker.focus(current)
	return false, nil
}

// Update offers a key to the question and reports whether it has been answered.
// The narrowing box gets it first, exactly as it does on a question page - a
// list of snapshots is long, and typing through it is the only sane way down.
func (a *ask) Update(key tea.KeyMsg, app *app) (cmd tea.Cmd, given bool) {
	if a.loading {
		return nil, false
	}
	if took, cmd := a.filter.Update(key); took {
		a.narrow()
		return cmd, false
	}
	if !confirms(key) {
		a.picker.Update(key)
		return nil, false
	}
	value, ok := a.picker.chosen()
	if !ok {
		return nil, false
	}
	if why := app.store.Invalid(a.v, value); why != "" {
		a.problem = why
		return nil, false
	}
	app.store.Set(a.v.Name, value)
	return app.save(), true
}

// rows is the answers the box has left of them, and a word where it has left
// none.
func (a *ask) rows() []item {
	items := narrow(a.values, a.filter.query())
	if len(items) == 0 {
		items = append(items, item{title: labelNoMatch(), disabled: true})
	}
	return items
}

func (a *ask) narrow() {
	sel := a.picker.selected()
	a.picker = newPicker(a.rows())
	a.picker.focus(sel)
}

func (a *ask) Hint() string {
	if a.loading {
		return labelHintRunning()
	}
	if a.filter.permanent {
		return labelHintFilterAnswer()
	}
	return filterHint(labelHintAnswer(), a.filter)
}

// View is the question as it sits inside the run: what the value is for, then
// the answers. The same page a question is asked on anywhere else, minus the
// page.
func (a *ask) View(width, height int) string {
	if a.loading {
		return ""
	}
	var b strings.Builder
	if help := a.v.Help(); help != "" {
		head := paragraph(help, width) + "\n\n"
		height -= strings.Count(head, "\n")
		b.WriteString(head)
	}
	rows := 0
	if a.problem != "" {
		rows = 2
	}
	b.WriteString(a.filter.View())
	b.WriteString(a.picker.View(width, height-rows-a.filter.rows()))
	if a.problem != "" {
		b.WriteString("\n\n" + failStyle.Render(truncate(a.problem, width)))
	}
	return b.String()
}
