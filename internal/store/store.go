// Package store holds the answers: what every variable is set to right now, and
// the file they survive a restart in.
package store

import (
	"os"
	"slices"

	"github.com/murkl/oak/internal/exec"
	"github.com/murkl/oak/internal/i18n"
	"github.com/murkl/oak/internal/spec"
)

// Store is every declared variable and its current value.
//
// A value can come from four places, each beating the one before it: the
// declared default, the saved answer file, the preset somebody chose, and the
// answer somebody typed. That order is what lets a saved file stop the
// questions from being asked twice.
//
// The environment this program was started in is not among them. What a script
// is handed is settled here and given to it — a variable inherited from
// whatever shell happened to start this run is as often an accident as an
// instruction, and an installation is not a place to guess which.
type Store struct {
	mod   *spec.Module
	val   map[string]string
	path  string // where the answers are written
	debug bool
}

// New builds a store from the folder's declarations, already carrying every
// default. path is the answer file, read by Load and written by Save, and
// debug is whether this run only pretends to work.
func New(mod *spec.Module, path string, debug bool) *Store {
	s := &Store{mod: mod, val: map[string]string{}, path: path, debug: debug}
	for _, v := range mod.Vars {
		s.val[v.Name] = v.Default.String()
	}
	return s
}

// Path is the answer file this store reads and writes.
func (s *Store) Path() string { return s.path }

// Simulating reports whether this run only pretends to work. Nothing reaches
// the machine, so there is nothing on it for a task's own check to read.
func (s *Store) Simulating() bool { return s.debug }

// Get reads a value.
func (s *Store) Get(name string) string { return s.val[name] }

// Set records an answer. It does not save — the caller decides when the file is
// written, because a value being tried out and a value being settled are not
// the same thing.
func (s *Store) Set(name, value string) { s.val[name] = value }

// Env is what a script sees: the process environment, then every declared
// variable, and then the two names Oak keeps for itself. Later entries win, so
// a variable always carries the value the store holds and never a stale
// inherited one.
//
// Those two are the whole of what Oak adds. MODULE_CONF is the answer file,
// which is the one channel in both directions — a script reads its answers from
// the environment and writes one back by appending a line to that file, exactly
// as somebody editing it by hand would. DEBUG is there only when the run was
// started with --debug, so `[ "$DEBUG" = true ]` is the whole test and there is
// no second value to remember. Everything else a script used to be handed it
// can work out for itself: its own folder is where lib.sh was sourced from.
//
// Secrets are in here like anything else — that is the whole reason they are
// asked for. They reach one bash process and go no further: not to the answer
// file, not to the log.
func (s *Store) Env() exec.Env {
	env := append(exec.Env{}, os.Environ()...)
	for _, v := range s.mod.Vars {
		env = append(env, v.Name+"="+s.val[v.Name])
	}
	env = append(env, spec.ConfVar+"="+s.path)
	if s.debug {
		env = append(env, spec.DebugVar+"="+spec.BoolTrue)
	}
	return env
}

// Apply takes the values of a chosen preset option. Nothing else about it
// survives being chosen: it is a set of answers, not a mode the installer stays
// in, so from here on every one of them is an ordinary value that can be
// changed.
func (s *Store) Apply(o *spec.PresetOption) {
	for name, value := range o.Values {
		s.val[name] = value.String()
	}
}

// Missing lists the questions still standing, in the order they were declared:
// every variable that is required, means something given the answers so far,
// and has no acceptable value yet.
//
// Two kinds are not among them, and for the same reason: there is no answering
// them yet, so leaving them in would be a machine that can never be finished
// answering. A secret is never written down and is asked for immediately before
// the run that needs it — see Secrets. A deferred value is one a task asks for
// mid-run, because until that task's turn there is nothing to choose from.
func (s *Store) Missing() []*spec.Variable {
	var out []*spec.Variable
	for _, v := range s.mod.Vars {
		if v.Secret() || v.Deferred() || !v.Applies(s.Get) {
			continue
		}
		if s.Invalid(v, s.val[v.Name]) != "" {
			out = append(out, v)
		}
	}
	return out
}

// Upfront lists the questions this module wants settled before anything else
// happens at all: before a network is joined, before the machine is checked,
// before a starting point is chosen.
//
// The same rule as Missing, narrowed — so a question already answered is not
// asked again on the way in, and a second start goes straight to the network.
func (s *Store) Upfront() []*spec.Variable {
	var out []*spec.Variable
	for _, v := range s.Missing() {
		if v.First {
			out = append(out, v)
		}
	}
	return out
}

// Secrets lists the variables that have to be typed before a run and are never
// kept — in declaration order, so a folder decides what is asked first.
func (s *Store) Secrets() []*spec.Variable {
	var out []*spec.Variable
	for _, v := range s.mod.Vars {
		if v.Secret() && v.Applies(s.Get) {
			out = append(out, v)
		}
	}
	return out
}

// Visible lists the variables worth showing on the settings page: everything
// that means something given the answers so far, in declaration order.
//
// A deferred value is not among them. It is a row nobody could answer from
// here — what it offers is read off work that has not happened yet — and a
// settings page is a promise that every row on it can be opened.
func (s *Store) Visible() []*spec.Variable {
	var out []*spec.Variable
	for _, v := range s.mod.Vars {
		if !v.Deferred() && v.Applies(s.Get) {
			out = append(out, v)
		}
	}
	return out
}

// Invalid returns why a value will not do, or "" if it will. The rules come
// from the declaration, so a value typed at a prompt and one edited straight
// into the answer file are held to exactly the same standard.
func (s *Store) Invalid(v *spec.Variable, value string) string {
	if value == "" {
		if v.Required {
			return v.Why()
		}
		return ""
	}
	switch {
	case v.Shape() == spec.TypeBool:
		if value != spec.BoolTrue && value != spec.BoolFalse {
			return v.Why()
		}
	case len(v.Values) > 0 && !slices.Contains(v.Values, value):
		return v.Why()
	}
	if !v.Matches(value) {
		return v.Why()
	}
	return ""
}

// Display is what a value looks like on a page: a secret as dots, a bool in
// words, and an unanswered question as a dash rather than as nothing at all —
// an empty column reads as a row that is still loading.
func (s *Store) Display(v *spec.Variable) string {
	value := s.val[v.Name]
	switch {
	case v.Secret():
		return i18n.T("asked just before the run")
	case value == "":
		return "—"
	}
	return Label(value)
}

// Label is how one value is read out loud: true and false in the interface's
// own language wherever they turn up, and every other value as itself.
//
// The two words are the runtime's rather than a folder's — they are what a
// script tests against, and what they are called on screen is not something a
// folder should have to translate — so a list that offers a third answer beside
// them still reads as Yes and No.
func Label(value string) string {
	switch value {
	case spec.BoolTrue:
		return i18n.T("Yes")
	case spec.BoolFalse:
		return i18n.T("No")
	}
	return value
}

// Forget drops every secret, so nothing is left in memory once the run that
// needed it is over.
func (s *Store) Forget() {
	for _, v := range s.mod.Vars {
		if v.Secret() {
			s.val[v.Name] = ""
		}
	}
}
