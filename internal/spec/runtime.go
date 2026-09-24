package spec

import (
	"fmt"
	"os"
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

// FileRuntimeShell is the shell every module of the product shares, beside
// oak.yaml: what two modules would otherwise each carry a copy of and have to
// keep in step by hand. It is loaded in front of each module's own module.sh,
// which may therefore build on it and override it. Being there is the
// declaration, the same as for module.sh.
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

	// Version is what this product calls this build of itself, in the corner of
	// every page. It is the product's own and not the binary's: a release of the
	// modules is what somebody downloads, and which Oak drove it is a dependency
	// of that rather than its name.
	//
	// Left out, no version is shown. Oak's own is what `oak --version` answers
	// and what the splash signs off with, and it is never put on screen as
	// though it were the product's.
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
	// has not.
	Shell string `yaml:"-"`
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
	if err := r.check(); err != nil {
		return nil, err
	}
	if err := r.Status.settle(dir, FileRuntime); err != nil {
		return nil, fmt.Errorf("%s: %w", FileRuntime, err)
	}
	if r.Modules, err = discover(filepath.Join(dir, DirModules)); err != nil {
		return nil, err
	}
	return &r, nil
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
		mod.Shared = r.Shell
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
	// Script is shell, or the file it lives in, whose exit status is the answer:
	// zero for yes. Run with the module's shell loaded and its answers in the
	// environment, like everything else a module runs.
	Script string `yaml:"script"`

	// Every is how many seconds lie between two runs of it. Left out, ten.
	Every int `yaml:"every"`

	// Pass and Fail are what the line reads while the script says yes and while
	// it says no. Either may be left out, and the mark alone says it.
	Pass string `yaml:"pass"`
	Fail string `yaml:"fail"`

	// file is where it was declared, as the module's folder names it.
	file string
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
	case strings.TrimSpace(st.Script) == "":
		return fmt.Errorf("status: script is what the status is read with, and it is missing")
	case st.Every < 0:
		return fmt.Errorf("status: every is a number of seconds, and %d is not one", st.Every)
	}
	script, err := shell(dir, st.Script)
	if err != nil {
		return fmt.Errorf("status: %w", err)
	}
	st.Script, st.file = script, file
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
