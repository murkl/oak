package spec

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/murkl/oak/internal/i18n"
)

// Action is something a module does outside its run: a folder under actions/
// whose action.sh says yes by exiting 0, run wherever a rule names it. It has
// one kind of page at most, so a flow of several kinds is several actions, each
// opened on the failure of the one before.
type Action struct {
	Title       string
	Description string

	// Rules is its own: offer-if, the actions that must say yes before this
	// one is offered at all, and on-failure, what is opened where it says no -
	// the first of them this machine offers.
	Rules Rules

	// Error is what a no from this one means, in words: the page in front of
	// the work, the reason a module is not offered, the headline over a failure.
	Error string

	// Vars is its questions before it runs, a page each, like the module's own:
	// their answers are handed to its script and kept for this session only.
	Vars []*Variable

	// Report is its one page after it has run, and Shows the answer drawn
	// there as a code. The script writes that answer into the answer file, and
	// being named here is its whole declaration.
	Report string
	Shows  string

	// TTY is its one page being the terminal itself: the interface stands
	// aside, and the script has the keyboard until it exits.
	TTY bool

	// Simulates runs it under --debug as well, for one that reads DEBUG itself.
	Simulates bool

	work  Script
	shown *Variable
	id    string
	dir   string
}

// actionDeclaration is action.yaml as it is written: the same keys a module
// writes them in, wherever they mean the same.
type actionDeclaration struct {
	Title       string      `yaml:"title"`
	Description string      `yaml:"description"`
	Rules       Rules       `yaml:"rules"`
	Error       string      `yaml:"error"`
	Variables   []*Variable `yaml:"variables"`
	Report      string      `yaml:"report"`
	Shows       string      `yaml:"shows"`
	TTY         bool        `yaml:"tty"`
	Simulates   bool        `yaml:"simulates"`
}

// Rules is where actions run, under `rules:` in module.yaml and action.yaml
// alike, each a list of names in the order they run or stand. An -if answers
// yes or no by itself, an on- is a row offered at that place, and an action's
// own are offer-if and on-failure.
type Rules struct {
	// OfferIf is what a machine has to say yes to for this module to be offered
	// on it at all - the only thing run before a module is opened.
	OfferIf []string `yaml:"offer-if"`

	// StartIf is what has to say yes before the work starts. The first that
	// says no stands a page in front of everything, with what it opens on
	// failure where it has that.
	StartIf []string `yaml:"start-if"`

	// OnSettings is rows on the settings page, under the language the interface
	// is read in: what is changed about this machine for the session rather
	// than answered for the work.
	OnSettings []string `yaml:"on-settings"`

	// OnLeave, OnFailure and OnSuccess are rows too: on the page every way out
	// arrives at, under a run that failed, and under a run that finished.
	OnLeave   []string `yaml:"on-leave"`
	OnFailure []string `yaml:"on-failure"`
	OnSuccess []string `yaml:"on-success"`
}

// ID is the folder this action was read from, which is the name every place
// reaches it by.
func (a *Action) ID() string { return a.id }

// Dir is its own folder, absolute: everything it ships with is in there.
func (a *Action) Dir() string { return a.dir }

// Work is the action.sh it does its work in.
func (a *Action) Work() Script { return a.work }

func (a *Action) Label() string { return i18n.T(a.Title) }
func (a *Action) Help() string  { return i18n.T(a.Description) }

// Refusal is what a no from it means, translated and with the answers filled
// in. Empty where it says nothing about it.
func (a *Action) Refusal(get func(string) string) string {
	return strings.TrimSpace(Expand(i18n.T(a.Error), get))
}

// Reports reports whether it stops on a page of its own once it has run.
func (a *Action) Reports() bool { return a.Report != "" }

// ReportText is that page's words, translated and with the answers filled in:
// the headline first, then whatever else it has to say.
func (a *Action) ReportText(get func(string) string) (headline, body string) {
	text := strings.TrimSpace(Expand(i18n.T(a.Report), get))
	headline, body, _ = strings.Cut(text, "\n\n")
	return headline, strings.TrimSpace(body)
}

// vars is every value this action has: its questions, and the answer its
// report shows.
func (a *Action) vars() []*Variable {
	out := append([]*Variable{}, a.Vars...)
	if a.shown != nil {
		out = append(out, a.shown)
	}
	return out
}

// Action finds one by the name every place reaches it by.
func (s *Module) Action(name string) *Action {
	for _, a := range s.Actions {
		if a.id == name {
			return a
		}
	}
	return nil
}

// Named is the actions a list of names stands for, in its order.
func (s *Module) Named(names []string) []*Action {
	out := make([]*Action, 0, len(names))
	for _, name := range names {
		out = append(out, s.Action(name))
	}
	return out
}

// Declared is every variable this module has a value for: its own questions,
// and then those of its actions.
func (s *Module) Declared() []*Variable {
	out := append([]*Variable{}, s.Vars...)
	for _, a := range s.Actions {
		out = append(out, a.vars()...)
	}
	return out
}

// loadActions reads the actions/ folder in dir, one folder per action. A
// folder with no actions/ has none, which is not an error.
func loadActions(dir string) ([]*Action, error) {
	base := filepath.Join(dir, DirActions)
	entries, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []*Action
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		a, err := loadAction(filepath.Join(base, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("%s/%s: %w", DirActions, entry.Name(), err)
		}
		out = append(out, a)
	}
	return out, nil
}

// loadAction reads one folder: what it is, and the action.sh it does it in.
func loadAction(where string) (*Action, error) {
	var d actionDeclaration
	if err := read(filepath.Join(where, FileAction), &d); err != nil {
		return nil, err
	}
	work := Script(beside(where, FileActionScript))
	if work == "" {
		return nil, fmt.Errorf("no %s here — an action does its work in one", FileActionScript)
	}
	return &Action{
		Title: d.Title, Description: d.Description,
		Rules: d.Rules, Error: d.Error, Vars: d.Variables,
		Report: d.Report, Shows: d.Shows, TTY: d.TTY, Simulates: d.Simulates,
		work: work, id: filepath.Base(where), dir: where,
	}, nil
}

// How an action is run, which decides what it may say.
const (
	// opened is an action somebody opens: a row, a preset, or what another opens on failure.
	opened = iota
	// unasked runs by itself to answer a question, so it has no page.
	unasked
	// gating runs by itself, and a no from it is read by somebody - the page in
	// front of the work, the reason a module is not offered - so it says why.
	gating
)

// gather settles how each of this module's actions is run, the strictest place
// first, and refuses one nothing names: it would never run.
func (s *Module) gather(own []*Action) (map[string]int, error) {
	find := func(name string) *Action {
		if i := slices.IndexFunc(own, func(a *Action) bool { return a.id == name }); i >= 0 {
			return own[i]
		}
		return nil
	}

	runs := map[string]int{}
	var queue []*Action
	place := func(where, key string, names []string, how int) error {
		for _, name := range names {
			a := find(name)
			if a == nil {
				return fmt.Errorf("%s: %s: no such action: %s — an action is a folder under %s/", where, key, name, DirActions)
			}
			was, seen := runs[name]
			runs[name] = max(was, how)
			if !seen {
				queue = append(queue, a)
			}
		}
		return nil
	}

	r := s.Rules
	for _, at := range []struct {
		key   string
		names []string
		how   int
	}{
		{"rules: offer-if", r.OfferIf, gating},
		{"rules: start-if", r.StartIf, gating},
		{"rules: on-settings", r.OnSettings, opened},
		{"rules: on-leave", r.OnLeave, opened},
		{"rules: on-failure", r.OnFailure, opened},
		{"rules: on-success", r.OnSuccess, opened},
	} {
		if err := place(FileModule, at.key, at.names, at.how); err != nil {
			return nil, err
		}
	}
	for _, o := range s.Presets {
		if o.Action == "" {
			continue
		}
		if err := place(FileModule, "presets: "+o.Title, []string{o.Action}, opened); err != nil {
			return nil, err
		}
	}
	for len(queue) > 0 {
		a := queue[0]
		queue = queue[1:]
		where := fmt.Sprintf("%s/%s", DirActions, a.id)
		if err := place(where, "rules: offer-if", a.Rules.OfferIf, unasked); err != nil {
			return nil, err
		}
		if err := place(where, "rules: on-failure", a.Rules.OnFailure, opened); err != nil {
			return nil, err
		}
	}

	for _, a := range own {
		if _, ok := runs[a.id]; !ok {
			return nil, fmt.Errorf("%s/%s: nothing names it, so it never runs — name it under rules: in %s or in another action", DirActions, a.id, FileModule)
		}
	}
	s.Actions = own
	return runs, nil
}

// checkActions settles what each action says against how it is run: one that
// runs by itself shows no page, and one whose no somebody reads says why.
func (s *Module) checkActions(runs map[string]int) error {
	where := func(a *Action) string { return fmt.Sprintf("%s/%s", DirActions, a.id) }
	for _, a := range s.Actions {
		if err := s.checkAction(a, runs[a.id]); err != nil {
			return fmt.Errorf("%s: %w", where(a), err)
		}
	}
	if slices.ContainsFunc(s.Named(s.Rules.OnLeave), func(a *Action) bool { return len(a.Vars) > 0 }) {
		return fmt.Errorf("%s: rules: on-leave: a way out asks nothing, it only goes", FileModule)
	}
	if ring := s.circle(); ring != "" {
		return fmt.Errorf("%s: actions that wait on each other: %s", DirActions, ring)
	}
	// Conditions once every page is known, so one may be guarded by a page of
	// another action.
	for _, a := range s.Actions {
		for _, v := range a.Vars {
			cond, err := s.conditions(v.Conditions)
			if err != nil {
				return fmt.Errorf("%s: %s: %w", where(a), v.Name, err)
			}
			v.cond = cond
		}
	}
	return nil
}

// checkAction refuses what an action cannot mean, given how it is run.
func (s *Module) checkAction(a *Action, how int) error {
	pages := 0
	for _, has := range []bool{len(a.Vars) > 0, a.Reports(), a.TTY} {
		if has {
			pages++
		}
	}
	r := a.Rules
	switch {
	case a.Title == "":
		return fmt.Errorf("title is required")
	case pages > 1:
		return fmt.Errorf("an action has one kind of page: variables, report or tty — a second kind is a second action, named under rules: on-failure")
	case how != opened && pages > 0:
		return fmt.Errorf("it runs by itself where it is named, so it has no page")
	case how == gating && a.Error == "":
		return fmt.Errorf("error: it runs by itself in front of the work, and a no there is read as this sentence")
	case slices.Contains(r.OnFailure, a.id):
		return fmt.Errorf("rules: on-failure: an action cannot put itself right")
	case len(r.StartIf)+len(r.OnSettings)+len(r.OnLeave)+len(r.OnSuccess) > 0:
		return fmt.Errorf("rules: an action's own are offer-if and on-failure - start-if, on-settings, on-leave and on-success place a module's gates and rows, in %s", FileModule)
	}
	for _, v := range a.Vars {
		switch {
		case v.First:
			return fmt.Errorf("%s: first: a page is asked when its action is opened", v.Name)
		case v.Group != "":
			return fmt.Errorf("%s: group: a page is never on the settings page", v.Name)
		case v.Derived():
			return fmt.Errorf("%s: value-from: a page is asked, and a value worked out is not", v.Name)
		case v.Deferred():
			return fmt.Errorf("%s: type: %s is asked by a task mid-run, and a page when its action is opened", v.Name, TypeDeferred)
		}
		if err := s.checkVar(v, a.dir); err != nil {
			return err
		}
	}
	if err := s.checkShown(a); err != nil {
		return err
	}
	if err := s.checkText("error", a.Error); err != nil {
		return err
	}
	return s.checkText("report", a.Report)
}

// checkShown settles a `shows:`, which is an answer put on the page a
// `report:` draws - as a code to scan, and under it as itself. Being named is
// its declaration: the action's script answers it, and nothing else does.
func (s *Module) checkShown(a *Action) error {
	if a.Shows == "" {
		return nil
	}
	if !a.Reports() {
		return fmt.Errorf("shows: there is no report for it to appear on")
	}
	if a.shown == nil {
		a.shown = &Variable{Name: a.Shows, Title: a.Shows, Type: TypeText}
	}
	if err := s.checkVar(a.shown, a.dir); err != nil {
		return fmt.Errorf("shows: %w", err)
	}
	return nil
}

// circle is the first ring of actions that wait on each other through their
// rules, written the way it goes round, or empty where there is none. Such a
// ring would ask one of them forever.
func (s *Module) circle() string {
	// Nothing in the map is a name not walked yet.
	const (
		open = iota + 1
		done
	)
	state := map[string]int{}
	var path []string
	var walk func(name string) string
	walk = func(name string) string {
		switch state[name] {
		case open:
			at := slices.Index(path, name)
			return strings.Join(append(path[at:], name), " → ")
		case done:
			return ""
		}
		state[name] = open
		path = append(path, name)
		a := s.Action(name)
		next := append(append([]string{}, a.Rules.OfferIf...), a.Rules.OnFailure...)
		for _, n := range next {
			if ring := walk(n); ring != "" {
				return ring
			}
		}
		path = path[:len(path)-1]
		state[name] = done
		return ""
	}
	for _, a := range s.Actions {
		if ring := walk(a.id); ring != "" {
			return ring
		}
	}
	return ""
}
