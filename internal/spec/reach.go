package spec

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// The one kind of authoring mistake a module's shape does not rule out on its
// own.
//
// Everywhere else the same fact is written down once: the order comes out of
// the stages and the needs, the settings page comes out of the variables, the
// translation template comes out of the module. A guard is the exception — a
// question says when it is worth asking and a task says when it is worth
// running, and the two are separate sentences in separate files that are meant
// to agree. When they drift, everything still loads and everything still runs;
// the only symptom is a question somebody answered that nothing acted on.
//
// So it is checked here instead. Not at load: a module that behaves is not a module
// that refuses to start, and a released image must never turn a lint into a
// machine that will not boot. `tools/inspect` reports it and `make check` runs
// that, which puts it in front of whoever wrote the guard, before the commit.

// Unread is a question asked where nothing reads the answer.
type Unread struct {
	// Var is the question. Task is the one that reads it and cannot run where
	// it is asked, empty where nothing reads it at all, and Need the condition
	// that task carries and the question does not.
	Var  string
	Task string
	Need string
}

func (u Unread) String() string {
	if u.Task == "" {
		return fmt.Sprintf("%s: nothing in this module reads this answer", u.Var)
	}
	return fmt.Sprintf("%s is asked where %s/%s cannot run: %s", u.Var, DirTasks, u.Task, u.Need)
}

// Unread lists them, in the order the questions were declared.
//
// A question is unread when every task that reads it carries a guard the
// question does not: turn the desktop extras off and the row is still offered,
// still answered, and the task that would act on it is not in the run. One task
// that can run wherever the question is asked is enough for the answer to mean
// something, so a value several tasks read is only reported when none of them
// can.
//
// Read outside a task — in the shared library, a hook, or the declaration's own
// shell — and there is nothing to compare against: those run whatever the
// answers say, so the question is answered by definition.
func (s *Module) Unread() ([]Unread, error) {
	free, byTask, err := s.readers()
	if err != nil {
		return nil, err
	}
	var out []Unread
	for _, v := range s.Vars {
		if free[v.Name] {
			continue
		}
		tasks := byTask[v.Name]
		if len(tasks) == 0 {
			out = append(out, Unread{Var: v.Name})
			continue
		}
		if gap, found := s.unreachable(v, tasks); found {
			out = append(out, gap)
		}
	}
	return out, nil
}

// unreachable is the first task that reads this question and cannot run where
// it is asked, and whether every one of them is like that.
//
// A condition about the question itself is passed over: a task guarded on the
// very value being asked for — `AUR_HELPER != none` — is not a task that runs
// somewhere else, it is the answer being acted on.
func (s *Module) unreachable(v *Variable, tasks []*Task) (Unread, bool) {
	var first Unread
	for _, t := range tasks {
		need := ""
		for _, c := range t.cond {
			if c.name == v.Name || s.implies(v.cond, c) {
				continue
			}
			need = c.String()
			break
		}
		if need == "" {
			return Unread{}, false
		}
		if first.Var == "" {
			first = Unread{Var: v.Name, Task: t.ID(), Need: need}
		}
	}
	return first, true
}

// readers is where each declared variable is named as a value: the tasks whose
// own files name it, and whether anything outside a task does.
func (s *Module) readers() (free map[string]bool, byTask map[string][]*Task, err error) {
	free, byTask = map[string]bool{}, map[string][]*Task{}
	declared := map[string]bool{}
	for _, v := range s.Vars {
		declared[v.Name] = true
	}

	for _, path := range s.everywhere() {
		names, err := named(path, declared)
		if err != nil {
			return nil, nil, err
		}
		for name := range names {
			free[name] = true
		}
	}

	// And each task, whole: its script, where it says it belongs, and every
	// file it ships with.
	for _, t := range s.Tasks {
		names, err := namedUnder(filepath.Dir(t.Path()), declared)
		if err != nil {
			return nil, nil, err
		}
		for _, name := range t.reads() {
			if declared[name] {
				names[name] = true
			}
		}
		for name := range names {
			byTask[name] = append(byTask[name], t)
		}
	}
	return free, byTask, nil
}

// everywhere is every file of this module that runs whatever the answers say:
// the declaration and its shell, the shared library, and each hook. Nothing
// here is guarded by anything, so a value one of them reads is read on every
// run there is.
func (s *Module) everywhere() []string {
	out := []string{filepath.Join(s.Dir, s.File)}
	if s.Lib != "" {
		out = append(out, s.Lib)
	}
	for _, name := range HookNames {
		if path := s.Hook(name); path != "" {
			out = append(out, path)
		}
	}
	return out
}

// reads is what a task consumes by declaring it rather than by naming it in
// shell: a guard is an answer being read — it is what decides whether this task
// runs at all — and so are the values it stops the run to ask for and to show.
func (t *Task) reads() []string {
	out := make([]string, 0, len(t.cond)+2)
	for _, c := range t.cond {
		out = append(out, c.name)
	}
	return append(out, t.Asks, t.Shows)
}

// reference is a variable being read rather than written down: shell's $NAME
// and ${NAME}, and the {{NAME}} a sentence is filled in by. A bare word is a
// name being declared or a condition naming one, and neither is a read.
var reference = regexp.MustCompile(`\$\{?([A-Za-z_][A-Za-z0-9_]*)|\{\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)

// named is the declared variables one file reads.
func named(path string, declared map[string]bool) (map[string]bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, m := range reference.FindAllStringSubmatch(string(raw), -1) {
		name := m[1] + m[2] // one of the two alternatives matched, the other is empty
		if declared[name] {
			out[name] = true
		}
	}
	return out, nil
}

// namedUnder is the same for a whole folder, which is what a task is.
func namedUnder(dir string, declared map[string]bool) (map[string]bool, error) {
	out := map[string]bool{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		names, err := named(path, declared)
		if err != nil {
			return err
		}
		for name := range names {
			out[name] = true
		}
		return nil
	})
	return out, err
}

// String renders a condition the way the yaml wrote it, which is how it has to
// read back in a report: somebody is about to go and find that line.
func (c *condition) String() string {
	op := "!="
	if c.equal {
		op = "=="
	}
	return strings.Join([]string{c.name, op, c.want}, " ")
}

// The other direction, and the other half of the same question.
//
// Unread is a question nothing reads. Unset is a read nothing answers: a name
// the module's own shell reaches for that this module does not declare, does
// not set anywhere itself, and that Oak does not put in the environment either.
//
// In shell that is not an error. An unset name is an empty string, the line
// runs, and what comes out the far end is a path with a hole in it — which is
// exactly what happens to a module written against an older Oak that used to be
// handed more than it is now.
//
// It is a description rather than a verdict, and it is not a list of mistakes:
// $HOME and $PATH belong on it and are perfectly sound. Which of the two a name
// is takes a person a second and takes this program a list of every variable
// every Unix has ever had, so it says what it found and leaves it there.
//
// Only names in capitals are looked at, and what bash sets for itself is left
// out. A script's own working values are lower case by the convention every
// shell has kept, and treating either of those as a missing answer would bury
// the line that matters.
func (s *Module) Unset() ([]string, error) {
	read, own := map[string]bool{}, map[string]bool{}
	files := s.everywhere()
	for _, t := range s.Tasks {
		under, err := filesUnder(filepath.Dir(t.Path()))
		if err != nil {
			return nil, err
		}
		files = append(files, under...)
	}
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		text := string(raw)
		for _, m := range reference.FindAllStringSubmatch(text, -1) {
			if name := m[1] + m[2]; shouted.MatchString(name) {
				read[name] = true
			}
		}
		for _, re := range []*regexp.Regexp{assignment, binding} {
			for _, m := range re.FindAllStringSubmatch(text, -1) {
				own[m[1]] = true
			}
		}
	}
	var out []string
	for name := range read {
		if own[name] || bashOwn[name] || s.byName[name] != nil || runtimeVar(name) {
			continue
		}
		out = append(out, name)
	}
	slices.Sort(out)
	return out, nil
}

// bashOwn is what the shell sets before a script's first line, so a script
// reading one of these is reading something that is always there.
//
// A closed list out of the bash manual, and short on purpose: what the
// *environment* holds — HOME, PATH, TERM — is not here, because a module
// reading one of those is a module that expects something of the machine, and
// that is worth a line.
var bashOwn = map[string]bool{
	"BASH": true, "BASHPID": true, "BASH_COMMAND": true, "BASH_LINENO": true,
	"BASH_SOURCE": true, "BASH_SUBSHELL": true, "BASH_VERSION": true,
	"EUID": true, "FUNCNAME": true, "GROUPS": true, "HOSTNAME": true,
	"HOSTTYPE": true, "IFS": true, "LINENO": true, "MACHTYPE": true,
	"OLDPWD": true, "OPTARG": true, "OPTIND": true, "OSTYPE": true,
	"PIPESTATUS": true, "PPID": true, "PWD": true, "RANDOM": true,
	"REPLY": true, "SECONDS": true, "SHLVL": true, "UID": true,
}

// shouted is the convention that separates an answer from a script's own
// working value: an answer arrives as an environment variable, and an
// environment variable is written in capitals.
var shouted = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

// A name the module gives itself, which is answered by definition. Two shapes
// cover it: something put in front of an equals sign — plainly, or after
// export, local, declare or readonly, which all end in a space — and the two
// words that bind a name to what they read.
var (
	assignment = regexp.MustCompile(`(?:^|[\s;&|("'])([A-Z_][A-Z0-9_]*)\+?=`)
	binding    = regexp.MustCompile(`\b(?:for|read)\s+(?:-\S+\s+)*([A-Z_][A-Z0-9_]*)\b`)
)

// filesUnder is every file in a folder, which is what a task is.
func filesUnder(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		out = append(out, path)
		return nil
	})
	return out, err
}
