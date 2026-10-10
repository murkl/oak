// Package spec is what a runtime and its modules declare about themselves, read
// into memory. It knows the shape of the yaml, so nothing below it knows
// anything about the system being installed.
package spec

import (
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/murkl/oak/internal/i18n"
)

// What a module is made of, each part found by its own name, so a module turns
// a part off by leaving it out. The work is a folder each under tasks/ and what
// runs outside it a folder each under actions/, kept apart because a task is
// listed, ordered and guarded and an action runs wherever it is named.
const (
	FileModule = "module.yaml" // the declaration: what the module is, asks, and does
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

// Mark is what a stage folder under tasks/ wears, so a when and a what tell
// apart on sight, in a path and in an error. It also keeps a task dropped
// straight into tasks/ from passing for a stage nobody declared.
const Mark = "@"

// Stage is the folder one is kept in: the name module.yaml gave it, marked.
func Stage(name string) string { return Mark + name }

// marked reports whether a folder name carries it.
func marked(name string) bool { return strings.HasPrefix(name, Mark) }

// Script is the file a task or an action works in, beside its yaml, so a
// failure has a line and a linter something to read. Empty where there is none.
type Script string

// Shell is that file as one piece of shell, for the places that only run it.
func (s Script) Shell() string { return source(string(s)) }

// The two names Oak puts into a script's environment, and all it puts there:
// how the run was started and where the answers live. Everything else a script
// is told is an answer under its own name.
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

	// Actions, in the order their folders sort, and the rules that say where
	// the module runs them - see Action.
	Actions []*Action
	Rules   Rules

	// Confirm is whether the work waits for a yes on the last page before it,
	// after every password. The question is the runtime's, the same in every
	// module.
	Confirm bool

	// Asks names deferred questions put every time the work is started, before
	// any password: a choice that only holds for this one run, such as the
	// device about to be written.
	Asks []string

	// Warnings is what loaded but says something that can never take effect. A
	// module that behaves is not a module that refuses to start, so these are
	// reported - by `--inspect`, and in the log when the module is opened -
	// rather than raised.
	Warnings []string

	// Locales is the folder its catalogs live in, or empty where it has none.
	Locales string

	// Shell is the product's one shell, FileRuntimeShell, which every script of
	// every module of it is given first. Empty where the product has none, and
	// for a module loaded on its own rather than as part of one.
	Shell string

	// answered is every name a module of the product declares. The product's
	// shell runs for each of them, so a name it reads is answered where any one
	// of them answers it.
	answered names

	// calls is every function of oak.sh the yaml names, and where it names it,
	// held to what oak.sh defines once the product is known.
	calls map[string]string

	// Status is what the header keeps an eye on while this module is open: its
	// own, or the product's where it declares none. Nil where neither does.
	Status *Status

	// Language names the variable whose answer also settles the words the
	// interface is read in, matched like a machine's locale so de_AT is German.
	// Empty keeps them apart, and the program asks for a language of its own.
	Language string

	byName map[string]*Variable
}

// UI is what a module says about itself, the words that make the frame this
// program rather than the one beside it. Its look belongs to the runtime; see
// Runtime.
type UI struct {
	// Title is the module's one name: the row that opens it, the trail over
	// every page and every sentence about it. The rows inside are named after
	// what they do, since the frame already carries the name.
	Title string

	// Icon stands over the menu's rows, in place of the runtime's: as written,
	// or as a function of oak.sh prints it.
	Icon string
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
func (s *Module) Leaves() bool { return len(s.Rules.OnLeave) > 0 }

// Preset is one starting point on the page a machine with no answer file sees
// first: the values choosing it fills in, and no mode the module stays in. The
// page itself is the runtime's.
type Preset struct {
	Title       string            `yaml:"title"`
	Description string            `yaml:"description"`
	Values      map[string]Scalar `yaml:"values"`

	// Action is opened in place of values, for the starting point that is
	// fetched rather than written out here: a code somebody was handed, and the
	// answers behind it, which its script writes into the answer file.
	Action string `yaml:"action"`
}

// Fetches reports whether choosing this row opens an action.
func (o *Preset) Fetches() bool { return o.Action != "" }

func (o *Preset) Label() string { return i18n.T(o.Title) }
func (o *Preset) Help() string  { return i18n.T(o.Description) }

// Task is one unit of work: a folder under tasks/ with what it is, its task.sh
// and, where there is one, the test.sh that checks the machine afterwards. Its
// stage is the folder it lies in, so adding a folder adds a step.
type Task struct {
	Title string `yaml:"title"`

	Needs      []string   `yaml:"needs"`
	Conditions Conditions `yaml:"conditions"`

	// Asks names a variable whose answer is not knowable before this point: the
	// snapshot to go back to, once the disk holding it is open. The run stops and
	// asks it every time, whatever the answer file says.
	Asks string `yaml:"asks"`

	// Confirm is a yes or no in the frame before this task runs; no skips it
	// and the run carries on. It comes after `asks`, so it can name what was
	// just chosen.
	Confirm string `yaml:"confirm"`

	// YesAfter names tasks of this module that run before this one. Where one
	// of them ran in this run, the confirm opens on Yes: this step is what
	// follows from that one.
	YesAfter []string `yaml:"yes-after"`

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
	// because it reads DEBUG and decides for itself what a simulated run does -
	// one that only reads, say. Every other task is only shown as run.
	Simulates bool `yaml:"simulates"`

	// AllowFailure marks a task the result stands without. Its failure does not
	// stop the run: the row keeps a cross, and what went wrong is counted and
	// read the way a failed test is.
	AllowFailure bool `yaml:"allow-failure"`

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

// The types a variable is declared as, which say what the frame draws and which
// keys apply. A password is asked right before the run and a deferred one by
// the task that names it under `asks:`.
const (
	TypeText        = "text"
	TypeBool        = "bool"
	TypeList        = "list"
	TypeOpenList    = "open-list"
	TypePassword    = "password"
	TypeNewPassword = "new-password"
	TypeDeferred    = "deferred"
)

// Types is every type there is, in the order the reference lists them.
var Types = []string{TypeText, TypeBool, TypeList, TypeOpenList, TypePassword, TypeNewPassword, TypeDeferred}

// TypeKeys are the keys only some types take. One set on a type that does not
// take it is refused, since nothing would read it.
var TypeKeys = map[string][]string{
	TypeText:        {"default", "prefill", "value-from", "pattern"},
	TypeBool:        {"default", "value-from"},
	TypeList:        {"options", "options-from", "filter", "default", "prefill", "pattern"},
	TypeOpenList:    {"options", "options-from", "filter", "default", "prefill", "pattern"},
	TypePassword:    {"check"},
	TypeNewPassword: {},
	TypeDeferred:    {"options", "options-from", "filter"},
}

// The two answers a bool variable has. They are written into the answer file
// and read by scripts as plain shell truth, so they are these words and not
// yes/no - a script tests `[ "$X" = true ]`.
const (
	BoolTrue  = "true"
	BoolFalse = "false"
)

// What a list does with its narrowing box before anything is typed. collapsed
// waits for / and costs the page nothing, open is for a list that would
// otherwise be scrolled through.
const (
	FilterOpen      = "open"
	FilterCollapsed = "collapsed"
)

// Variable is one thing a module needs to know, and everything known
// about what a valid answer looks like. The rules live here once and are used
// both when asking and when reading back an answer file somebody edited by
// hand - a value typed into the file never passed a prompt.
type Variable struct {
	Name        string `yaml:"name"`
	Title       string `yaml:"title"`
	Description string `yaml:"description"`

	// Group is the heading this row sits under on the settings page. Rows keep
	// the order they were declared in, so a group is simply the run of rows
	// that named it - there is nothing to declare up front.
	Group string `yaml:"group"`

	// Type is one of Types, and required. A password exists already and is
	// typed once; a new-password is chosen and typed twice, since nothing can
	// check it before the first boot.
	Type     string `yaml:"type"`
	Default  Scalar `yaml:"default"`
	Required bool   `yaml:"required"`

	// Check tries a password on what it opens before it is taken, refusing it
	// on its page with a non-zero exit. Every other answer is held to its
	// pattern.
	Check string `yaml:"check"`

	// First asks this question before anything else, for the answer everything
	// after is typed on, such as the keyboard, and should be promised
	// sparingly. Its list's narrowing box is open from the start, since the key
	// to open it is typed on a layout nobody chose yet.
	First bool `yaml:"first"`

	// Where a list's answers come from: written out, or printed one per line by
	// shell.
	Options     []string `yaml:"options"`
	OptionsFrom string   `yaml:"options-from"`

	// Filter is what this list does with its narrowing box (see FilterMode),
	// declared rather than counted from rows. A list is thirty long on one
	// machine and three on the next, and a page that changed with it could not
	// be told in advance.
	Filter string `yaml:"filter"`

	// Prefill prints a suggested answer - a timezone guessed from the network,
	// a keymap read off the live system. Only ever a suggestion: it fills the
	// box, it does not answer the question.
	Prefill string `yaml:"prefill"`

	// ValueFrom is shell that prints the value the machine can see, read when
	// the module opens and whenever an answer changes. Such a variable is never
	// asked, shown or stored, since a stored copy could only disagree.
	ValueFrom string `yaml:"value-from"`

	// Apply puts this answer into effect on the machine Oak runs on, such as a
	// console keyboard every later answer is typed on. It runs when the answer
	// is given and once at startup, asking again where it fails, since nobody
	// watched it then.
	Apply string `yaml:"apply"`

	Pattern    string     `yaml:"pattern"`
	Error      string     `yaml:"error"`
	Conditions Conditions `yaml:"conditions"`

	re   *regexp.Regexp
	cond []*condition
}

// typedKeys are the keys of TypeKeys this question sets.
func (v *Variable) typedKeys() []string {
	var keys []string
	for _, k := range []struct {
		key string
		set bool
	}{
		{"options", len(v.Options) > 0}, {"options-from", v.OptionsFrom != ""}, {"filter", v.Filter != ""},
		{"default", v.Default != ""}, {"prefill", v.Prefill != ""}, {"value-from", v.ValueFrom != ""},
		{"pattern", v.Pattern != ""}, {"check", v.Check != ""},
	} {
		if k.set {
			keys = append(keys, k.key)
		}
	}
	return keys
}

// Deferred reports whether this value is one the opening run of questions has
// no business asking: a task asks it mid-run, or the module as the work is
// started, both under `asks:`. A snapshot to go back to cannot be chosen, or
// shown on a settings page, while the disk holding it is still locked. Its
// answer holds for one run, so it is never written down.
func (v *Variable) Deferred() bool { return v.Type == TypeDeferred }

// Derived reports whether this value is read off the machine rather than asked
// for. Like a deferred one it is not a question, and for the mirror reason:
// there is nothing here for anybody to decide.
func (v *Variable) Derived() bool { return v.ValueFrom != "" }

func (v *Variable) Label() string { return i18n.T(v.Title) }
func (v *Variable) Help() string  { return i18n.T(v.Description) }

// Open reports whether this list also takes an answer typed in: the list only
// suggests.
func (v *Variable) Open() bool { return v.Type == TypeOpenList }

// Listed reports whether the answer is chosen from a list.
func (v *Variable) Listed() bool {
	return v.Type == TypeList || v.Type == TypeOpenList || v.Type == TypeDeferred
}
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

// Secret reports whether this answer is never written down: either kind of
// password. It is asked for immediately before the run that needs it, kept in
// memory for that run, and forgotten - so it is also the one required variable
// that does not stop the program from being ready.
func (v *Variable) Secret() bool { return v.Type == TypePassword || v.Type == TypeNewPassword }

// Repeats reports whether this password is typed twice to catch a typo in it:
// one being chosen, never one that already exists and is only handed over.
func (v *Variable) Repeats() bool { return v.Type == TypeNewPassword }

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
// open - a name typed into a box, a list a command prints. It is what makes a
// guard something that can be reasoned about rather than only evaluated.
func (v *Variable) domain() []string {
	switch {
	case len(v.Options) > 0:
		return v.Options
	case v.Type == TypeBool:
		return []string{BoolTrue, BoolFalse}
	}
	return nil
}

// ID is the folder this module was read from. It is what the page offering it
// is keyed on, the word that opens it from the command line, and the name its
// answers and its log are kept under - one identity, so there is nothing to
// keep in step.
func (s *Module) ID() string { return filepath.Base(s.Dir) }

// Var finds a variable by name.
func (s *Module) Var(name string) *Variable { return s.byName[name] }

// Name is the module's own title, translated: what it is called.
func (s *Module) Name() string { return i18n.T(s.UI.Title) }

// Message is one thing a module says: the text, what it is, and the files it
// was read out of. The last two are all a translator has - the words arrive out
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
	for _, o := range s.Presets {
		add(decl, "a starting point: its row", o.Title)
		add(decl, "starting point "+o.Title+": what choosing it does", o.Description)
	}
	for _, v := range s.Vars {
		add(decl, v.Name+": the question", v.Title)
		add(decl, v.Name+": what it means", v.Description)
		add(decl, v.Name+": the heading its row sits under", v.Group)
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
		file := path.Join(DirActions, a.ID(), FileAction)
		add(file, "an action: its row, and the heading over its page", a.Title)
		add(file, "an action: what it does, under its row", a.Description)
		add(file, "an action: what a no from it means", a.Error)
		add(file, "asked before the action does its work", a.Confirm)
		add(file, "read once the action is done, and held on until somebody has", a.Report)
		for _, v := range a.Vars {
			add(file, v.Name+": a page of the action", v.Title)
			add(file, v.Name+": what it means", v.Description)
			add(file, v.Name+": what a wrong answer is told", v.Error)
		}
	}
	return out
}
