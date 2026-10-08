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

// Store is every declared variable and its current value: the default, then the
// answer file, then a preset, then what was typed, each beating the one before.
// The environment Oak was started in is none of them, since an inherited
// variable is as often an accident as an instruction.
type Store struct {
	mod   *spec.Module
	val   map[string]string
	path  string // where the answers are written
	debug bool

	// unoffered is every answer a list turned away when it was read again,
	// and the value it turned away - see Unoffer.
	unoffered map[string]string
}

// New builds a store from the folder's declarations, already carrying every
// default. path is the answer file, read by Load and written by Save, and
// debug is whether this run only pretends to work.
func New(mod *spec.Module, path string, debug bool) *Store {
	s := &Store{mod: mod, val: map[string]string{}, path: path, debug: debug, unoffered: map[string]string{}}
	for _, v := range mod.Declared() {
		s.val[v.Name] = v.Default.String()
	}
	return s
}

// Path is the answer file this store reads and writes.
func (s *Store) Path() string { return s.path }

// Get reads a value.
func (s *Store) Get(name string) string { return s.val[name] }

// Set records an answer. It does not save - the caller decides when the file is
// written, because a value being tried out and a value being settled are not
// the same thing.
func (s *Store) Set(name, value string) { s.val[name] = value }

// Env is what a script sees: the process environment, every declared variable
// over it, and MODULE_CONF, the answer file a script appends an answer to, with
// DEBUG=true under --debug. Secrets are in it and go no further than the
// processes it is handed to.
func (s *Store) Env() exec.Env {
	env := append(exec.Env{}, os.Environ()...)
	for _, v := range s.mod.Declared() {
		env = append(env, v.Name+"="+s.val[v.Name])
	}
	env = append(env, spec.ConfVar+"="+s.path)
	if s.debug {
		env = append(env, spec.DebugVar+"="+spec.BoolTrue)
	}
	return env
}

// Debug reports whether this run only pretends to work.
func (s *Store) Debug() bool { return s.debug }

// Apply takes the values of a chosen starting point. Nothing else about it
// survives being chosen: it is a set of answers, not a mode the installer stays
// in, so from here on every one of them is an ordinary value that can be
// changed.
func (s *Store) Apply(o *spec.Preset) {
	for name, value := range o.Values {
		s.val[name] = value.String()
	}
}

// Missing lists the questions still standing in declaration order: every
// required variable that means something given the answers so far and has no
// acceptable value. Secrets, deferred and derived values are no question
// anybody could answer here, so they are not among them.
func (s *Store) Missing() []*spec.Variable {
	var out []*spec.Variable
	for _, v := range s.mod.Vars {
		if v.Secret() || v.Deferred() || v.Derived() || !v.Applies(s.Get) {
			continue
		}
		if s.Invalid(v, s.val[v.Name]) != "" {
			out = append(out, v)
		}
	}
	return out
}

// Upfront lists the questions this module wants settled before anything else
// happens at all: before anything the work waits for is looked at, before a
// starting point is chosen.
//
// The same rule as Missing, narrowed - so a question already answered is not
// asked again on the way in, and a second start goes straight past them.
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
// kept - in declaration order, so a folder decides what is asked first.
func (s *Store) Secrets() []*spec.Variable {
	var out []*spec.Variable
	for _, v := range s.mod.Vars {
		if v.Secret() && v.Applies(s.Get) {
			out = append(out, v)
		}
	}
	return out
}

// Visible lists what the settings page shows: every variable that means
// something given the answers so far, in declaration order. It leaves out what
// Missing leaves out, since a row that cannot be opened only asks why not.
func (s *Store) Visible() []*spec.Variable {
	var out []*spec.Variable
	for _, v := range s.mod.Vars {
		if !v.Secret() && !v.Deferred() && !v.Derived() && v.Applies(s.Get) {
			out = append(out, v)
		}
	}
	return out
}

// Unoffer records that name's answer is no longer among those its list offers,
// which only running the list can tell (see Runner.Unoffered). From then on
// that value is turned away like one that breaks a rule.
func (s *Store) Unoffer(name string) { s.unoffered[name] = s.val[name] }

// Reoffer takes back what Unoffer recorded, for a list read again that offers
// the answer once more - a stick plugged back in.
func (s *Store) Reoffer(name string) { delete(s.unoffered, name) }

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
	if turned, ok := s.unoffered[v.Name]; ok && value == turned {
		return v.WhyUnoffered()
	}
	switch {
	case v.Type == spec.TypeBool:
		if value != spec.BoolTrue && value != spec.BoolFalse {
			return v.Why()
		}
	case len(v.Options) > 0 && !slices.Contains(v.Options, value):
		return v.Why()
	}
	if !v.Matches(value) {
		return v.Why()
	}
	return ""
}

// Display is what a value looks like on a page: a bool in words, and an
// unanswered question as a dash rather than as nothing at all - an empty column
// reads as a row that is still loading.
func (s *Store) Display(v *spec.Variable) string {
	if value := s.val[v.Name]; value != "" {
		return Label(value)
	}
	return "—"
}

// Label is how one value is read out loud: true and false in the interface's
// own language wherever they turn up, and every other value as itself.
//
// The two words are the runtime's rather than a folder's - they are what a
// script tests against, and what they are called on screen is not something a
// folder should have to translate - so a list that offers a third answer beside
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

// Forget drops every secret, so nothing is left in memory once the run or the
// option that needed it is over.
func (s *Store) Forget() {
	for _, v := range s.mod.Declared() {
		if v.Secret() {
			s.val[v.Name] = ""
		}
	}
}
