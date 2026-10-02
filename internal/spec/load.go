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
// be typed — but for the subjects with parts of their own: the words of its
// pages, the rules that run its actions, and the header's status.
type declaration struct {
	Title    string `yaml:"title"`
	Language string `yaml:"language"`

	// What this module is and the phases its work happens in.
	Description string   `yaml:"description"`
	Stages      []string `yaml:"stages"`

	// The words its own pages are drawn with, in place of the runtime's.
	Text Text `yaml:"text"`

	// Where it runs its actions, each a list of their names — see Rules.
	Rules Rules `yaml:"rules"`

	Presets   []*Preset   `yaml:"presets"`
	Variables []*Variable `yaml:"variables"`

	// The header's line for this module, in place of the product's.
	Status *Status `yaml:"status"`
}

// Load reads one module folder and checks it over — every reference resolved,
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
	s.UI = UI{Title: head.Title, Description: head.Description, Text: head.Text}
	s.Presets, s.Vars, s.Language = head.Presets, head.Variables, head.Language
	s.Stages, s.Rules = head.Stages, head.Rules
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
	"hooks/":    "the runtime runs no hooks — each is an action now, a folder under actions/ that module.yaml names under rules:",
	"options/":  "an option is an action now, a folder under actions/ that module.yaml names under rules:",
	"module.sh": "a module has no shell of its own — what its scripts share with each other and with the modules beside it is a function in oak.sh beside oak.yaml",
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
			return fmt.Errorf("stage %q: the %s is the folder's, not the name's — list it as %s", stage, Mark, strings.TrimPrefix(stage, Mark))
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

// loadTasks reads tasks/, which is two levels: a folder per stage, marked, and
// in each of those a folder per task.
//
// A task's stage is therefore where it lies rather than a line it writes, so it
// can never say one thing and sit in another.
//
// The task folder's name is its identity — what another task's `needs` in the
// same stage points at — and no more than that: what runs when is the stage it
// lies in and the order inside it.
//
// A folder under tasks/ without the mark is refused rather than passed over. It
// is the one mistake that would otherwise load and do nothing at all: a task
// dropped a level too high runs in no stage, and nothing about a module that
// starts would say so.
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
			return nil, fmt.Errorf("%s/%s: a task lies in the folder of its stage — %s/%s/%s/",
				DirTasks, name, DirTasks, Stage("<stage>"), name)
		}
		stage := strings.TrimPrefix(name, Mark)
		if !slices.Contains(stages, stage) {
			return nil, fmt.Errorf("%s/%s: no such stage — %s declares %s",
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
		return nil, fmt.Errorf("no %s here — a task does its work in one", FileTaskScript)
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

// retired is a key a product used to be able to declare, and what to write
// instead. Each of them was dropped because the same thing was already said
// somewhere else, so there is always a sentence to point at.
//
// It is not a compatibility layer: the file is still refused. It is the
// refusal saying what to do about itself, which is all a message that stops a
// build is for.
var retired = map[string]string{
	"blind":          "a question asked first opens its filter by itself",
	"id":             "a starting point is named by its title, and nothing anywhere points at one",
	"name":           "a title is what a person reads; a name only ever names a variable",
	"execute":        "a task does its work in the task.sh beside it, and is tested by the test.sh beside it",
	"script":         "a task does its work in the task.sh beside it, an action in the action.sh beside it, and the header's status reads check",
	"test":           "a task is tested by the test.sh beside it",
	"stage":          "a task lies in the folder of its stage, and that is the whole of where it runs",
	"network":        "a wireless network is an action under actions/, and the internet the work waits for is one named under rules: start-if",
	"action":         "the word for starting the work is text: start",
	"start":          "the word for starting the work is text: start",
	"start-title":    "it is text: start",
	"settings-title": "it is text: settings",
	"console":        "the row that leaves to the console is the runtime's own",
	"confirm":        "a task that needs asking says confirm itself, the last page before a module's work is text: confirm, and an action is agreed to by choosing its row",
	"default":        "every confirm opens on no",
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
	"settings":       "the settings page is named by text: settings, and a row on it is an action under rules:, as on-settings",
	"values":         "a question's list is options, and what prints one is options-from",
	"command":        "what prints a question's list is options-from",
	"answer":         "a value worked out instead of asked is value-from",
	"optional":       "a task the run goes on past when it fails says allow-failure",
	"fail":           "what a no from an action means is its error",
	"leave":          "it is a rule now: under rules:, as on-leave",
	"failure":        "it is a rule now: under rules:, as on-failure",
	"success":        "it is a rule now: under rules:, as on-success",
	"fallback":       "it is a rule now: under rules:, as on-failure",
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
			line += " — " + instead
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
	if err := s.checkText("text: confirm", s.UI.Text.Confirm); err != nil {
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
	return s.checkDeferredAsked(tasks)
}

// checkText holds a sentence to the answers this module has. A {{VAR}} naming
// none of them is filled in with nothing and leaves a sentence that still reads
// as one — "everything on will be erased" — at the moment somebody is deciding
// whether to go ahead. Refused where it is written rather than noticed where it
// is read, which is too late by then.
//
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

// checkNeeds resolves what every task waits for, and says what it found.
//
// `needs` orders tasks across one stage; the stages order the rest. So a name
// belonging to another stage
// says nothing the stages have not already said, and is dropped with a word
// about it rather than refused — a module that behaves is not a module that
// refuses to start. A name belonging to nothing is a different thing entirely:
// it is a task waiting for something that does not exist, and there is no
// reading of it that runs.
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
					warnings = append(warnings, fmt.Sprintf("%s: needs %s, which is in %s — needs orders tasks within one stage, the stages order the rest",
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
		return fmt.Errorf("asks: %s is asked on the way in — a question asked mid-run says type: %s", t.Asks, TypeDeferred)
	}
	return nil
}

// checkDeferred holds a question asked mid-run to what the frame can put there.
//
// Only a list qualifies. A text box mid-run would be a second way of answering
// with nothing to check it against on a page nobody navigated to, and a yes or
// no in front of a task is its `confirm:`. What only the way in or the settings
// page reads is refused, since neither ever shows it.
func checkDeferred(v *Variable) error {
	switch {
	case len(v.Options) == 0 && v.OptionsFrom == "":
		return fmt.Errorf("a question asked mid-run is a list, and this one has no options or options-from")
	case v.First:
		return fmt.Errorf("first: a deferred question is asked by its task, mid-run")
	case v.Group != "":
		return fmt.Errorf("group: a deferred question is never on the settings page")
	case v.Derived():
		return fmt.Errorf("value-from: a deferred question is asked, and a value worked out is not")
	}
	return nil
}

// checkDeferredAsked refuses a deferred question no task asks, which would be
// asked nowhere at all: not on the way in, not on the settings page, not in
// the run.
func (s *Module) checkDeferredAsked(tasks []*Task) error {
	asked := map[string]bool{}
	for _, t := range tasks {
		asked[t.Asks] = true
	}
	for _, v := range s.Vars {
		if v.Deferred() && !asked[v.Name] {
			return fmt.Errorf("%s: %s: type %s is asked by a task under asks:, and no task asks it", FileModule, v.Name, TypeDeferred)
		}
	}
	return nil
}

// normalize settles every word the module says into the shape it is both shown in
// and translated by.
//
// Where a line ends in the yaml is not where it ends on screen: a description is
// written in a block scalar and wrapped by whoever was editing it, to whatever
// width their editor was that day. Those breaks are undone here, once, so that
// the text and the key a catalog looks it up by are the same string — and so a
// translator is handed one line per message instead of somebody's line wrapping
// to reproduce.
//
// A blank line survives, because that is the one break that was meant.
func (s *Module) normalize(tasks []*Task) {
	fields := []*string{&s.UI.Title, &s.UI.Description, &s.UI.Text.Start, &s.UI.Text.Settings, &s.UI.Text.Confirm}
	for _, o := range s.Presets {
		fields = append(fields, &o.Title, &o.Description)
	}
	for _, a := range s.Actions {
		fields = append(fields, &a.Title, &a.Description, &a.Error, &a.Report)
	}
	for _, v := range s.Declared() {
		fields = append(fields, &v.Title, &v.Description, &v.Group, &v.Free, &v.Error)
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
// shell it names relative to dir — the folder of the yaml it was written in —
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
	switch v.Shape() {
	case TypeText:
	case TypeBool, TypeSecret:
		if len(v.Options) > 0 || v.OptionsFrom != "" {
			return fmt.Errorf("%s: a %s variable has no options of its own", v.Name, v.Shape())
		}
	case TypeDeferred:
		if err := checkDeferred(v); err != nil {
			return fmt.Errorf("%s: %w", v.Name, err)
		}
	default:
		return fmt.Errorf("%s: unknown type %q", v.Name, v.Type)
	}
	if v.Secret() && v.Default != "" {
		return fmt.Errorf("%s: a secret is never stored, so it cannot have a default", v.Name)
	}
	if v.Secret() && v.First {
		return fmt.Errorf("%s: a secret is asked for immediately before the run that needs it, so it cannot also be asked first", v.Name)
	}
	if v.Existing && !v.Secret() {
		return fmt.Errorf("%s: existing says a password is entered rather than chosen, and only a secret is either", v.Name)
	}
	if v.Check != "" && !v.Secret() {
		return fmt.Errorf("%s: check looks at a secret as it is typed, and any other answer is held to its pattern", v.Name)
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
	if v.Filter != "" && len(v.Options) == 0 && v.OptionsFrom == "" {
		return fmt.Errorf("%s: filter narrows a list of answers, and this question is a box to type in", v.Name)
	}
	if v.Derived() {
		switch {
		case v.Secret():
			return fmt.Errorf("%s: a secret is typed by a person, never worked out", v.Name)
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

// shell settles a field that names shell, against dir - the folder of the yaml
// it was written in. It names either a function of oak.sh, written name(), or a
// file beside the yaml, written ./file.sh, so the shell is always somewhere a
// linter reads and a failure has a line. Shell written into the yaml itself is
// refused. Handed back are what to run and the function it calls, if it does.
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
