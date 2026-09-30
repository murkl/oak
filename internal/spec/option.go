package spec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/murkl/oak/internal/i18n"
)

// Where an option stands as a row. Left out, it has none, and is opened only on
// the way in — see Option.Start.
const (
	MenuMain  = "main"  // the menu, between the row that starts the work and Settings
	MenuLeave = "leave" // the page every way out of the interface arrives at
)

// Option is something a module offers to be opened rather than run as part of
// its work: a folder under options/, holding what it is and the script that
// does it.
//
// It is the one thing a module can do outside its run, and the runtime knows
// nothing about what any of them is for. Joining a wireless network, switching
// the machine off, checking that it booted the right way — each is an option
// the module wrote, opened from a row or on the way in, asking what its pages
// ask and running what its script runs.
type Option struct {
	Title       string
	Description string

	// Menu is where its row stands: MenuMain, MenuLeave, or nowhere.
	Menu string

	// Requires is shell that says whether this machine has this option at all —
	// a wireless network on a machine with no card is not one. Read the way a
	// module's own is: exit 0 offers it. Empty offers it everywhere.
	Requires Script

	// Start is shell the work waits for. Before anything is started, it has to
	// say yes; while it says no, a page stands in front of everything with what
	// it wrote on stderr, looks again by itself every few seconds, and opens
	// this option where there is one to open. Empty waits for nothing.
	Start Script

	// Vars are the pages it asks when it is opened, in order: questions like
	// the module's own, whose answers are handed to its script and kept for
	// this session only — never written down and never on the settings page.
	Vars []*Variable

	// Work is what it does once its pages are answered. Empty for an option that
	// only has something to wait for.
	Work Script

	// Simulates runs Work under --debug as well, the way it does a task's.
	Simulates bool

	id  string
	dir string
}

// optionDeclaration is option.yaml as it is written.
type optionDeclaration struct {
	Title       string      `yaml:"title"`
	Description string      `yaml:"description"`
	Menu        string      `yaml:"menu"`
	Requires    string      `yaml:"requires"`
	Start       string      `yaml:"start"`
	Variables   []*Variable `yaml:"variables"`
	Script      string      `yaml:"script"`
	Simulates   bool        `yaml:"simulates"`
}

// ID is the folder this option was read from.
func (o *Option) ID() string { return o.id }

// Dir is its own folder, absolute: everything it ships with is in there.
func (o *Option) Dir() string { return o.dir }

func (o *Option) Label() string { return i18n.T(o.Title) }
func (o *Option) Help() string  { return i18n.T(o.Description) }

// Menu is the options that stand as a row on one page, in their order.
func (s *Module) Menu(where string) []*Option {
	var out []*Option
	for _, o := range s.Options {
		if o.Menu == where {
			out = append(out, o)
		}
	}
	return out
}

// Starts is the options the work waits for, in their order.
func (s *Module) Starts() []*Option {
	var out []*Option
	for _, o := range s.Options {
		if !o.Start.Empty() {
			out = append(out, o)
		}
	}
	return out
}

// Declared is every variable this module has a value for: its own questions,
// and then the pages of its options.
func (s *Module) Declared() []*Variable {
	out := append([]*Variable{}, s.Vars...)
	for _, o := range s.Options {
		out = append(out, o.Vars...)
	}
	return out
}

// loadOptions reads options/, one folder per option. A module with no options/
// has none, which is not an error.
func loadOptions(dir string) ([]*Option, error) {
	base := filepath.Join(dir, DirOptions)
	entries, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []*Option
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		o, err := loadOption(filepath.Join(base, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("%s/%s: %w", DirOptions, entry.Name(), err)
		}
		out = append(out, o)
	}
	return out, nil
}

// loadOption reads one folder and settles every piece of shell it names.
func loadOption(where string) (*Option, error) {
	var d optionDeclaration
	if err := read(filepath.Join(where, FileOption), &d); err != nil {
		return nil, err
	}
	o := &Option{
		Title: d.Title, Description: d.Description, Menu: d.Menu,
		Vars: d.Variables, Simulates: d.Simulates,
		id: filepath.Base(where), dir: where,
	}
	var err error
	if o.Requires, err = written(where, "requires", d.Requires); err != nil {
		return nil, err
	}
	if o.Start, err = written(where, "start", d.Start); err != nil {
		return nil, err
	}
	if o.Work, err = pick(where, "script", d.Script, FileOptionScript); err != nil {
		return nil, err
	}
	return o, nil
}

// written settles a key that holds shell or names the file it lives in, with
// nothing beside it to fall back on. Left blank, it holds neither.
func written(dir, key, expr string) (Script, error) {
	file, err := scriptFile(dir, expr)
	switch {
	case err != nil:
		return Script{}, fmt.Errorf("%s: %w", key, err)
	case file != "":
		return Script{File: file}, nil
	case strings.TrimSpace(expr) == "":
		return Script{}, nil
	}
	return Script{Shell: expr}, nil
}

// checkOptions settles every option against the rest of the module: its pages
// are held to the same rules as any question and share the module's names, and
// whatever it says has to be something that can take effect.
func (s *Module) checkOptions() error {
	for _, o := range s.Options {
		if err := s.checkOption(o); err != nil {
			return fmt.Errorf("%s/%s: %w", DirOptions, o.id, err)
		}
	}
	// Conditions once every page is known, so one may be guarded by a page
	// after it.
	for _, o := range s.Options {
		for _, v := range o.Vars {
			cond, err := s.conditions(v.Conditions)
			if err != nil {
				return fmt.Errorf("%s/%s: %s: %w", DirOptions, o.id, v.Name, err)
			}
			v.cond = cond
		}
	}
	return nil
}

// checkOption refuses what an option cannot mean. Each refusal is a line that
// would load and never do anything.
func (s *Module) checkOption(o *Option) error {
	switch {
	case o.Title == "":
		return fmt.Errorf("title is required")
	case o.Menu != "" && o.Menu != MenuMain && o.Menu != MenuLeave:
		return fmt.Errorf("menu: %s or %s, got %q", MenuMain, MenuLeave, o.Menu)
	case o.Menu == "" && o.Start.Empty():
		return fmt.Errorf("nothing opens it: give it a menu, or a start the work waits for")
	case o.Menu != "" && o.Work.Empty():
		return fmt.Errorf("menu: a row has to do something, and there is no %s and no script", FileOptionScript)
	case len(o.Vars) > 0 && o.Work.Empty():
		return fmt.Errorf("variables: nothing is handed the answers, since there is no %s and no script", FileOptionScript)
	case o.Simulates && o.Work.Empty():
		return fmt.Errorf("simulates: there is no script to run")
	case o.Menu == MenuLeave && len(o.Vars) > 0:
		return fmt.Errorf("variables: a way out asks nothing, it only goes")
	case o.Menu == MenuLeave && !o.Start.Empty():
		return fmt.Errorf("start: a way out is nothing the work waits for")
	}
	for _, v := range o.Vars {
		switch {
		case v.First:
			return fmt.Errorf("%s: first: a page is asked when its option opens", v.Name)
		case v.Group != "":
			return fmt.Errorf("%s: group: a page is never on the settings page", v.Name)
		case v.Derived():
			return fmt.Errorf("%s: answer: a page is asked, and an answer worked out is not", v.Name)
		}
		if err := s.checkVar(v, o.dir); err != nil {
			return err
		}
	}
	return nil
}
