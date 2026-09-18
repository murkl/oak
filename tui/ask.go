package tui

import (
	"fmt"
	"strings"

	"github.com/murkl/oak/internal/runner"
	"github.com/murkl/oak/internal/spec"

	tea "github.com/charmbracelet/bubbletea"
)

// ask is a question put in the middle of a run: a value that could not have
// been known before some of the work was done — which snapshot to go back to,
// once the disk holding them is open.
//
// It stands where the list of tasks was rather than on a page of its own. A run
// is one thing happening, and stepping out of it to answer something and back
// in again would say that two things are, when what is really going on is a
// single stretch of work that stopped to ask.
//
// It is answered or it is not got past. There is nothing behind it to go back
// to: the task after it needs the value, and half the work is already done.
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

// fill takes the answers and reports what to do with the task behind them.
//
// A list that came back empty is a task with nothing to do: this machine has no
// snapshot to go back to, no second disk to pick. That is a step to skip, not a
// run to abandon — and it is the one outcome a module cannot declare in
// advance, because what there is to choose from is read off work that has only
// just happened.
//
// A command that *failed* is something else entirely and stops the run: nothing
// was read, so nothing is known, and skipping on that would be a step quietly
// left out because a script had a typo in it.
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
// The narrowing box gets it first, exactly as it does on a question page — a
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
