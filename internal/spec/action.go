package spec

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/murkl/oak/internal/i18n"
)

// Action is something a module does outside its run: a folder under actions/,
// holding what it is and the script that does it.
//
// It is a script and nothing more. It says yes by exiting 0, and where it says
// no, what it wrote on stderr is why. Where it is run is not the action's
// business but the place that names it: the module's `offered:` and
// `requires:`, its `menu:`, `leave:` and `failure:`, and another action's
// `requires:` and `fallback:`. The runtime knows nothing about what any of
// them is for — joining a wireless network, switching the machine off and
// checking that it booted the right way are all actions a module wrote.
type Action struct {
	Title       string
	Description string

	// Requires is the actions that must say yes before this one is offered at
	// all: a wireless network is not one on a machine with no card.
	Requires []string

	// Fallback is the action offered where this one says no, to put right what
	// it found: no internet, and a wireless network to join.
	Fallback string

	// Vars are the pages it asks before it runs, in order: questions like the
	// module's own, whose answers are handed to its script and kept for this
	// session only — never on the settings page, never in the answer file.
	Vars []*Variable

	// Work is what it does.
	Work Script

	// Confirm, Default, Report and Shows are what they are on a task: a yes or
	// no before it runs, which of the two that opens on, the page it stops on
	// once it has, and the answer drawn on that page as a code.
	Confirm string
	Default Scalar
	Report  string
	Shows   string

	// Simulates runs it under --debug as well, the way it does a task.
	Simulates bool

	id  string
	dir string
}

// actionDeclaration is action.yaml as it is written.
type actionDeclaration struct {
	Title       string      `yaml:"title"`
	Description string      `yaml:"description"`
	Requires    []string    `yaml:"requires"`
	Fallback    string      `yaml:"fallback"`
	Variables   []*Variable `yaml:"variables"`
	Script      string      `yaml:"script"`
	Confirm     string      `yaml:"confirm"`
	Default     Scalar      `yaml:"default"`
	Report      string      `yaml:"report"`
	Shows       string      `yaml:"shows"`
	Simulates   bool        `yaml:"simulates"`
}

// Places is where a module names its actions. Each is a list of their names,
// in the order they are run or stand as rows.
type Places struct {
	// Offered is what a machine has to say yes to for this module to be on
	// offer on it at all — the only thing run before a module is opened.
	Offered []string `yaml:"offered"`

	// Requires is what has to say yes before the work begins. The first that
	// says no stands a page in front of everything, with its fallback to open
	// where it has one.
	Requires []string `yaml:"requires"`

	// Menu, Leave and Failure are rows: on the menu between the work and the
	// settings, on the page every way out arrives at, and on the page a run
	// that failed stops on.
	Menu    []string `yaml:"menu"`
	Leave   []string `yaml:"leave"`
	Failure []string `yaml:"failure"`
}

// ID is the folder this action was read from, which is the name every place
// reaches it by.
func (a *Action) ID() string { return a.id }

// Dir is its own folder, absolute: everything it ships with is in there.
func (a *Action) Dir() string { return a.dir }

func (a *Action) Label() string { return i18n.T(a.Title) }
func (a *Action) Help() string  { return i18n.T(a.Description) }

// Confirms reports whether it asks before it runs, and Declines whether that
// question opens on no.
func (a *Action) Confirms() bool { return a.Confirm != "" }
func (a *Action) Declines() bool { return a.Default == ConfirmNo }

// Question is the offer, translated and with the answers filled in.
func (a *Action) Question(get func(string) string) string {
	return strings.TrimSpace(Expand(i18n.T(a.Confirm), get))
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
// and then the pages of its actions.
func (s *Module) Declared() []*Variable {
	out := append([]*Variable{}, s.Vars...)
	for _, a := range s.Actions {
		out = append(out, a.Vars...)
	}
	return out
}

// loadActions reads actions/, one folder per action. A module with no
// actions/ has none, which is not an error.
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

// loadAction reads one folder: what it is, and the script it runs. An action
// that runs nothing is not one.
func loadAction(where string) (*Action, error) {
	var d actionDeclaration
	if err := read(filepath.Join(where, FileAction), &d); err != nil {
		return nil, err
	}
	work, err := pick(where, "script", d.Script, FileActionScript)
	if err != nil {
		return nil, err
	}
	if work.Empty() {
		return nil, fmt.Errorf("no %s here, and no script in %s", FileActionScript, FileAction)
	}
	return &Action{
		Title: d.Title, Description: d.Description,
		Requires: d.Requires, Fallback: d.Fallback, Vars: d.Variables,
		Work: work, Confirm: d.Confirm, Default: d.Default,
		Report: d.Report, Shows: d.Shows, Simulates: d.Simulates,
		id: filepath.Base(where), dir: where,
	}, nil
}

// checkActions settles the actions against the rest of the module: every name
// a place gives is an action, every action is named somewhere, and what each
// one says can take effect where it is run.
//
// An action is run in one of two ways, and what it may say follows from which.
// One that answers a question by itself — whether this module is on offer,
// whether the work may begin, whether another action is — runs unasked, so it
// asks nothing. One that somebody opens — a row, or the fallback of a check that
// said no — may ask its pages and its yes or no first.
func (s *Module) checkActions() error {
	named := map[string]bool{}
	unasked := map[string]string{}
	place := func(key string, names []string, asked bool) error {
		for _, name := range names {
			if s.Action(name) == nil {
				return fmt.Errorf("%s: no such action: %s — an action is a folder under %s/", key, name, DirActions)
			}
			named[name] = true
			if !asked {
				unasked[name] = key
			}
		}
		return nil
	}
	p := s.Places
	for _, at := range []struct {
		key   string
		names []string
		asked bool
	}{
		{"offered", p.Offered, false},
		{"requires", p.Requires, false},
		{"menu", p.Menu, true},
		{"leave", p.Leave, true},
		{"failure", p.Failure, true},
	} {
		if err := place(at.key, at.names, at.asked); err != nil {
			return fmt.Errorf("%s: %w", FileModule, err)
		}
	}
	for _, a := range s.Actions {
		where := fmt.Sprintf("%s/%s", DirActions, a.id)
		if err := place("requires", a.Requires, false); err != nil {
			return fmt.Errorf("%s: %w", where, err)
		}
		if a.Fallback != "" {
			if err := place("fallback", []string{a.Fallback}, true); err != nil {
				return fmt.Errorf("%s: %w", where, err)
			}
		}
	}
	for _, a := range s.Actions {
		where := fmt.Sprintf("%s/%s", DirActions, a.id)
		if !named[a.id] {
			return fmt.Errorf("%s: nothing names it, so it never runs — name it in %s or in another action", where, FileModule)
		}
		if err := s.checkAction(a, unasked[a.id]); err != nil {
			return fmt.Errorf("%s: %w", where, err)
		}
	}
	if slices.ContainsFunc(p.Leave, func(name string) bool { return len(s.Action(name).Vars) > 0 }) {
		return fmt.Errorf("%s: leave: a way out asks nothing, it only goes", FileModule)
	}
	if ring := s.circle(); ring != "" {
		return fmt.Errorf("%s: actions that wait on each other: %s", DirActions, ring)
	}
	// Conditions once every page is known, so one may be guarded by a page
	// after it.
	for _, a := range s.Actions {
		for _, v := range a.Vars {
			cond, err := s.conditions(v.Conditions)
			if err != nil {
				return fmt.Errorf("%s/%s: %s: %w", DirActions, a.id, v.Name, err)
			}
			v.cond = cond
		}
	}
	return nil
}

// checkAction refuses what an action cannot mean. unasked names the place that
// runs it by itself, empty where only somebody opening it does.
func (s *Module) checkAction(a *Action, unasked string) error {
	switch {
	case a.Title == "":
		return fmt.Errorf("title is required")
	case unasked != "" && len(a.Vars) > 0:
		return fmt.Errorf("variables: it is run unasked where %s names it, so it asks nothing", unasked)
	case unasked != "" && a.Confirms():
		return fmt.Errorf("confirm: it is run unasked where %s names it, so nothing waits for a yes", unasked)
	case a.Fallback == a.id:
		return fmt.Errorf("fallback: an action cannot put itself right")
	}
	for _, v := range a.Vars {
		switch {
		case v.First:
			return fmt.Errorf("%s: first: a page is asked when its action is opened", v.Name)
		case v.Group != "":
			return fmt.Errorf("%s: group: a page is never on the settings page", v.Name)
		case v.Derived():
			return fmt.Errorf("%s: answer: a page is asked, and an answer worked out is not", v.Name)
		}
		if err := s.checkVar(v, a.dir); err != nil {
			return err
		}
	}
	if err := checkOffer(a.Default, a.Confirms()); err != nil {
		return err
	}
	if err := s.checkText("confirm", a.Confirm); err != nil {
		return err
	}
	if err := s.checkText("report", a.Report); err != nil {
		return err
	}
	return s.checkShown(a.Shows, a.Reports())
}

// circle is the first ring of actions that wait on each other through their
// requires and fallbacks, written the way it goes round, or empty where there
// is none. Such a ring would ask one of them forever.
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
		next := append([]string{}, a.Requires...)
		if a.Fallback != "" {
			next = append(next, a.Fallback)
		}
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
