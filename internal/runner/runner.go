// Package runner joins the module to the answers: what a question offers now,
// which tasks a run holds and how one starts. It is the only thing above the
// shell layer that starts anything, so the interface never reaches for a
// process itself.
package runner

import (
	"slices"
	"strings"

	"github.com/murkl/oak/internal/exec"
	"github.com/murkl/oak/internal/logging"
	"github.com/murkl/oak/internal/spec"
	"github.com/murkl/oak/internal/store"
)

type Runner struct {
	mod   *spec.Module
	store *store.Store
	sh    exec.Runner
}

func New(mod *spec.Module, st *store.Store) *Runner {
	sh := exec.Runner{Shell: mod.Shell, Module: mod.Name()}
	return &Runner{mod: mod, store: st, sh: sh}
}

// step is one piece of a module's work as the shell layer takes it. What it
// runs is passed in, because a task holds two: the work, and the test that
// looks at what the work left behind.
func step(t *spec.Task, script spec.Script) exec.Step {
	return exec.Step{Name: t.Label(), Script: exec.Script{File: string(script)}}
}

// actionStep is an action as the shell layer takes it.
func actionStep(a *spec.Action) exec.Step {
	return exec.Step{Name: a.Label(), Action: true, Script: exec.Script{File: string(a.Work())}}
}

// Option is one answer a question offers: the value stored, and the text it is
// chosen by. A command prints both with a tab between them where the value
// alone tells nothing, such as a disk by its size and model.
type Option struct {
	Value string
	Label string
}

// Options is the set of answers a question offers, in order, and nil where the
// answer is typed. A bool's two values are the runtime's, since scripts test
// against them and the interface names them in its own language.
func (r *Runner) Options(v *spec.Variable) ([]Option, error) {
	switch {
	case v.Type == spec.TypeBool:
		return []Option{
			{Value: spec.BoolTrue, Label: store.Label(spec.BoolTrue)},
			{Value: spec.BoolFalse, Label: store.Label(spec.BoolFalse)},
		}, nil
	case len(v.Options) > 0:
		out := make([]Option, len(v.Options))
		for i, value := range v.Options {
			out[i] = Option{Value: value, Label: store.Label(value)}
		}
		return out, nil
	case v.OptionsFrom != "":
		lines, err := r.sh.Lines(v.OptionsFrom, r.store.Env())
		if err != nil {
			logging.Warn("options for %s: %s", v.Name, err)
			return nil, err
		}
		return listed(lines), nil
	}
	return nil, nil
}

// listed reads what a command printed as the answers it offers.
func listed(lines []string) []Option {
	out := make([]Option, 0, len(lines))
	for _, line := range lines {
		value, label, tabbed := strings.Cut(line, "\t")
		value = strings.TrimSpace(value)
		if !tabbed {
			label = value
		}
		out = append(out, Option{Value: value, Label: strings.TrimSpace(label)})
	}
	return out
}

// Unoffered reads every printed list an answer was chosen from once more, right
// before the work, and names the answers it no longer offers: an answer file
// from another machine names its disks, and the list is the rule. A list that
// also takes a typed answer, or cannot be read, vouches neither way; the shell
// runs off the frame, like Import's.
func (r *Runner) Unoffered() func() []string {
	env := r.store.Env()
	type held struct {
		v     *spec.Variable
		value string
	}
	var lists []held
	for _, v := range r.mod.Vars {
		if v.OptionsFrom == "" || v.Open() || v.Secret() || v.Deferred() || v.Derived() || !v.Applies(r.store.Get) {
			continue
		}
		if value := r.store.Get(v.Name); value != "" {
			lists = append(lists, held{v, value})
		}
	}
	return func() []string {
		var out []string
		for _, h := range lists {
			lines, err := r.sh.Lines(h.v.OptionsFrom, env)
			if err != nil {
				logging.Warn("options for %s: %s", h.v.Name, err)
				continue
			}
			if !slices.ContainsFunc(listed(lines), func(o Option) bool { return o.Value == h.value }) {
				logging.Info("%s: %s is not among the answers offered", h.v.Name, h.value)
				out = append(out, h.v.Name)
			}
		}
		return out
	}
}

// Prefill is the value a question opens on when nothing has answered it yet - a
// timezone guessed from the network, a keymap read off the live system. Only
// ever a suggestion, and a command that fails or says nothing simply leaves the
// question empty, because a suggestion is never worth stopping for.
func (r *Runner) Prefill(v *spec.Variable) string {
	if v.Prefill == "" {
		return ""
	}
	out, err := r.sh.Run(v.Prefill, r.store.Env())
	if err != nil {
		logging.Warn("prefill for %s: %s", v.Name, err)
		return ""
	}
	return out
}

// Apply puts an answer into effect on the machine Oak runs on, such as the
// console keyboard, which has to be loaded before the next thing is typed. A
// failure is logged and handed back, and the answer stands, since whoever chose
// it sees whether the keyboard changed; see Settle for one nobody watched.
func (r *Runner) Apply(v *spec.Variable) error {
	if v.Apply == "" {
		return nil
	}
	_, err := r.sh.Run(v.Apply, r.store.Env())
	if err != nil {
		logging.Warn("apply for %s: %s", v.Name, err)
	}
	return err
}

// Check tries a secret on the module's check before it is taken and answers
// whether it was accepted, logging what a no said; nil where none is declared.
// The shell runs off the frame, since a wrong password is refused slowly on
// purpose.
func (r *Runner) Check(v *spec.Variable, value string) func() bool {
	if v.Check == "" {
		return nil
	}
	env := append(r.store.Env(), v.Name+"="+value)
	return func() bool {
		err := r.sh.Reason(v.Check, env)
		if err != nil {
			logging.Warn("check for %s: %s", v.Name, err)
		}
		return err == nil
	}
}

// Imported reads back what a script left in the answer file and puts whatever
// it answered into force - the console keyboard, most of all, since
// what is typed next is typed on it.
func (r *Runner) Imported() error {
	// Load again over what is held. The answer file is the channel because it
	// already is one: shell, KEY='value' to a line, and somebody editing it in
	// an editor is doing exactly what such a script does - so there is no second
	// way in for a script to learn and no second way for one to go wrong.
	if err := r.store.Load(); err != nil {
		return err
	}
	r.Settle()
	return nil
}

// Resolve works out every answer the module reads off the machine, when it
// opens and whenever an answer changes. A script that fails or prints nothing
// leaves the value empty, as an unanswered question is.
func (r *Runner) Resolve() {
	env := r.store.Env()
	for _, v := range r.mod.Vars {
		if !v.Derived() {
			continue
		}
		out, err := r.sh.Run(v.ValueFrom, env)
		if err != nil {
			logging.Warn("value-from for %s: %s", v.Name, err)
			out = ""
		}
		r.store.Set(v.Name, out)
	}
}

// Settle reads the machine's values again and applies every standing answer, at
// startup and after a preset, whose values no prompt applied one by one. An
// answer that cannot be put in force goes back to unset and is asked again,
// since nobody watched it fail.
func (r *Runner) Settle() {
	r.Resolve()
	for _, v := range r.mod.Vars {
		if r.store.Get(v.Name) == "" {
			continue
		}
		if err := r.Apply(v); err != nil {
			r.store.Set(v.Name, v.Default.String())
		}
	}
}

// Status reads the header's status as the machine stands now: whether its
// script says yes, nil where nothing declares one. The shell, which may go out
// to the network, runs off the frame like Import's.
func (r *Runner) Status() func() bool {
	st := r.mod.Status
	if st == nil {
		return nil
	}
	env := r.store.Env()
	return func() bool {
		_, err := r.sh.Run(st.Check, env)
		return err == nil
	}
}

// Tasks is what this run consists of: the ones whose conditions hold, in the
// order the module put them in. One that
// has ruled itself out is not listed at all - the list is a promise of what is
// about to happen, and a row that will be skipped is not part of that promise.
func (r *Runner) Tasks() []*spec.Task {
	var out []*spec.Task
	for _, t := range r.mod.Tasks {
		if t.Applies(r.store.Get) {
			out = append(out, t)
		}
	}
	return out
}

// Simulated reports whether this task is only shown as run: under --debug,
// everything but a task that declared it simulates itself. Its test goes with
// it, since there is nothing a run that changed nothing could have left behind.
func (r *Runner) Simulated(t *spec.Task) bool {
	return r.store.Debug() && !t.Simulates
}

// Start runs one task in the background. Its output goes to the log and
// nowhere else; what comes back here is whether it worked.
func (r *Runner) Start(t *spec.Task) (*exec.Session, error) {
	logging.Info("%s", t.Title)
	return r.sh.Start(step(t, t.Work()), r.store.Env())
}

// Test runs what a task declared as its own proof that the work took: the same
// kind of script, started the same way, reading the machine the work was done
// to and changing nothing on it.
//
// It is a task's second script rather than a task of its own because it belongs
// to the one that did the work - a test listed beside the work would be a
// second list of the installation's steps, able to fall out of step with the
// first.
func (r *Runner) Test(t *spec.Task) (*exec.Session, error) {
	logging.Info("%s: %s", t.Title, "test")
	return r.sh.Start(step(t, t.Check()), r.store.Env())
}

// Terminal is an action that takes the terminal over, built but not started -
// the interface has to stand aside first, and only it knows how.
func (r *Runner) Terminal(a *spec.Action) *exec.Handover {
	logging.Info("action %s", a.ID())
	return r.sh.Terminal(exec.Script{File: string(a.Work())}, r.store.Env())
}

// Fail is what comes back from an action the interface stood aside for, in the
// one shape failures are reported in.
func (r *Runner) Fail(a *spec.Action, err error) error {
	return r.sh.Fail(actionStep(a), err)
}

// Offered is whether this machine has an action at all: every action its
// offer-if names says yes. Everything is offered under --debug, as every module
// is, and the shell, which may wait for a card, runs off the frame.
func (r *Runner) Offered(a *spec.Action) func() bool {
	if len(a.Rules.OfferIf) == 0 || r.store.Debug() {
		return func() bool { return true }
	}
	env, required := r.store.Env(), r.mod.Named(a.Rules.OfferIf)
	return func() bool {
		for _, c := range required {
			if err := r.sh.Guard(c.Work().Shell(), env); err != nil {
				logging.Info("action %s: not offered, %s said no: %s", a.ID(), c.ID(), err)
				return false
			}
		}
		return true
	}
}

// Says asks an action the work requires whether it says yes, logging what it
// wrote with a no, since what somebody reads is its fail. Nothing is asked
// under --debug; it runs off the frame like Offered.
func (r *Runner) Says(a *spec.Action) func() bool {
	if r.store.Debug() {
		return func() bool { return true }
	}
	env := r.store.Env()
	return func() bool {
		err := r.sh.Guard(a.Work().Shell(), env)
		if err != nil {
			logging.Info("action %s said no: %s", a.ID(), err)
		}
		return err == nil
	}
}

// Open starts an action somebody opened, with its page's answer in the
// environment, the way Start starts a task. Under --debug only one that
// simulates itself starts, and a nil session means nothing started.
func (r *Runner) Open(a *spec.Action) (*exec.Session, error) {
	if r.store.Debug() && !a.Simulates {
		logging.Info("action %s: simulated", a.ID())
		return nil, nil
	}
	logging.Info("action %s", a.ID())
	return r.sh.Start(actionStep(a), r.store.Env())
}
