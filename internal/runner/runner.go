// Package runner joins the module to the answers: what a question offers right
// now, which tasks this run consists of, and how one of them is started.
//
// It is the only thing above the shell layer that starts anything, and the only
// thing below the interface that knows what a task is. The interface asks it
// questions and draws the answers; it never reaches for a process itself.
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
	sh := exec.Runner{Shells: mod.Shells(), Module: mod.Name()}
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

// Option is one answer a question offers: the value that gets stored, and the
// text it is chosen by.
//
// They are usually the same word, and for a written-out set they always are.
// They part company where the value is a name nobody can pick between on its
// own — a disk is /dev/nvme0n1, and what tells it apart from the other one is
// its size and its model. A command may hand back both by putting a tab
// between them: everything before the tab is stored, everything after it is
// read. One rule, one character, and nothing to declare.
type Option struct {
	Value string
	Label string
}

// Options is the set of answers a question offers, in the order it offers them.
// Nil means there is no set and the answer is typed.
//
// A bool answers itself — the two values are the runtime's, not the folder's,
// because what they mean is the runtime's business: they are what a script
// tests against, and what they are called is a word in the interface's own
// language rather than anything a folder should have to translate.
func (r *Runner) Options(v *spec.Variable) ([]Option, error) {
	switch {
	case v.Shape() == spec.TypeBool:
		return []Option{
			{Value: spec.BoolTrue, Label: store.Label(spec.BoolTrue)},
			{Value: spec.BoolFalse, Label: store.Label(spec.BoolFalse)},
		}, nil
	case len(v.Values) > 0:
		out := make([]Option, len(v.Values))
		for i, value := range v.Values {
			out[i] = Option{Value: value, Label: store.Label(value)}
		}
		return out, nil
	case v.Command != "":
		lines, err := r.sh.Lines(v.Command, r.store.Env())
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

// Unoffered reads every list an answer was chosen from once more and names the
// answers it no longer offers. A list vouches for nothing it does not print
// now: an answer file copied over from another machine, or one shared and
// fetched, names a disk or a keymap as it stood there, and a value made up
// outright passes no list at all. The rules a value is written against do not
// catch it, because the list is the rule.
//
// Read right before the work starts, which is the last moment the answer can
// still be put again: a device name is a path, and what is at the path is
// whatever this machine has plugged in now. Only lists a command prints are
// read - a written-out set is checked by the store every time - and only ones
// that do not also take a typed answer, since that one is not supposed to be in
// the list. A list that cannot be read vouches neither way and is let be.
//
// Handed back as something to run, like Import: the environment is taken now,
// on the goroutine that owns the answers, and the shell runs off the frame.
func (r *Runner) Unoffered() func() []string {
	env := r.store.Env()
	type held struct {
		v     *spec.Variable
		value string
	}
	var lists []held
	for _, v := range r.mod.Vars {
		if v.Command == "" || v.Free != "" || v.Secret() || v.Deferred() || v.Derived() || !v.Applies(r.store.Get) {
			continue
		}
		if value := r.store.Get(v.Name); value != "" {
			lists = append(lists, held{v, value})
		}
	}
	return func() []string {
		var out []string
		for _, h := range lists {
			lines, err := r.sh.Lines(h.v.Command, env)
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

// Prefill is the value a question opens on when nothing has answered it yet — a
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

// Apply puts an answer into effect on the machine the installer is running on,
// rather than on the one being installed — the console keyboard, which is
// unusable as a stored string and has to be loaded before the next thing is
// typed.
//
// A failure is logged and handed back. Where the answer was just given, it
// stands all the same: an installer that stops because a keymap would not load
// is worse than one carrying on with the layout it already had, and whoever
// chose it is looking at the keyboard it did or did not change. See Settle for
// the answer nobody is looking at.
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

// Check hands a secret to the module's check before it is taken — an existing
// password, tried on what it opens — and answers whether it was accepted. What
// the check said where it said no goes to the log, since the page reads the
// module's own words. Nil where the variable declares none.
//
// Handed back as something to run: the environment is taken now,
// with the value in it under its own name, and the shell runs off the frame —
// a wrong password is refused slowly on purpose.
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
// it answered into force — the console keyboard, most of all, since
// what is typed next is typed on it.
func (r *Runner) Imported() error {
	// Load again over what is held. The answer file is the channel because it
	// already is one: shell, KEY='value' to a line, and somebody editing it in
	// an editor is doing exactly what such a script does — so there is no second
	// way in for a script to learn and no second way for one to go wrong.
	if err := r.store.Load(); err != nil {
		return err
	}
	r.Settle()
	return nil
}

// Resolve works out every answer the module reads off the machine instead of
// asking for it. It runs when the module opens and again whenever an answer
// changes, so a value worked out from another answer follows it.
//
// A script that fails or prints nothing leaves the value empty, exactly as an
// unanswered question is: there is no question here to fall back on, and a
// guard on an empty name is simply false.
func (r *Runner) Resolve() {
	env := r.store.Env()
	for _, v := range r.mod.Vars {
		if !v.Derived() {
			continue
		}
		out, err := r.sh.Run(v.Answer, env)
		if err != nil {
			logging.Warn("answer for %s: %s", v.Name, err)
			out = ""
		}
		r.store.Set(v.Name, out)
	}
}

// Settle brings the live system in line with the answers: whatever the module
// reads for itself is read again, and every answer that stands is applied. At
// startup, so a second start stands where the first one left off, and after a
// preset, whose values were never typed at a prompt that could have applied
// them one at a time.
//
// An answer that cannot be put in force goes back to what it was before anybody
// answered, so it is asked again. Nobody watched it being applied: an answer
// file handed over from another machine names a keymap that loaded there, and a
// password typed next on the layout it failed to load is refused without a word
// about why.
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

// Status reads the header's status once, as the machine stands now: whether
// its script says yes. Nil where neither the module nor the product declares
// one.
//
// Handed back as something to run, like Import: the environment is taken now,
// on the goroutine that owns the answers, and the shell — which may go out to
// the network — runs off the frame.
func (r *Runner) Status() func() bool {
	st := r.mod.Status
	if st == nil {
		return nil
	}
	env := r.store.Env()
	return func() bool {
		_, err := r.sh.Run(st.Script, env)
		return err == nil
	}
}

// Tasks is what this run consists of: the ones whose conditions hold, in the
// order the module put them in. One that
// has ruled itself out is not listed at all — the list is a promise of what is
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
// to the one that did the work — a test listed beside the work would be a
// second list of the installation's steps, able to fall out of step with the
// first.
func (r *Runner) Test(t *spec.Task) (*exec.Session, error) {
	logging.Info("%s: %s", t.Title, "test")
	return r.sh.Start(step(t, t.Check()), r.store.Env())
}

// Terminal is an action that takes the terminal over, built but not started —
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

// Offered is whether this machine has an action at all: every action it
// requires says yes, each run by itself. Everything is offered under --debug,
// the way every module is: a simulated run is read on whatever machine somebody
// is sitting at, and a list narrowed to it would hide the pages they opened it
// for.
//
// Handed back as something to run: the environment is taken now, on the
// goroutine that owns the answers, and the shell — which may wait for a card to
// show up — runs off the frame.
func (r *Runner) Offered(a *spec.Action) func() bool {
	if len(a.Requires) == 0 || r.store.Debug() {
		return func() bool { return true }
	}
	env, required := r.store.Env(), r.mod.Named(a.Requires)
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

// Says runs an action by itself, as a question rather than as work: whether it
// says yes. That is how an action the work requires is asked. What it wrote on
// stderr where it says no goes to the log: what somebody reads is its fail.
// Nothing is asked under --debug, for the same reason everything is offered
// there.
//
// Handed back as something to run, like Offered.
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

// Open starts what an action does once somebody has opened it, with its page's
// answer in the environment, the way Start starts a task: in the background,
// reporting how it broke where it did. It is not part of a run, and nothing
// follows it that a list would show.
//
// Under --debug it is only started where it declared it simulates itself: the
// runtime cannot know what a script would change. A nil session is that —
// nothing started, and nothing to wait for.
func (r *Runner) Open(a *spec.Action) (*exec.Session, error) {
	if r.store.Debug() && !a.Simulates {
		logging.Info("action %s: simulated", a.ID())
		return nil, nil
	}
	logging.Info("action %s", a.ID())
	return r.sh.Start(actionStep(a), r.store.Env())
}
