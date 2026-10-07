package spec

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/murkl/oak/internal/i18n"
)

// FileRuntime is the one yaml beside the binary that is not a module, and
// DirModules the folder beside it that holds them.
//
// Both have reserved names because they are what the binary reads to know what
// it is: everything a run needs before a module has been chosen — the product's
// name, its wordmark, its one colour — and the modules it offers.
const (
	FileRuntime = "oak.yaml"
	DirModules  = "modules"
)

// FileRuntimeShell is the product's one shell, beside oak.yaml: the library
// every script of every module is given first, and the only place they share
// code - a function two tasks, two actions or two modules would otherwise each
// carry a copy of. Being there is the declaration.
const FileRuntimeShell = "oak.sh"

// Runtime is the whole of what a binary and the folders beside it add up to.
//
// It is what no module can answer for itself. An installer and a recovery of
// one product are two programs and one product — the same wordmark on the way
// in, the same colour on every page, the same name over the question of which
// of them to open — and a module that declared any of that would be declaring
// it for its neighbours as well.
//
// It is also the whole of what makes this binary *this* product rather than
// another one. Nothing about any particular operating system is compiled in: a
// different name, a different colour and a different folder of modules is a
// different product, out of the same binary.
type Runtime struct {
	// Title is the product, over the page that asks which of its modules to
	// open. Not translated: it is a name, and the same one in every language.
	Title string `yaml:"title"`

	// Accent is the one colour everything on screen is built from, #rrggbb, and
	// Logo the wordmark the interface comes up out of — everything above the
	// first blank line a dim eyebrow over it.
	Accent string `yaml:"accent"`
	Logo   string `yaml:"logo"`

	// Icon is what stands beside the words over every module's menu, in the
	// accent, in place of the runtime's tick. A module may say its own.
	Icon string `yaml:"icon"`

	// Version is what this product calls this build of itself, under the
	// wordmark on the way in and in the corner of every page. It is the product's own and not the binary's: a release of the
	// modules is what somebody downloads, and which Oak drove it is a dependency
	// of that rather than its name.
	//
	// Left out, no version is shown. Oak's own is what `oak --version` answers
	// first, and it is never put on screen as though it were the product's.
	Version string `yaml:"version"`

	// Status is the line opposite the name in every module's header, unless a
	// module says one of its own — see Status.
	Status *Status `yaml:"status"`

	// Modules is what this runtime offers, in the order it offers them: the
	// folders in DirModules, by name.
	//
	// Read off the filesystem rather than written down here, because a list
	// beside the folders is a second copy of them that can disagree. Adding a
	// module is a folder and taking one away is deleting it, and what each of
	// them is called on the page that offers it is in its own declaration.
	Modules []string `yaml:"-"`

	// Where this was read: the file itself, and the folder its modules sit in.
	File string `yaml:"-"`
	Dir  string `yaml:"-"`

	// Shell is FileRuntimeShell where the product has one, and empty where it
	// has not, and defined the functions it holds.
	Shell   string `yaml:"-"`
	defined names
}

// LoadRuntime reads the declaration beside the binary, or in the folder named
// outright, and finds the modules next to it.
//
// Both are required. A binary with no oak.yaml beside it is not a product yet:
// it has no name, no colours and nothing to offer, and saying so at startup is
// better than coming up blank.
func LoadRuntime(explicit string) (*Runtime, error) {
	dir, err := root(explicit)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, FileRuntime)
	var r Runtime
	if err := read(path, &r); err != nil {
		if os.IsNotExist(err) {
			return nil, missing(i18n.T("A %s file has to sit next to this program, in %s.", FileRuntime, dir))
		}
		return nil, err
	}
	r.File, r.Dir = path, dir
	r.Shell = beside(dir, FileRuntimeShell)
	// An action is a module's own; one beside oak.yaml would be a second place
	// to look for a name, and what modules share is a function in oak.sh.
	if beside(dir, DirActions) != "" {
		return nil, fmt.Errorf("%s/: an action is a module's own, a folder under its %s/ - what modules share is a function in %s", DirActions, DirActions, FileRuntimeShell)
	}
	if err := r.check(); err != nil {
		return nil, err
	}
	if err := r.Status.settle(dir, FileRuntime); err != nil {
		return nil, fmt.Errorf("%s: %w", FileRuntime, err)
	}
	if r.defined, err = functions(r.Shell); err != nil {
		return nil, err
	}
	if r.Status != nil && r.Status.calls != "" {
		if err := checkCalls(map[string]string{r.Status.calls: FileRuntime + ": status: check"}, r.defined); err != nil {
			return nil, err
		}
	}
	if r.Logo, err = r.picture(r.Logo, FileRuntime+": logo"); err != nil {
		return nil, err
	}
	if r.Icon, err = r.picture(r.Icon, FileRuntime+": icon"); err != nil {
		return nil, err
	}
	if r.Modules, err = discover(filepath.Join(dir, DirModules)); err != nil {
		return nil, err
	}
	return &r, nil
}

// ProductVersion is the version the product beside the binary, or in the folder
// named outright, calls itself by. Empty where there is no product, or it names
// none: a binary on its own is a perfectly good thing to ask the version of, so
// the one file is read and nothing else is loaded.
func ProductVersion(explicit string) (string, error) {
	dir, err := root(explicit)
	if err != nil {
		return "", err
	}
	var r Runtime
	if err := read(filepath.Join(dir, FileRuntime), &r); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return r.Version, nil
}

func (r *Runtime) check() error {
	if r.Accent != "" && !hexColor.MatchString(r.Accent) {
		return fmt.Errorf("%s: accent must be #rrggbb, got %q", FileRuntime, r.Accent)
	}
	return nil
}

// discover is the modules in a folder, in name order — which is the order they
// are offered in. Every folder in it is one; whether it holds something that
// loads is decided by loading it, so a broken module is a startup error rather
// than a row quietly missing from the page.
func discover(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	var out []string
	for _, entry := range entries {
		if entry.IsDir() {
			out = append(out, entry.Name())
		}
	}
	if len(out) == 0 {
		return nil, missing(i18n.T("A %s folder with something in it has to sit next to this program, in %s.", DirModules, filepath.Dir(dir)))
	}
	return out, nil
}

// missing is how a run says it was started somewhere that holds no product.
func missing(detail string) error {
	return fmt.Errorf("%s\n%s", i18n.T("Nothing to run here."), detail)
}

// Path is where a module's folder is.
func (r *Runtime) Path(id string) string { return filepath.Join(r.Dir, DirModules, id) }

// LoadModules reads every module this runtime offers, in the order it offers
// them.
//
// All of them, whichever one a run turns out to be about. A release ships its
// modules together, so one that will not load is a broken release, and saying
// so at startup beats a row that fails when somebody chooses it.
func (r *Runtime) LoadModules() ([]*Module, error) {
	out := make([]*Module, 0, len(r.Modules))
	for _, id := range r.Modules {
		mod, err := Load(r.Path(id))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", id, err)
		}
		if err := checkCalls(mod.calls, r.defined); err != nil {
			return nil, fmt.Errorf("%s: %w", id, err)
		}
		if mod.UI.Icon, err = r.picture(mod.UI.Icon, FileModule+": icon"); err != nil {
			return nil, fmt.Errorf("%s: %w", id, err)
		}
		mod.Shell = r.Shell
		if mod.Status == nil && r.Status != nil {
			// Named from the module's folder, the way a translator's template
			// names every other file a string of the module came out of.
			from, err := filepath.Rel(mod.Dir, r.File)
			if err != nil {
				return nil, err
			}
			inherited := *r.Status
			inherited.file = filepath.ToSlash(from)
			mod.Status = &inherited
		}
		out = append(out, mod)
	}
	answered := names{}
	for _, mod := range out {
		for _, v := range mod.Declared() {
			answered[v.Name] = true
		}
	}
	for _, mod := range out {
		mod.answered = answered
	}
	return out, nil
}

// root is where a run looks for everything it was shipped with: the folder
// named outright, or the one the binary is in.
func root(explicit string) (string, error) {
	dir := explicit
	if dir == "" {
		dir = binaryDir()
	}
	return filepath.Abs(dir)
}

// Status is one thing about the machine the header keeps an eye on while a
// module is open — whether it is online, most of all. Shell run every so often,
// and the few words it reads as while that shell says yes and while it says no.
//
// It stands opposite the name on the pages where nothing else does: the mark
// that turns while something runs takes its place, and so does a page's own
// count. The two marks it is shown with are Oak's, drawn from the same set as
// every other mark, so a console font that holds the interface holds them too.
//
// Declared once in oak.yaml for every module of the product, and in a module's
// own declaration for that module alone, which replaces the product's outright.
type Status struct {
	// Check is a function of oak.sh, or a file, whose exit status is the
	// answer: zero for yes. Run with the product's shell loaded and the answers
	// in the environment, like everything else a module runs.
	Check string `yaml:"check"`

	// Every is how many seconds lie between two runs of it. Left out, ten.
	Every int `yaml:"every"`

	// Pass and Fail are what the line reads while the script says yes and while
	// it says no. Either may be left out, and the mark alone says it.
	Pass string `yaml:"pass"`
	Fail string `yaml:"fail"`

	// file is where it was declared, as the module's folder names it, and
	// calls the function of oak.sh its check names, if it names one.
	file  string
	calls string
}

// statusEvery is how often a status is read where its declaration says nothing:
// often enough for a cable plugged in to show before anybody wonders, rarely
// enough that a check going out to the network is not a load of its own.
const statusEvery = 10

// settle checks a status over and resolves its script against the folder it
// was declared in. Nothing declared is nothing to check.
func (st *Status) settle(dir, file string) error {
	if st == nil {
		return nil
	}
	switch {
	case strings.TrimSpace(st.Check) == "":
		return fmt.Errorf("status: check is what the status is read with, and it is missing")
	case st.Every < 0:
		return fmt.Errorf("status: every is a number of seconds, and %d is not one", st.Every)
	}
	run, fn, err := shell(dir, st.Check)
	if err != nil {
		return fmt.Errorf("status: check: %w", err)
	}
	st.Check, st.calls, st.file = run, fn, file
	return nil
}

// Interval is how long lies between two reads of it.
func (st *Status) Interval() time.Duration {
	if st.Every == 0 {
		return statusEvery * time.Second
	}
	return time.Duration(st.Every) * time.Second
}

// Words is what the line reads as for an answer, translated.
func (st *Status) Words(pass bool) string {
	if pass {
		return i18n.T(st.Pass)
	}
	return i18n.T(st.Fail)
}

// picture is a field that draws something: as written, or as the function of
// oak.sh it names prints it. Run once, here, so a picture that cannot be drawn
// stops the start rather than a page.
func (r *Runtime) picture(raw, where string) (string, error) {
	m := call.FindStringSubmatch(strings.TrimSpace(raw))
	if m == nil {
		return raw, nil
	}
	if err := checkCalls(map[string]string{m[1]: where}, r.defined); err != nil {
		return "", err
	}
	out, err := exec.Command("bash", "-c", source(r.Shell)+" && "+m[1]).Output()
	if err != nil {
		return "", fmt.Errorf("%s: %s() failed: %w", where, m[1], err)
	}
	if strings.TrimSpace(string(out)) == "" {
		return "", fmt.Errorf("%s: %s() printed nothing", where, m[1])
	}
	return string(out), nil
}
