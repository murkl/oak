package spec

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"os"
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
// be typed.
type declaration struct {
	Title    string `yaml:"title"`
	Action   string `yaml:"action"`
	Console  string `yaml:"console"`
	Language string `yaml:"language"`

	// What this module is and what it does: one sentence about the program, the
	// last warning before a run starts, and the phases that run happens in.
	Description string   `yaml:"description"`
	Confirm     string   `yaml:"confirm"`
	Stages      []string `yaml:"stages"`

	Presets   []*Preset   `yaml:"presets"`
	Variables []*Variable `yaml:"variables"`
}

// Load reads one module folder and checks it over — every reference resolved,
// every script found, every condition naming a variable that exists, every task
// in a stage that exists and in an order that can be walked.
//
// A module that loads is a module that runs: an authoring mistake is a message
// at startup, never a task that silently never fires.
func Load(dir string) (*Module, error) {
	s := &Module{Dir: dir, byName: map[string]*Variable{}}

	var head declaration
	if err := read(filepath.Join(dir, FileModule), &head); err != nil {
		return nil, err
	}
	s.UI = UI{Title: head.Title, Action: head.Action, Description: head.Description, Console: head.Console}
	s.Presets, s.Vars, s.Language = head.Presets, head.Variables, head.Language
	s.Confirm, s.Stages = head.Confirm, head.Stages
	if err := checkStages(s.Stages); err != nil {
		return nil, fmt.Errorf("%s: %w", FileModule, err)
	}
	s.Shell = beside(dir, FileShell)
	s.Locales = beside(dir, DirLocales)

	tasks, err := loadTasks(dir, s.Stages)
	if err != nil {
		return nil, err
	}
	hooks, err := loadHooks(dir)
	if err != nil {
		return nil, err
	}
	if err := s.check(tasks, hooks); err != nil {
		return nil, err
	}
	return s, nil
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
// A task's stage is therefore where it lies rather than a line it writes, the
// same way a step of a hook takes its moment from the folder above it. The two
// read alike on disk, and neither can say one thing and sit in another.
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
		// Both levels wear the mark, so a hook dropped in here looks exactly
		// like a stage. It is named for what it is rather than for the stage it
		// is not: the folder is right, the half of the tree is wrong.
		if slices.Contains(Hooks, name) {
			return nil, fmt.Errorf("%s/%s: %s is one of the runtime's own moments, which lives under %s/",
				DirTasks, name, name, DirHooks)
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
		t, err := loadTask(filepath.Join(base, entry.Name()), FileTask, FileTaskScript)
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

// loadHooks reads hooks/, where every folder is one of the runtime's own
// moments and every folder in one of those is a step of it.
//
// A module that fills none of them has no hooks/ at all, which is not an error:
// it simply does not get those parts of the program.
func loadHooks(dir string) (map[string][]*Task, error) {
	base := filepath.Join(dir, DirHooks)
	entries, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := map[string][]*Task{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		steps, err := loadHook(filepath.Join(base, name), name)
		if err != nil {
			return nil, err
		}
		out[name] = steps
	}
	return out, nil
}

// loadHook reads the steps of one hook, in name order, which is what order
// falls back on for two steps nothing separates.
func loadHook(base, name string) ([]*Task, error) {
	if !slices.Contains(Hooks, name) {
		return nil, fmt.Errorf("%s/%s: no such hook — the runtime has %s",
			DirHooks, name, strings.Join(Hooks, ", "))
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, err
	}
	var out []*Task
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		t, err := loadTask(filepath.Join(base, entry.Name()), FileHook, FileHookScript)
		if err != nil {
			return nil, fmt.Errorf("%s/%s/%s: %w", DirHooks, name, entry.Name(), err)
		}
		t.hook = name
		out = append(out, t)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s/%s: no steps", DirHooks, name)
	}
	return out, nil
}

// loadTask reads one folder: the yaml it is declared in, and what it turns out
// to run. A folder without that file is an authoring mistake rather than an
// opt-out — it is an error, not a step quietly dropped from the run.
func loadTask(where, decl, script string) (*Task, error) {
	t := &Task{id: filepath.Base(where), dir: where}
	if err := read(filepath.Join(where, decl), t); err != nil {
		return nil, err
	}
	if err := t.resolve(decl, script); err != nil {
		return nil, err
	}
	return t, nil
}

// resolve settles what a task actually runs and what tests it afterwards: what
// its yaml wrote, or the file of that name beside it. Both is two answers to
// the same question, and neither for the work itself is a task that does
// nothing.
func (t *Task) resolve(decl, script string) error {
	work, err := pick(t.dir, "script", t.Script, script)
	if err != nil {
		return err
	}
	if work.Empty() {
		return fmt.Errorf("no %s here, and no script in %s", script, decl)
	}
	check, err := pick(t.dir, "test", t.Test, FileTest)
	if err != nil {
		return err
	}
	t.work, t.check = work, check
	return nil
}

// pick settles one of the two: the shell the yaml wrote, the file it named, or
// — where it said nothing — the file of that name lying beside it.
func pick(dir, key, expr, name string) (Script, error) {
	file := beside(dir, name)
	switch {
	case expr != "" && file != "":
		return Script{}, fmt.Errorf("%s: there is a %s here as well, and a task runs one thing", key, name)
	case expr == "":
		return Script{File: file}, nil
	}
	named, err := scriptFile(dir, expr)
	if err != nil {
		return Script{}, fmt.Errorf("%s: %w", key, err)
	}
	if named != "" {
		return Script{File: named}, nil
	}
	return Script{Shell: expr}, nil
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
	"run":     "a module is named once, by its title, and that is what one run of it is called",
	"blind":   "a question asked first opens its filter by itself",
	"id":      "a starting point is named by its title, and nothing anywhere points at one",
	"name":    "a title is what a person reads; a name only ever names a variable",
	"execute": "a task says what it does under script, and how it is tested afterwards under test",
	"stage":   "a task lies in the folder of its stage, and that is the whole of where it runs",
}

// unknownField is how the decoder says a key is not one of them. It names the
// Go type it was decoding into, which is true and of no use to anybody holding
// the yaml.
var unknownField = regexp.MustCompile(`^line (\d+): field (\w+) not found in type \S+$`)

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

func (s *Module) check(tasks []*Task, hooks map[string][]*Task) error {
	s.normalize(tasks, hooks)
	if s.UI.Title == "" {
		return fmt.Errorf("%s: title is required", FileModule)
	}
	if err := s.checkVars(); err != nil {
		return fmt.Errorf("%s: %w", FileModule, err)
	}
	if err := s.checkPresets(); err != nil {
		return fmt.Errorf("%s: %w", FileModule, err)
	}
	return s.checkTasks(tasks, hooks)
}

// checkTasks settles what runs and in what order: every task checked over, the
// needs resolved, and what is left sorted once and for all.
//
// The hooks are kept apart from the work. They run at their own moment rather
// than as part of it, so a step in one is neither ordered against the rest nor
// listed anywhere a run is.
func (s *Module) checkTasks(tasks []*Task, hooks map[string][]*Task) error {
	byStage := map[string][]*Task{}
	for _, t := range tasks {
		if err := s.checkTask(t); err != nil {
			return fmt.Errorf("%s/%s/%s: %w", DirTasks, Stage(t.stage), t.id, err)
		}
		byStage[t.stage] = append(byStage[t.stage], t)
	}
	for name, steps := range hooks {
		for _, t := range steps {
			if err := t.checkHook(); err != nil {
				return fmt.Errorf("%s/%s/%s: %w", DirHooks, name, t.id, err)
			}
		}
	}
	warnings, err := checkNeeds(byStage, hooks)
	if err != nil {
		return err
	}
	s.Warnings = warnings

	s.hooks = map[string][]*Task{}
	for name, steps := range hooks {
		ordered, err := order(steps)
		if err != nil {
			return fmt.Errorf("%s/%s: %w", DirHooks, name, err)
		}
		s.hooks[name] = ordered
	}
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
	if err := checkConfirm(t); err != nil {
		return err
	}
	if err := s.checkShows(t); err != nil {
		return err
	}
	cond, err := s.conditions(t.Conditions)
	if err != nil {
		return err
	}
	t.cond = cond
	return nil
}

// checkHook refuses everything a step of a hook cannot mean. What is left is
// its title, what it needs and what it does.
//
// A hook is run at a fixed moment rather than listed, offered or reported on,
// and it is not part of the work, so there is nothing for a stage, a guard or
// a check afterwards to answer to. Saying any of it would be writing down a
// line that can never take effect.
func (t *Task) checkHook() error {
	if t.Title == "" {
		return fmt.Errorf("title is required")
	}
	said := []struct {
		key  string
		used bool
	}{
		{"test", t.Checks()},
		{"conditions", len(t.Conditions) > 0},
		{"asks", t.Asks != ""},
		{"confirm", t.Confirms()},
		{"default", t.Default != ""},
		{"report", t.Reports()},
		{"shows", t.Shows != ""},
		{"quits", t.Quits},
		{"tty", t.TTY},
	}
	for _, k := range said {
		if k.used {
			return fmt.Errorf("%s: %s is run by the runtime rather than as part of the work, so there is nothing for it to answer to", k.key, t.hook)
		}
	}
	return nil
}

// checkNeeds resolves what every task waits for, and says what it found.
//
// `needs` orders tasks across one stage, and the steps of one hook among
// themselves; the stages order the rest. So a name belonging to another stage
// says nothing the stages have not already said, and is dropped with a word
// about it rather than refused — a module that behaves is not a module that
// refuses to start. A name belonging to nothing is a different thing entirely:
// it is a task waiting for something that does not exist, and there is no
// reading of it that runs.
func checkNeeds(byStage, hooks map[string][]*Task) ([]string, error) {
	// A hook carries the mark and a stage may not, so the two sets of names can
	// never collide.
	groups := map[string][]*Task{}
	maps.Copy(groups, byStage)
	maps.Copy(groups, hooks)

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
func (t *Task) where() string {
	if t.hook != "" {
		return fmt.Sprintf("%s/%s/%s", DirHooks, t.hook, t.id)
	}
	return fmt.Sprintf("%s/%s", DirTasks, t.id)
}

// checkConfirm settles a task's `default:`, which says which of the two answers
// its offer opens on. There are exactly two, and a task that names one without
// making an offer at all has said something that can never take effect.
func checkConfirm(t *Task) error {
	switch {
	case t.Default == "":
		return nil
	case !t.Confirms():
		return fmt.Errorf("default: there is no confirm for it to answer")
	case t.Default != ConfirmYes && t.Default != ConfirmNo:
		return fmt.Errorf("default: %s or %s, got %q", ConfirmYes, ConfirmNo, t.Default)
	}
	return nil
}

// checkShows settles a task's `shows:`, which is an answer put on the page its
// `report:` draws — as a code to scan, and under it as itself.
//
// A secret is refused for the reason it is refused everywhere: it is never
// written down, and drawing one at a size a camera across the room can read is
// the opposite of what it is for.
func (s *Module) checkShows(t *Task) error {
	if t.Shows == "" {
		return nil
	}
	v := s.byName[t.Shows]
	switch {
	case v == nil:
		return fmt.Errorf("shows: no such variable: %s", t.Shows)
	case v.Secret():
		return fmt.Errorf("shows: %s is a secret, and a secret is not put on screen to be read across a room", t.Shows)
	case !t.Reports():
		return fmt.Errorf("shows: there is no report for it to appear on")
	}
	v.deferred = true
	return nil
}

// checkAsks settles a task's `asks:`, which is a question put in the middle of
// a run and therefore has to be one the frame can put there.
//
// Only a list qualifies. A text box mid-run would be a second way of answering
// with nothing to check it against on a page nobody navigated to, and a secret
// is already asked for at the one moment it is safe to — immediately before the
// run that needs it.
func (s *Module) checkAsks(t *Task) error {
	if t.Asks == "" {
		return nil
	}
	v := s.byName[t.Asks]
	switch {
	case v == nil:
		return fmt.Errorf("asks: no such variable: %s", t.Asks)
	case v.Secret():
		return fmt.Errorf("asks: %s is a secret, which is asked for immediately before the run", t.Asks)
	case len(v.Values) == 0 && v.Command == "" && v.Shape() != TypeBool:
		return fmt.Errorf("asks: %s has no answers to choose from, and a question asked mid-run is a list", t.Asks)
	}
	v.deferred = true
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
func (s *Module) normalize(tasks []*Task, hooks map[string][]*Task) {
	fields := []*string{&s.UI.Title, &s.UI.Description, &s.UI.Console, &s.Confirm}
	for _, p := range s.Presets {
		fields = append(fields, &p.Title, &p.Description)
		for _, o := range p.Options {
			fields = append(fields, &o.Title, &o.Description)
		}
	}
	for _, v := range s.Vars {
		fields = append(fields, &v.Title, &v.Description, &v.Group, &v.Free, &v.Error)
	}
	for _, t := range tasks {
		fields = append(fields, &t.Title, &t.Confirm, &t.Report)
	}
	for _, steps := range hooks {
		for _, t := range steps {
			fields = append(fields, &t.Title)
		}
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
			if len(v.Values) > 0 || v.Command != "" {
				return fmt.Errorf("%s: a %s variable has no values of its own", v.Name, v.Shape())
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
		if len(v.Values) > 0 && v.Command != "" {
			return fmt.Errorf("%s: values and command are two answers to the same question", v.Name)
		}
		if v.Pattern != "" {
			re, err := regexp.Compile(v.Pattern)
			if err != nil {
				return fmt.Errorf("%s: pattern: %w", v.Name, err)
			}
			v.re = re
		}
		for _, expr := range []*string{&v.Command, &v.Prefill, &v.Apply} {
			resolved, err := shell(s.Dir, *expr)
			if err != nil {
				return fmt.Errorf("%s: %w", v.Name, err)
			}
			*expr = resolved
		}
		s.byName[v.Name] = v
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

// checkPresets settles the starting points. A preset is named by its title and
// nothing else: it is a page with rows on it, and there is nothing about it a
// module ever has to point at from somewhere else.
func (s *Module) checkPresets() error {
	for i, p := range s.Presets {
		where := fmt.Sprintf("preset %d", i+1)
		switch {
		case p.Title == "":
			return fmt.Errorf("%s: title is required", where)
		case len(p.Options) == 0:
			// A page with nothing on it to choose would be a page nobody can
			// get past, which is an authoring mistake rather than a way of
			// turning the page off: leaving the whole preset out is that.
			return fmt.Errorf("%s: no options", p.Title)
		}
		for j, o := range p.Options {
			if o.Title == "" {
				return fmt.Errorf("%s: option %d: title is required", p.Title, j+1)
			}
			for name := range o.Values {
				if s.byName[name] == nil {
					return fmt.Errorf("%s: %s: no such variable: %s", p.Title, o.Title, name)
				}
			}
			if err := s.checkFetch(o); err != nil {
				return fmt.Errorf("%s: %s: %w", p.Title, o.Title, err)
			}
		}
	}
	return nil
}

// checkFetch settles a preset option's `asks:` and `apply:` — the starting
// point that is fetched rather than written out here.
//
// The question is put on a page of its own, so unlike a task's it may be a text
// box: a code somebody was handed is typed, not chosen. A secret is refused,
// because a starting point is a set of answers and a secret is never one of
// them.
func (s *Module) checkFetch(o *PresetOption) error {
	if o.Asks == "" {
		if o.Apply != "" {
			return fmt.Errorf("apply: there is no asks for it to work from")
		}
		return nil
	}
	v := s.byName[o.Asks]
	switch {
	case v == nil:
		return fmt.Errorf("asks: no such variable: %s", o.Asks)
	case v.Secret():
		return fmt.Errorf("asks: %s is a secret, which is asked for immediately before the run", o.Asks)
	}
	resolved, err := shell(s.Dir, o.Apply)
	if err != nil {
		return err
	}
	o.Apply = resolved
	v.deferred = true
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

// scriptFile is the file a field names rather than holds: a single line
// beginning with ./ or ../ names one, anything else is the shell itself. So a
// one-line option list stays in the yaml where it is read together with the
// variable, and a long one moves into a file beside it — without a second
// notation to learn.
//
// A path is relative to the folder of the yaml it was written in, which is the
// only place somebody reading that line can be looking.
func scriptFile(dir, expr string) (string, error) {
	trimmed := strings.TrimSpace(expr)
	if trimmed == "" || strings.Contains(trimmed, "\n") {
		return "", nil
	}
	if !strings.HasPrefix(trimmed, "./") && !strings.HasPrefix(trimmed, "../") {
		return "", nil
	}
	path := filepath.Join(dir, trimmed)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("no such script: %s", trimmed)
	}
	return path, nil
}

// shell settles a field that may hold either shell or the file it lives in.
func shell(dir, expr string) (string, error) {
	path, err := scriptFile(dir, expr)
	switch {
	case err != nil:
		return "", err
	case path == "":
		return expr, nil
	}
	return source(path), nil
}

// quote wraps a path for the shell, so a module whose name holds a space or a
// quote is still one word.
func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
