package spec

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// binaryDir is where this program's own file is, with symlinks resolved so that
// a link on the path still finds the modules the binary was installed with.
func binaryDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return filepath.Dir(exe)
}

// declaration is a module's yaml as it is written: flat, because every key in
// it is about the module as a whole and a nesting level would only be there to
// be typed - but for the subjects with parts of their own: the words of its
// pages, the rules that run its actions, and the header's status.
type declaration struct {
	Title    string `yaml:"title"`
	Language string `yaml:"language"`

	// The phases its work happens in, what it asks each time it starts, and
	// whether it waits for a yes first.
	Stages  []string `yaml:"stages"`
	Asks    []string `yaml:"asks"`
	Confirm bool     `yaml:"confirm"`

	// The picture over its menu.
	Icon string `yaml:"icon"`

	// Where it runs its actions, each a list of their names - see Rules.
	Rules Rules `yaml:"rules"`

	Presets   []*Preset   `yaml:"presets"`
	Variables []*Variable `yaml:"variables"`

	// The header's line for this module, in place of the product's.
	Status *Status `yaml:"status"`
}

// Load reads one module folder and checks it over - every reference resolved,
// every script found, every condition naming a variable that exists, every task
// in a stage that exists and in an order that can be walked.
//
// A module that loads is a module that runs: an authoring mistake is a message
// at startup, never a task that silently never fires.
func Load(dir string) (*Module, error) {
	s := &Module{Dir: dir, byName: map[string]*Variable{}, calls: map[string]string{}}

	var head declaration
	if err := read(filepath.Join(dir, FileModule), &head); err != nil {
		return nil, err
	}
	s.UI = UI{Title: head.Title, Icon: head.Icon}
	s.Presets, s.Vars, s.Language = head.Presets, head.Variables, head.Language
	s.Stages, s.Rules, s.Confirm, s.Asks = head.Stages, head.Rules, head.Confirm, head.Asks
	if err := head.Status.settle(dir, FileModule); err != nil {
		return nil, fmt.Errorf("%s: %w", FileModule, err)
	}
	if head.Status != nil && head.Status.calls != "" {
		s.calls[head.Status.calls] = FileModule + ": status: check"
	}
	s.Status = head.Status
	if err := checkStages(s.Stages); err != nil {
		return nil, fmt.Errorf("%s: %w", FileModule, err)
	}
	s.Locales = beside(dir, DirLocales)
	for _, old := range slices.Sorted(maps.Keys(retiredParts)) {
		if beside(dir, strings.TrimSuffix(old, "/")) != "" {
			return nil, fmt.Errorf("%s: %s", old, retiredParts[old])
		}
	}

	tasks, err := loadTasks(dir, s.Stages)
	if err != nil {
		return nil, err
	}
	own, err := loadActions(dir)
	if err != nil {
		return nil, err
	}
	runs, err := s.gather(own)
	if err != nil {
		return nil, err
	}
	if err := s.check(tasks, runs); err != nil {
		return nil, err
	}
	return s, nil
}

// retiredParts is a file or folder a module used to be able to hold, and what
// to make of it instead. It is refused rather than passed over, so a module
// written for an older Oak is told what to do instead of losing what was in it
// without a word.
var retiredParts = map[string]string{
	"hooks/":    "the runtime runs no hooks - each is an action now, a folder under actions/ that module.yaml names under rules:",
	"options/":  "an option is an action now, a folder under actions/ that module.yaml names under rules:",
	"module.sh": "a module has no shell of its own - what its scripts share with each other and with the modules beside it is a function in oak.sh beside oak.yaml",
}

// checkType settles a variable's type, which every one names, and the keys
// that belong to a list against it.
func checkType(v *Variable) error {
	takes, known := TypeKeys[v.Type]
	switch {
	case v.Type == "":
		return fmt.Errorf("type is required, one of %s", strings.Join(Types, ", "))
	case v.Type == "secret":
		return fmt.Errorf("type secret is %s, typed once where it exists already, or %s, typed twice where it is being chosen", TypePassword, TypeNewPassword)
	case !known:
		return fmt.Errorf("unknown type %q, which is one of %s", v.Type, strings.Join(Types, ", "))
	}
	for _, key := range v.typedKeys() {
		switch {
		case slices.Contains(takes, key):
		case len(takes) == 0:
			return fmt.Errorf("a %s takes no %s, nor any other key a type decides", v.Type, key)
		default:
			return fmt.Errorf("a %s takes no %s, of the keys a type decides only %s", v.Type, key, strings.Join(takes, ", "))
		}
	}
	if v.Listed() && len(v.Options) == 0 && v.OptionsFrom == "" {
		return fmt.Errorf("a %s takes its answers from options or options-from", v.Type)
	}
	if v.Deferred() {
		return checkDeferred(v)
	}
	return nil
}

// checkStages settles the phases the work happens in: at least one, each named
// once, and none of them carrying the mark that says the runtime runs it.
func checkStages(stages []string) error {
	if len(stages) == 0 {
		return fmt.Errorf("no stages")
	}
	seen := map[string]bool{}
	for _, stage := range stages {
		switch {
		case marked(stage):
			return fmt.Errorf("stage %q: the %s is the folder's, not the name's - list it as %s", stage, Mark, strings.TrimPrefix(stage, Mark))
		case seen[stage]:
			return fmt.Errorf("stage %q is listed twice", stage)
		}
		seen[stage] = true
	}
	return nil
}

// beside is the path of one of a module's optional parts, or empty where the
// module does not have it. Nothing declares them: being there is the
// declaration.
func beside(dir, name string) string {
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return path
}

// loadTasks reads tasks/: a marked folder per stage, a folder per task in it,
// so a task's stage is where it lies and its name what `needs` points at. An
// unmarked folder under tasks/ is refused, since a task dropped a level too
// high would load and never run.
func loadTasks(dir string, stages []string) ([]*Task, error) {
	base := filepath.Join(dir, DirTasks)
	entries, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no %s folder in %s", DirTasks, dir)
		}
		return nil, err
	}
	var out []*Task
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !marked(name) {
			return nil, fmt.Errorf("%s/%s: a task lies in the folder of its stage - %s/%s/%s/",
				DirTasks, name, DirTasks, Stage("<stage>"), name)
		}
		stage := strings.TrimPrefix(name, Mark)
		if !slices.Contains(stages, stage) {
			return nil, fmt.Errorf("%s/%s: no such stage - %s declares %s",
				DirTasks, name, FileModule, strings.Join(stages, ", "))
		}
		tasks, err := loadStage(filepath.Join(base, name), stage)
		if err != nil {
			return nil, err
		}
		out = append(out, tasks...)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: no tasks", DirTasks)
	}
	return out, nil
}

// loadStage reads the tasks of one stage. A stage folder with nothing in it is
// refused: an empty phase is a phase somebody meant to fill, and a run that
// simply skips it says nothing about the folder sitting there.
func loadStage(base, stage string) ([]*Task, error) {
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, err
	}
	var out []*Task
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		t, err := loadTask(filepath.Join(base, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("%s/%s/%s: %w", DirTasks, Stage(stage), entry.Name(), err)
		}
		t.stage = stage
		out = append(out, t)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s/%s: no tasks", DirTasks, Stage(stage))
	}
	return out, nil
}

// loadTask reads one folder: the yaml it is declared in, the task.sh that does
// the work and the test.sh that checks it, where there is one. A folder without
// either file it needs is an authoring mistake rather than an opt-out.
func loadTask(where string) (*Task, error) {
	t := &Task{id: filepath.Base(where), dir: where}
	if err := read(filepath.Join(where, FileTask), t); err != nil {
		return nil, err
	}
	t.work = Script(beside(where, FileTaskScript))
	t.check = Script(beside(where, FileTest))
	if t.work == "" {
		return nil, fmt.Errorf("no %s here - a task does its work in one", FileTaskScript)
	}
	return t, nil
}

// read decodes one file with unknown keys refused. A misspelled key that is
// merely ignored is the worst kind of authoring bug: everything loads, nothing
// behaves, and there is nothing to look at.
func read(path string, into any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(into); err != nil {
		return refused(path, err)
	}
	return nil
}

// retired is a key a product used to declare, and what to write instead. The
// file is still refused; the message only says what to do about it.
var retired = map[string]string{
	"blind":          "a question asked first opens its filter by itself",
	"id":             "a starting point is named by its title, and nothing anywhere points at one",
	"name":           "a title is what a person reads; a name only ever names a variable",
	"execute":        "a task does its work in the task.sh beside it, and is tested by the test.sh beside it",
	"script":         "a task does its work in the task.sh beside it, an action in the action.sh beside it, and the header's status reads check",
	"test":           "a task is tested by the test.sh beside it",
	"stage":          "a task lies in the folder of its stage, and that is the whole of where it runs",
	"network":        "a wireless network is an action under actions/, and the internet the work waits for is one named under rules: start-if",
	"action":         "the menu's rows are the runtime's own, Start and Setup",
	"start":          "the menu's rows are the runtime's own, Start and Setup",
	"start-title":    "the menu's rows are the runtime's own, Start and Setup",
	"settings-title": "the menu's rows are the runtime's own, Start and Setup",
	"console":        "the row that leaves to the console is the runtime's own",
	"default":        "a confirm opens on no, and on yes after a task its yes-after names has run",
	"variables":      "an action has one page: its variable, and a second question is a second action named under its rules: on-failure",
	"shows":          "a code is drawn by an action, beside its report",
	"quits":          "a way out is an action, named under rules: on-success or on-leave",
	"tty":            "a shell handed the terminal is an action with tty, named under rules: on-success or settings",
	"asks":           "a starting point that is fetched names the action that fetches it",
	"apply":          "a starting point that is fetched names the action that fetches it",
	"options":        "a starting point stands under presets: itself, and the page they are offered on is the runtime's own",
	"offered":        "it is a rule now: under rules:, as offer-if",
	"requires":       "it is a rule now: under rules:, as start-if in module.yaml and as offer-if in action.yaml",
	"menu":           "its rows stand on the settings page: under rules:, as on-settings",
	"settings":       "the menu's rows are the runtime's own, Start and Setup, and a row on the settings page is an action under rules:, as on-settings",
	"values":         "a question's list is options, and what prints one is options-from",
	"command":        "what prints a question's list is options-from",
	"answer":         "a value worked out instead of asked is value-from",
	"optional":       "a task the run goes on past when it fails says allow-failure",
	"fail":           "what a no from an action means is its error",
	"leave":          "it is a rule now: under rules:, as on-leave",
	"failure":        "it is a rule now: under rules:, as on-failure",
	"success":        "it is a rule now: under rules:, as on-success",
	"fallback":       "it is a rule now: under rules:, as on-failure",
	"existing":       "a password that exists already is type: password, one being chosen type: new-password",
	"free":           "a list that also takes an answer typed in is type: open-list",
	"variable":       "an action's questions are variables:, a list like the module's",
	"description":    "a question, a starting point and an action are described, and a module's menu is its icon over its rows",
	"text":           "the menu's rows are the runtime's own, Start and Setup, and the yes before the work is confirm: true",
}

// unknownField is how the decoder says a key is not one of them. It names the
// Go type it was decoding into, which is true and of no use to anybody holding
// the yaml.
var unknownField = regexp.MustCompile(`^line (\d+): field ([\w-]+) not found in type \S+$`)

// refused says what is wrong with a file in the file's own terms.
func refused(path string, err error) error {
	var typed *yaml.TypeError
	if !errors.As(err, &typed) {
		return fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	said := make([]string, 0, len(typed.Errors))
	for _, e := range typed.Errors {
		m := unknownField.FindStringSubmatch(strings.TrimSpace(e))
		if m == nil {
			said = append(said, strings.TrimSpace(e))
			continue
		}
		line := fmt.Sprintf("line %s: %s is not a key here", m[1], m[2])
		if instead, ok := retired[m[2]]; ok {
			line += " - " + instead
		}
		said = append(said, line)
	}
	return fmt.Errorf("%s: %s", filepath.Base(path), strings.Join(said, "\n"))
}

func (s *Module) check(tasks []*Task, runs map[string]int) error {
	s.normalize(tasks)
	if s.UI.Title == "" {
		return fmt.Errorf("%s: title is required", FileModule)
	}
	if err := s.checkVars(); err != nil {
		return fmt.Errorf("%s: %w", FileModule, err)
	}
	if err := s.checkPresets(); err != nil {
		return fmt.Errorf("%s: %w", FileModule, err)
	}
	if err := s.checkActions(runs); err != nil {
		return err
	}
	if err := s.checkTasks(tasks); err != nil {
		return err
	}
	if err := s.checkModuleAsks(); err != nil {
		return fmt.Errorf("%s: %w", FileModule, err)
	}
	return s.checkDeferredAsked(tasks)
}

// checkText refuses a {{VAR}} that names no answer of this module, which would
// read as a whole sentence with a hole in it right where somebody decides.
// Checked once the variables are known, so a sentence may name one declared
// after it.
func (s *Module) checkText(key, text string) error {
	for _, name := range Names(text) {
		if s.byName[name] == nil {
			return fmt.Errorf("%s: {{%s}} is not a variable of this module", key, name)
		}
	}
	return nil
}

// checkTasks settles what runs and in what order: every task checked over, the
// needs resolved, and what is left sorted once and for all.
func (s *Module) checkTasks(tasks []*Task) error {
	byStage := map[string][]*Task{}
	for _, t := range tasks {
		if err := s.checkTask(t); err != nil {
			return fmt.Errorf("%s/%s/%s: %w", DirTasks, Stage(t.stage), t.id, err)
		}
		byStage[t.stage] = append(byStage[t.stage], t)
	}
	warnings, err := checkNeeds(byStage)
	if err != nil {
		return err
	}
	s.Warnings = warnings

	for _, stage := range s.Stages {
		ordered, err := order(byStage[stage])
		if err != nil {
			return fmt.Errorf("%s/%s: %w", DirTasks, Stage(stage), err)
		}
		s.Tasks = append(s.Tasks, ordered...)
	}
	return s.checkYesAfter()
}

// checkYesAfter holds every yes-after to tasks that can have run by the time
// its confirm is asked. Anything else is a confirm that never opens on Yes.
func (s *Module) checkYesAfter() error {
	for i, t := range s.Tasks {
		if len(t.YesAfter) > 0 && !t.Confirms() {
			return fmt.Errorf("%s: yes-after: a task without confirm is never asked", t.where())
		}
		for _, name := range t.YesAfter {
			j := slices.IndexFunc(s.Tasks, func(o *Task) bool { return o.id == name })
			switch {
			case j < 0:
				return fmt.Errorf("%s: yes-after: no such task: %s", t.where(), name)
			case j >= i:
				return fmt.Errorf("%s: yes-after: %s does not run before it", t.where(), name)
			}
		}
	}
	return nil
}

// checkTask settles one task on its own: what it is called, what it asks for
// and shows, and what it is guarded by. Which phase it belongs to was settled
// by the folder it was found in, before it was read at all.
func (s *Module) checkTask(t *Task) error {
	if t.Title == "" {
		return fmt.Errorf("title is required")
	}
	if err := s.checkAsks(t); err != nil {
		return err
	}
	if err := s.checkText("confirm", t.Confirm); err != nil {
		return err
	}
	if err := s.checkText("report", t.Report); err != nil {
		return err
	}
	cond, err := s.conditions(t.Conditions)
	if err != nil {
		return err
	}
	t.cond = cond
	return nil
}

// checkNeeds resolves what every task waits for. A name in another stage says
// nothing the stages do not, so it is dropped with a warning, while a name
// belonging to nothing is refused.
func checkNeeds(groups map[string][]*Task) ([]string, error) {
	elsewhere := map[string]string{}
	for group, tasks := range groups {
		for _, t := range tasks {
			elsewhere[t.id] = group
		}
	}
	var warnings []string
	for _, group := range slices.Sorted(maps.Keys(groups)) {
		here := map[string]bool{}
		for _, t := range groups[group] {
			here[t.id] = true
		}
		for _, t := range groups[group] {
			kept := t.Needs[:0]
			for _, n := range t.Needs {
				switch {
				case here[n]:
					kept = append(kept, n)
				case elsewhere[n] != "":
					warnings = append(warnings, fmt.Sprintf("%s: needs %s, which is in %s - needs orders tasks within one stage, the stages order the rest",
						t.where(), n, elsewhere[n]))
				default:
					return nil, fmt.Errorf("%s: needs unknown task: %s", t.where(), n)
				}
			}
			t.Needs = kept
		}
	}
	return warnings, nil
}

// where is the folder a task was read from, as a module's author knows it.
func (t *Task) where() string { return fmt.Sprintf("%s/%s", DirTasks, t.id) }

// checkAsks settles a task's `asks:`, which names a question put in the middle
// of a run. The variable says so itself, with `type: deferred`, so a module read
// from the top tells it from the questions asked on the way in.
func (s *Module) checkAsks(t *Task) error {
	if t.Asks == "" {
		return nil
	}
	v := s.byName[t.Asks]
	switch {
	case v == nil:
		return fmt.Errorf("asks: no such variable: %s", t.Asks)
	case !v.Deferred():
		return fmt.Errorf("asks: %s is asked on the way in - a question asked mid-run says type: %s", t.Asks, TypeDeferred)
	}
	return nil
}

// checkModuleAsks settles the module's `asks:`, the questions put each time the
// work is started. Each names a deferred question once, since one the answer
// file kept would be asked again all the same.
func (s *Module) checkModuleAsks() error {
	seen := map[string]bool{}
	for _, name := range s.Asks {
		v := s.byName[name]
		switch {
		case v == nil:
			return fmt.Errorf("asks: no such variable: %s", name)
		case !v.Deferred():
			return fmt.Errorf("asks: %s is asked on the way in - a question asked as the work starts says type: %s", name, TypeDeferred)
		case seen[name]:
			return fmt.Errorf("asks: %s is listed twice", name)
		}
		seen[name] = true
	}
	return nil
}

// checkDeferred holds a question asked mid-run to a list, since a text box
// there has nothing to check it and a yes or no is a task's `confirm:`. What
// only the way in or the settings page reads is refused, since neither shows
// it.
func checkDeferred(v *Variable) error {
	switch {
	case v.First:
		return fmt.Errorf("first: a deferred question is asked under asks:, never on the way in")
	case v.Group != "":
		return fmt.Errorf("group: a deferred question is never on the settings page")
	}
	return nil
}

// checkDeferredAsked refuses a deferred question nothing asks, which would be
// asked nowhere at all: not on the way in, not on the settings page, not in
// the run. The module and a task asking the same one would ask it twice.
func (s *Module) checkDeferredAsked(tasks []*Task) error {
	asked := map[string]bool{}
	for _, name := range s.Asks {
		asked[name] = true
	}
	for _, t := range tasks {
		if t.Asks != "" && slices.Contains(s.Asks, t.Asks) {
			return fmt.Errorf("%s: asks: %s is asked by the module as the work starts already", t.where(), t.Asks)
		}
		asked[t.Asks] = true
	}
	for _, v := range s.Vars {
		if v.Deferred() && !asked[v.Name] {
			return fmt.Errorf("%s: %s: type %s is asked under asks:, by a task or the module, and nothing asks it", FileModule, v.Name, TypeDeferred)
		}
	}
	return nil
}

// normalize undoes the line breaks of a block scalar in every word the module
// says, so the text shown and the key a catalog looks it up by are the same
// string. A blank line survives, the one break that was meant.
func (s *Module) normalize(tasks []*Task) {
	fields := []*string{&s.UI.Title}
	for _, o := range s.Presets {
		fields = append(fields, &o.Title, &o.Description)
	}
	for _, a := range s.Actions {
		fields = append(fields, &a.Title, &a.Description, &a.Error, &a.Confirm, &a.Report)
	}
	for _, v := range s.Declared() {
		fields = append(fields, &v.Title, &v.Description, &v.Group, &v.Error)
	}
	for _, t := range tasks {
		fields = append(fields, &t.Title, &t.Confirm, &t.Report)
	}
	for _, f := range fields {
		*f = reflow(*f)
	}
}

// reflow joins the lines of each paragraph and keeps the blank lines between
// them.
func reflow(s string) string {
	paras := strings.Split(s, "\n\n")
	out := make([]string, 0, len(paras))
	for _, p := range paras {
		if p = strings.Join(strings.Fields(p), " "); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, "\n\n")
}

var (
	hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	varName  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

func (s *Module) checkVars() error {
	for _, v := range s.Vars {
		if err := s.checkVar(v, s.Dir); err != nil {
			return err
		}
	}
	if s.Language != "" && s.byName[s.Language] == nil {
		return fmt.Errorf("language: no such variable: %s", s.Language)
	}
	// Conditions are checked once every name is known, so a variable may be
	// guarded by one declared after it.
	for _, v := range s.Vars {
		cond, err := s.conditions(v.Conditions)
		if err != nil {
			return fmt.Errorf("%s: %w", v.Name, err)
		}
		v.cond = cond
	}
	return nil
}

// checkVar holds one question to the rules every question keeps, settles the
// shell it names relative to dir - the folder of the yaml it was written in -
// and makes its name the module's.
func (s *Module) checkVar(v *Variable, dir string) error {
	switch {
	case !varName.MatchString(v.Name):
		return fmt.Errorf("%q is not a usable variable name", v.Name)
	case runtimeVar(v.Name):
		return fmt.Errorf("%s belongs to the runtime and cannot be declared", v.Name)
	case s.byName[v.Name] != nil:
		return fmt.Errorf("%s is declared twice", v.Name)
	case v.Title == "":
		return fmt.Errorf("%s: title is required", v.Name)
	}
	if err := checkType(v); err != nil {
		return fmt.Errorf("%s: %w", v.Name, err)
	}
	if v.Secret() && v.First {
		return fmt.Errorf("%s: a password is asked for immediately before the run that needs it, so it cannot also be asked first", v.Name)
	}
	if len(v.Options) > 0 && v.OptionsFrom != "" {
		return fmt.Errorf("%s: options and options-from are two answers to the same question", v.Name)
	}
	switch v.Filter {
	case "", FilterOpen, FilterCollapsed:
	default:
		return fmt.Errorf("%s: unknown filter %q, which is %s or %s", v.Name, v.Filter, FilterOpen, FilterCollapsed)
	}
	if v.Filter != "" && v.First {
		return fmt.Errorf("%s: a question asked first carries its box open by itself, so filter says nothing here", v.Name)
	}
	if v.Derived() {
		switch {
		case v.Prefill != "":
			return fmt.Errorf("%s: value-from settles the value, prefill only suggests one - a question is asked or it is not", v.Name)
		case v.First:
			return fmt.Errorf("%s: a derived answer is never asked, so it cannot be asked first", v.Name)
		}
	}
	if v.Pattern != "" {
		re, err := regexp.Compile(v.Pattern)
		if err != nil {
			return fmt.Errorf("%s: pattern: %w", v.Name, err)
		}
		v.re = re
	}
	file := FileModule
	if dir != s.Dir {
		file = path.Join(DirActions, filepath.Base(dir), FileAction)
	}
	for _, f := range []struct {
		key  string
		expr *string
	}{
		{"options-from", &v.OptionsFrom}, {"prefill", &v.Prefill}, {"apply", &v.Apply},
		{"value-from", &v.ValueFrom}, {"check", &v.Check},
	} {
		run, fn, err := shell(dir, *f.expr)
		if err != nil {
			return fmt.Errorf("%s: %s: %w", v.Name, f.key, err)
		}
		*f.expr = run
		if fn != "" {
			s.calls[fn] = fmt.Sprintf("%s: %s: %s", file, v.Name, f.key)
		}
	}
	s.byName[v.Name] = v
	return nil
}

// checkPresets settles the starting points. One is named by its title and
// nothing else: it is a row on a page, and nothing a module declares ever has
// to point at one.
func (s *Module) checkPresets() error {
	for i, o := range s.Presets {
		if o.Title == "" {
			return fmt.Errorf("presets: %d: title is required", i+1)
		}
		for name := range o.Values {
			if s.byName[name] == nil {
				return fmt.Errorf("presets: %s: no such variable: %s", o.Title, name)
			}
		}
		if o.Fetches() && len(o.Values) > 0 {
			return fmt.Errorf("presets: %s: a starting point is written out in values or fetched by an action, not both", o.Title)
		}
	}
	return nil
}

// conditions parses a `conditions:` and checks that every line of it is about a
// variable this module actually declares.
func (s *Module) conditions(exprs Conditions) ([]*condition, error) {
	var out []*condition
	for _, expr := range exprs {
		if strings.TrimSpace(expr) == "" {
			continue
		}
		c, err := parseCondition(expr)
		if err != nil {
			return nil, err
		}
		if s.byName[c.name] == nil {
			return nil, fmt.Errorf("conditions: no such variable: %s", c.name)
		}
		out = append(out, c)
	}
	return out, nil
}

// call is how a field names a function of oak.sh: its name and a pair of
// parentheses, so it reads as a call and never as the shell it stands for.
var call = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\(\)$`)

// shell settles a field that names shell against the folder of its yaml: a
// function of oak.sh written name(), or a file beside the yaml written
// ./file.sh, so a linter reads it and a failure has a line. It hands back what
// to run and the function it calls, if any.
func shell(dir, expr string) (run, fn string, err error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return "", "", nil
	}
	if m := call.FindStringSubmatch(expr); m != nil {
		return m[1], m[1], nil
	}
	relative := strings.HasPrefix(expr, "./") || strings.HasPrefix(expr, "../")
	if !relative || !strings.HasSuffix(expr, ScriptExt) || strings.ContainsAny(expr, " \t\n") {
		return "", "", fmt.Errorf("%q is neither a function of %s, written name(), nor a file beside this yaml, written ./name%s", expr, FileRuntimeShell, ScriptExt)
	}
	file := filepath.Join(dir, expr)
	if _, err := os.Stat(file); err != nil {
		return "", "", fmt.Errorf("no such script: %s", expr)
	}
	return source(file), "", nil
}

// definition is the line that opens a function in bash, either way bash
// writes one.
var definition = regexp.MustCompile(`(?m)^[ \t]*(?:function[ \t]+([A-Za-z_][A-Za-z0-9_]*)|([A-Za-z_][A-Za-z0-9_]*)[ \t]*\(\))`)

// functions is every function a shell file defines. Read rather than run: the
// library is loaded in front of every script, not at startup.
func functions(file string) (names, error) {
	out := names{}
	if file == "" {
		return out, nil
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	for _, m := range definition.FindAllStringSubmatch(string(raw), -1) {
		out[m[1]+m[2]] = true // one alternative matched, the other is empty
	}
	return out, nil
}

// checkCalls refuses a function the yaml calls that oak.sh does not define: a
// typo there would otherwise be a command not found in the middle of a run.
func checkCalls(calls map[string]string, defined names) error {
	for _, fn := range slices.Sorted(maps.Keys(calls)) {
		if !defined[fn] {
			return fmt.Errorf("%s: %s() is not a function in %s", calls[fn], fn, FileRuntimeShell)
		}
	}
	return nil
}

// quote wraps a path for the shell, so a module whose name holds a space or a
// quote is still one word.
func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
