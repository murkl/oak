// Package spec is what a runtime and its modules declare about themselves,
// read into memory: what the runtime is called and which modules it offers,
// and for each module what it needs to know and what it does.
//
// A module is one yaml and the folders beside it. Everything in it is data. The
// runtime ships none of its own — without a module there is nothing to run,
// only a binary that says so and stops. That is the whole point of the split:
// this package knows the shape of the yaml, and nothing in the program below it
// knows a single thing about the system being installed.
package spec

import (
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/murkl/oak/internal/i18n"
)

// What a module is made of. Only the declaration has to be there; everything
// else is found by its own name, so a module turns a part of the program off by
// leaving the file or folder out rather than by declaring anything.
//
// One folder holds one module, so every part of it has the name it has here.
// Nothing is configured and nothing points at anything: module.yaml is the
// module, module.sh is the shell it puts in front of everything it runs, the
// work is a folder each under tasks/, and what it does outside that work is a
// folder each under actions/.
//
// The two halves are kept apart because they are answerable to different
// things: a task is the module's own work, listed and ordered and guarded,
// while an action runs wherever the module names it — before the work, from a
// row, to put right what another said no to. Every file inside says which of
// the two it is, so nothing is read as the other.
const (
	FileModule = "module.yaml" // the declaration: what the module is, asks, and does
	FileShell  = "module.sh"   // shell put in front of every script this module runs
	DirTasks   = "tasks"       // the work, one folder per task
	DirActions = "actions"     // what it does outside the work, one folder per action
	DirLocales = "locales"     // one catalog per language the module speaks

	FileTask       = "task.yaml" // what a task is
	FileTaskScript = "task.sh"   // what it does
	FileTest       = "test.sh"   // how the machine is checked once it has, where there is one

	FileAction       = "action.yaml" // what an action is
	FileActionScript = "action.sh"   // what it does

	ScriptExt = ".sh"
)

// Mark is what a stage folder under tasks/ wears: it stands for a moment of the
// run rather than for a piece of work, and it holds the folders that are the
// work.
//
// So the two levels can be told apart on sight, in a path and in an error: a
// folder with the mark is a when, a folder without is a what. It also keeps the
// one mistake that would otherwise load and do nothing — a task dropped
// straight into tasks/ — from passing for a stage nobody declared.
const Mark = "@"

// Stage is the folder one is kept in: the name module.yaml gave it, marked.
func Stage(name string) string { return Mark + name }

// marked reports whether a folder name carries it.
func marked(name string) bool { return strings.HasPrefix(name, Mark) }

// Script is the file a task or an action does its work in, beside its yaml.
// Always a file, so a failure always has a line to point at and a linter
// always has something to read. Empty where there is none.
type Script string

// Shell is that file as one piece of shell, for the places that only run it.
func (s Script) Shell() string { return source(string(s)) }

// The two names Oak puts into a script's environment, and the whole of what it
// puts there. Neither is something a module's data can own: whether a run only
// pretends to work is how it was started rather than something it was told, and
// where the answers live is settled by whoever started the program.
//
// Everything else a script needs it works out for itself. A module's own folder
// is where its module.sh was sourced from, and what a script is told is every
// answer under its own name.
const (
	DebugVar = "DEBUG"       // true under --debug, absent otherwise
	ConfVar  = "MODULE_CONF" // the answer file, which is also how a script answers
)

// runtimeVar reports whether a name is the runtime's, and so not a module's to
// declare.
func runtimeVar(name string) bool { return name == DebugVar || name == ConfVar }

// Module is one whole program the runtime can run: everything one folder
// beside the binary declares about itself.
type Module struct {
	Dir string // absolute, and never written to

	UI      UI
	Presets []*Preset
	Vars    []*Variable

	// Stages are the phases the work happens in, in the order they happen. Each
	// is a folder under tasks/, and the steps in it are what puts them in the
	// run and where.
	Stages []string

	// Tasks, already in the order they run: by stage, and inside a stage by what
	// they declared they need. Sorted once when the module is loaded, so there is
	// one order and everything downstream reads it rather than works it out
	// again.
	Tasks []*Task

	// Actions, in the order their folders sort, and where the module names
	// them — see Action.
	Actions []*Action
	Places  Places

	// Warnings is what loaded but says something that can never take effect. A
	// module that behaves is not a module that refuses to start, so these are
	// reported — by `--inspect`, and in the log when the module is opened —
	// rather than raised.
	Warnings []string

	// Shell is what every script of this module is given before its own, and
	// Locales the folder its catalogs live in. Both are whatever FileShell and
	// DirLocales turned out to be, or empty where the module has neither.
	Shell   string
	Locales string

	// Shared is the product's own shell, FileRuntimeShell, which every module
	// of it is given in front of its own. Empty where the product has none, and
	// for a module loaded on its own rather than as part of one.
	Shared string

	// Status is what the header keeps an eye on while this module is open: its
	// own, or the product's where it declares none. Nil where neither does.
	Status *Status

	// Language names the variable whose answer also settles the words this
	// interface is read in — a module that asks where a machine is has asked which
	// language it speaks, and asking again would be the same question twice. The
	// answer is matched against the catalogs on offer the way a machine's own
	// locale is, so de_AT is German without anything having to say so.
	//
	// Empty leaves the two apart, and the program asks for a language of its own
	// on the way in.
	Language string

	byName map[string]*Variable
}

// UI is what a module says about itself: the words that make the frame this
// program rather than the one beside it. What they all look like is not here —
// one wordmark and one colour belong to the runtime, not to any module in it.
// See Runtime.
type UI struct {
	// Title is what this module is called, and the only name it has: the row
	// that opens it, the trail across the top of every page once it is open,
	// and every sentence the interface writes about it.
	//
	// One name. The frame carries it on every page, so the rows inside a
	// module are named after what they do — "Start", "Settings" — rather than
	// after the module all over again.
	Title string

	// Description is what this module is, in one sentence, read on its menu
	// under the row that starts the work.
	Description string

	// Start is what starting the work is called — "Install", "Repair" — on the
	// first row of the menu and on the button of the page before the run. A verb
	// rather than a second name: the title already stands over both. Empty
	// leaves the runtime's own word.
	Start string
}

// Help is what this module is, in one sentence: the line under the row that
// starts its work.
func (s *Module) Help() string { return i18n.T(s.UI.Description) }

// Start is what starting the work is called, translated. Empty where the
// module leaves it to the runtime.
func (s *Module) Start() string { return i18n.T(s.UI.Start) }

// Shells is everything loaded in front of a script of this module, in the
// order it is loaded: the product's shell, then the module's own.
func (s *Module) Shells() []string {
	var out []string
	for _, path := range []string{s.Shared, s.Shell} {
		if path != "" {
			out = append(out, path)
		}
	}
	return out
}

// Checks reports whether anything in this module says how to tell that it
// worked. A module with nothing to check is never offered the setting that
// turns checking off, and never stops on the page that reports on it.
func (s *Module) Checks() bool {
	for _, t := range s.Tasks {
		if t.Checks() {
			return true
		}
	}
	return false
}

// source is the shell that runs a script file.
func source(path string) string { return "source " + quote(path) }

// Leaves reports whether this machine can be left at all: a module that says
// how is saying the machine booted to run it, so every way out of the interface
// asks what to do with the machine instead of quitting.
func (s *Module) Leaves() bool { return len(s.Places.Leave) > 0 }

// Preset is one page of starting points: a question a machine with no answer
// file is asked before the real ones, answered by choosing one of the options
// under it. It is the only place a value arrives without being typed.
//
// A module may declare several, each a page of its own, asked in the order
// they are declared.
type Preset struct {
	Title       string          `yaml:"title"`
	Description string          `yaml:"description"`
	Options     []*PresetOption `yaml:"options"`
}

// PresetOption is one answer to that question: the values choosing it fills in.
// Nothing else about it survives being chosen — it is a set of answers, not a
// mode the module stays in.
type PresetOption struct {
	Title       string            `yaml:"title"`
	Description string            `yaml:"description"`
	Values      map[string]Scalar `yaml:"values"`

	// Action is opened in place of values, for the starting point that is
	// fetched rather than written out here: a code somebody was handed, and the
	// answers behind it, which its script writes into the answer file.
	Action string `yaml:"action"`
}

// Fetches reports whether choosing this row opens an action.
func (o *PresetOption) Fetches() bool { return o.Action != "" }

func (p *Preset) Label() string { return i18n.T(p.Title) }
func (p *Preset) Help() string  { return i18n.T(p.Description) }

func (o *PresetOption) Label() string { return i18n.T(o.Title) }
func (o *PresetOption) Help() string  { return i18n.T(o.Description) }

// Task is one unit of work: a folder under tasks/, holding what it is, the
// task.sh that does it, and — where there is one — the test.sh that checks the
// machine afterwards.
//
// Which phase it belongs to is the stage folder it lies in, and what it needs
// from that same stage is all it says about when it runs. Nothing keeps a list
// of the installation's steps: adding a folder adds a step.
type Task struct {
	Title string `yaml:"title"`

	Needs      []string   `yaml:"needs"`
	Conditions Conditions `yaml:"conditions"`

	// Asks names a variable whose answer is not knowable before this point: the
	// snapshot to go back to, once the disk holding it is open. The run stops and
	// asks it every time, whatever the answer file says.
	Asks string `yaml:"asks"`

	// Confirm is asked before this one runs, as a yes or no in the frame.
	// Declining skips it and the run carries on. It comes after `asks`, so the
	// offer can name what was just chosen.
	Confirm string `yaml:"confirm"`

	// Report is what the run stops to say once this one has run: the milestone
	// somebody watching a list of task names has no other way of recognising.
	// {{VAR}} is filled in from the answers, and the first paragraph is the
	// headline.
	Report string `yaml:"report"`

	// Progress marks a task whose output is its progress: the one line it drew
	// last is shown under its name while it runs. Everything any other task
	// prints stays in the log.
	Progress bool `yaml:"progress"`

	// Simulates marks a task that is run under --debug as well, test and all,
	// because it reads DEBUG and decides for itself what a simulated run does —
	// one that only reads, say. Every other task is only shown as run.
	Simulates bool `yaml:"simulates"`

	// Optional marks a task the result stands without. Its failure does not
	// stop the run: the row keeps a cross, and what went wrong is counted and
	// read the way a failed test is.
	Optional bool `yaml:"optional"`

	id    string
	stage string
	dir   string
	work  Script
	check Script
	cond  []*condition
}

func (t *Task) Label() string { return i18n.T(t.Title) }

// ID is the folder this task was read from, which is also the name other tasks
// in the same stage reach it by in their needs.
func (t *Task) ID() string { return t.id }

// Stage is the phase of the run this task belongs to: the folder it was found
// in, without the mark.
func (t *Task) Stage() string { return t.stage }

// Dir is its own folder, absolute: everything it ships with is in there.
func (t *Task) Dir() string { return t.dir }

// Work is what this task does, and Check how the machine is looked at once it
// has. Check is empty where the task has no test.sh.
func (t *Task) Work() Script  { return t.work }
func (t *Task) Check() Script { return t.check }

// Checks reports whether this task says how to tell that it worked.
func (t *Task) Checks() bool { return t.check != "" }

// Confirms reports whether this one is offered rather than simply run.
func (t *Task) Confirms() bool { return t.Confirm != "" }

// Question is the offer, translated and with the answers filled in.
func (t *Task) Question(get func(string) string) string {
	return strings.TrimSpace(Expand(i18n.T(t.Confirm), get))
}

// Reports reports whether the run stops on a page of its own once this one has
// run.
func (t *Task) Reports() bool { return t.Report != "" }

// ReportText is that page's words, translated and with the answers filled in:
// the headline first, then whatever else it has to say.
func (t *Task) ReportText(get func(string) string) (headline, body string) {
	text := strings.TrimSpace(Expand(i18n.T(t.Report), get))
	headline, body, _ = strings.Cut(text, "\n\n")
	return headline, strings.TrimSpace(body)
}

// The shapes a variable takes. The type is what the frame draws; a set of
// values or a command turns the default text box into a list without anything
// having to say so.
const (
	TypeText   = "text"
	TypeBool   = "bool"
	TypeSecret = "secret"
)

// The two answers a bool variable has. They are written into the answer file
// and read by scripts as plain shell truth, so they are these words and not
// yes/no — a script tests `[ "$X" = true ]`.
const (
	BoolTrue  = "true"
	BoolFalse = "false"
)

// What a question's list does with the narrowing box before anything is typed
// into it. collapsed is what a question that says nothing gets: the box waits
// for /, which costs the page nothing until somebody wants it. open is for the
// list that would otherwise have to be scrolled through to find a row.
const (
	FilterOpen      = "open"
	FilterCollapsed = "collapsed"
)

// Variable is one thing a module needs to know, and everything known
// about what a valid answer looks like. The rules live here once and are used
// both when asking and when reading back an answer file somebody edited by
// hand — a value typed into the file never passed a prompt.
type Variable struct {
	Name        string `yaml:"name"`
	Title       string `yaml:"title"`
	Description string `yaml:"description"`

	// Group is the heading this row sits under on the settings page. Rows keep
	// the order they were declared in, so a group is simply the run of rows
	// that named it — there is nothing to declare up front.
	Group string `yaml:"group"`

	Type     string `yaml:"type"`
	Default  Scalar `yaml:"default"`
	Required bool   `yaml:"required"`

	// Existing marks a secret that is not chosen here but entered: the disk
	// already has this password, and whatever it is handed to refuses it within
	// seconds. That one is asked once.
	//
	// The repeat everywhere else is not a setting either, and this is not a way
	// to turn it off. A password being chosen is checked by nothing — a typo in
	// it is found at the first boot of a system that took twenty minutes to
	// build — and four seconds against that is no trade. Typing a password
	// twice to open something that would have said no is.
	Existing bool `yaml:"existing"`

	// Check looks at a secret before it is taken, with the value under its own
	// name like every answer a script is handed: an existing password, tried on
	// the thing it opens. A non-zero exit refuses it on the page it was typed
	// on, in the words the shell said on stderr — so a typo costs a second go at
	// the box rather than a run that stops halfway on the step that needed it.
	//
	// Only a secret has one. Every other answer is on the settings page to be
	// read again and held to its pattern, while a secret is typed once, right
	// before the run, and gone afterwards.
	Check string `yaml:"check"`

	// First puts this question before everything else the program does — before
	// the network page, before the module's own check of the machine, before the
	// starting point is chosen. For the answer that everything after it is typed
	// on: a wireless passphrase given on a keyboard nobody chose is not the
	// passphrase, and there is nothing to be done about that afterwards.
	//
	// It is a promise a module should make sparingly. Every question here is a
	// question asked before the check that says this machine cannot be installed
	// onto at all.
	//
	// A list asked this early carries its narrowing box open from the first
	// frame and cannot close it, because the key that would open one is typed
	// on a layout nobody has chosen yet — a box nobody can find the key to open
	// is no box at all. Nothing declares that: being asked first is the
	// declaration.
	First bool `yaml:"first"`

	// Where the answers come from, when there is a set of them: written out, or
	// printed by a command one per line. A variable with neither is free text.
	Values  []string `yaml:"values"`
	Command string   `yaml:"command"`

	// Filter is what this question's list does with the narrowing box — see
	// FilterMode. Nothing counts rows for it: a list is thirty long on one
	// machine and three on the next — the variants of a keyboard layout, the
	// disks in a case — and a page that changed shape with that would be two
	// pages nobody can be told apart in advance. It is declared here, once, and
	// holds wherever the module runs.
	Filter string `yaml:"filter"`

	// Free is the row that opens a text box under a list of answers, for the
	// variable whose list is a suggestion rather than the whole set. Its text is
	// the row's own label; empty offers no such row, which is what a closed set
	// wants.
	Free string `yaml:"free"`

	// Prefill prints a suggested answer — a timezone guessed from the network,
	// a keymap read off the live system. Only ever a suggestion: it fills the
	// box, it does not answer the question.
	Prefill string `yaml:"prefill"`

	// Answer is shell that works the value out instead of asking for it, for
	// the question a machine can see the answer to: whether the disk in front
	// of it is encrypted is a fact, not an opinion. It prints the answer, and
	// printing nothing leaves the value empty.
	//
	// It is read when the module opens and again whenever an answer changes, so
	// a value worked out from another answer follows it. Such a variable is
	// never asked, never on the settings page and never written to the answer
	// file: it is read off the machine every run, and a stored copy could only
	// disagree with it.
	Answer string `yaml:"answer"`

	// Apply puts this answer into effect on the machine the runtime is running
	// on, rather than on the one being installed. Almost nothing needs it — an
	// answer is a string a script reads later — but a console keyboard is not a
	// string: until it is loaded, every answer after it is typed on a layout
	// nobody chose. It runs when the answer is given, and once at startup for an
	// answer this run began with — which is asked again where it fails, since
	// nobody watched it being put in force.
	Apply string `yaml:"apply"`

	Pattern    string     `yaml:"pattern"`
	Error      string     `yaml:"error"`
	Conditions Conditions `yaml:"conditions"`

	re       *regexp.Regexp
	cond     []*condition
	deferred bool
}

// Deferred reports whether this value is one the opening run of questions has
// no business asking. Nothing declares it: being named by a task's `asks:` is
// the declaration. A snapshot to go back to cannot be chosen, or shown on a
// settings page, while the disk holding it is still locked.
func (v *Variable) Deferred() bool { return v.deferred }

// Derived reports whether this value is read off the machine rather than asked
// for. Like a deferred one it is not a question, and for the mirror reason:
// there is nothing here for anybody to decide.
func (v *Variable) Derived() bool { return v.Answer != "" }

func (v *Variable) Label() string { return i18n.T(v.Title) }
func (v *Variable) Help() string  { return i18n.T(v.Description) }

// FreeLabel is the row that opens a text box under a list of answers.
func (v *Variable) FreeLabel() string { return i18n.T(v.Free) }
func (v *Variable) GroupLabel() string {
	if v.Group == "" {
		return ""
	}
	return i18n.T(v.Group)
}

// Why is what to say about an answer that will not do. A variable that declares
// nothing gets a sentence naming the rule it broke, so a module is never obliged
// to write one out for every field.
func (v *Variable) Why() string {
	if v.Error != "" {
		return i18n.T(v.Error)
	}
	if v.Pattern != "" {
		return i18n.T("This value has the wrong format.")
	}
	return i18n.T("This value is required.")
}

// WhyUnoffered is what an answer is told that its list no longer offers: a
// disk that is not in this machine, a keymap this system does not have. The
// module's own words where it wrote any, since it knows what its list is of.
func (v *Variable) WhyUnoffered() string {
	if v.Error != "" {
		return i18n.T(v.Error)
	}
	return i18n.T("This answer is not among the ones offered here.")
}

// WhyRefused is what a secret is told that its check turned away: the module's
// own words where it wrote any. Never what the check printed, which is a tool
// talking to a log in whatever language it was built in.
func (v *Variable) WhyRefused() string {
	if v.Error != "" {
		return i18n.T(v.Error)
	}
	return i18n.T("This password was not accepted.")
}

// Shape is the type with the empty default filled in, so everything else can
// switch on exactly three values.
func (v *Variable) Shape() string {
	if v.Type == "" {
		return TypeText
	}
	return v.Type
}

// Secret reports whether this answer is never written down. It is asked for
// immediately before the run that needs it, kept in memory for that run, and
// forgotten — so it is also the one required variable that does not stop the
// program from being ready.
func (v *Variable) Secret() bool { return v.Shape() == TypeSecret }

// Repeats reports whether this secret is typed twice to catch a typo in it.
// Every one that is being chosen; none that already exists somewhere and is
// only being handed over.
func (v *Variable) Repeats() bool { return v.Secret() && !v.Existing }

// FilterMode is what this question's list does with its narrowing box, with the
// default filled in. A question asked first is open whatever it says: the key
// that would open one is typed on a layout nobody has chosen yet.
func (v *Variable) FilterMode() string {
	switch {
	case v.First:
		return FilterOpen
	case v.Filter == "":
		return FilterCollapsed
	}
	return v.Filter
}

// Matches reports whether s satisfies the declared pattern. No pattern accepts
// anything.
func (v *Variable) Matches(s string) bool { return v.re == nil || v.re.MatchString(s) }

// domain is every answer this question has, or nil where the module left that
// open — a name typed into a box, a list a command prints. It is what makes a
// guard something that can be reasoned about rather than only evaluated.
func (v *Variable) domain() []string {
	switch {
	case len(v.Values) > 0:
		return v.Values
	case v.Shape() == TypeBool:
		return []string{BoolTrue, BoolFalse}
	}
	return nil
}

// ID is the folder this module was read from. It is what the page offering it
// is keyed on, the word that opens it from the command line, and the name its
// answers and its log are kept under — one identity, so there is nothing to
// keep in step.
func (s *Module) ID() string { return filepath.Base(s.Dir) }

// Var finds a variable by name.
func (s *Module) Var(name string) *Variable { return s.byName[name] }

// Name is the module's own title, translated: what it is called.
func (s *Module) Name() string { return i18n.T(s.UI.Title) }

// rel is a path as this module's folder names it: a translator's template
// names every file a string came out of from there, the product's included.
func (s *Module) rel(file string) string {
	if from, err := filepath.Rel(s.Dir, file); err == nil {
		return filepath.ToSlash(from)
	}
	return file
}

// Message is one thing a module says: the text, what it is, and the files it
// was read out of. The last two are all a translator has — the words arrive out
// of the module they belong to, one sentence at a time.
type Message struct {
	Text  string
	Note  string
	Files []string
}

// Messages is every word this module says, in the order it says them, with
// duplicates dropped.
//
// It is the list a translator works from, and the reason there is no separate
// file listing what needs translating: the strings are the yaml's own, and
// asking the loaded module for them means the list can never fall behind it.
func (s *Module) Messages() []Message {
	var out []Message
	at := map[string]int{}
	add := func(file, note, text string) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		// The same sentence in two places is one message said twice, and both
		// places are worth naming.
		if i, ok := at[text]; ok {
			if !slices.Contains(out[i].Files, file) {
				out[i].Files = append(out[i].Files, file)
			}
			return
		}
		at[text] = len(out)
		out = append(out, Message{Text: text, Note: note, Files: []string{file}})
	}

	decl := FileModule
	add(decl, "what this module is called, wherever the interface names it", s.UI.Title)
	add(decl, "what it is, in one sentence, under the row that starts the work", s.UI.Description)
	add(decl, "the row that starts the work, and the button on the page before it", s.UI.Start)
	for _, p := range s.Presets {
		add(decl, "a starting point: the question", p.Title)
		add(decl, "starting point "+p.Title+": what it means", p.Description)
		for _, o := range p.Options {
			add(decl, "starting point "+p.Title+": a row", o.Title)
			add(decl, "starting point "+p.Title+", "+o.Title+": what choosing it does", o.Description)
		}
	}
	for _, v := range s.Vars {
		add(decl, v.Name+": the question", v.Title)
		add(decl, v.Name+": what it means", v.Description)
		add(decl, v.Name+": the heading its row sits under", v.Group)
		add(decl, v.Name+": the row that opens a box for an answer of one's own", v.Free)
		add(decl, v.Name+": what a wrong answer is told", v.Error)
	}
	if st := s.Status; st != nil {
		add(st.file, "the header's status, while its check says yes", st.Pass)
		add(st.file, "the header's status, while its check says no", st.Fail)
	}
	for _, t := range s.Tasks {
		file := path.Join(DirTasks, Stage(t.Stage()), t.ID(), FileTask)
		add(file, "the step, as the run lists it", t.Title)
		add(file, "asked before the step runs", t.Confirm)
		add(file, "read once the step is done, and held on until somebody has", t.Report)
	}
	for _, a := range s.Actions {
		file := s.rel(filepath.Join(a.Dir(), FileAction))
		add(file, "an action: its row, and the heading over its page", a.Title)
		add(file, "an action: what it does, under its row", a.Description)
		add(file, "an action: what a no from it means", a.Fail)
		add(file, "read once the action is done, and held on until somebody has", a.Report)
		if v := a.Var; v != nil {
			add(file, v.Name+": the page of the action", v.Title)
			add(file, v.Name+": what it means", v.Description)
			add(file, v.Name+": the row that opens a box for an answer of one's own", v.Free)
			add(file, v.Name+": what a wrong answer is told", v.Error)
		}
	}
	return out
}
