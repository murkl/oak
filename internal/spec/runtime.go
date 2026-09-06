package spec

import (
	"fmt"
	"os"
	"path/filepath"

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

	// Version is what this product calls this build of itself, under the
	// wordmark and in the corner of every page. It is the product's own and not
	// the binary's: a release of the modules is what somebody downloads, and
	// which Oak drove it is a dependency of that rather than its name.
	//
	// Left out, no version is shown. Oak's own is what `oak --version` answers
	// and is never put on screen as though it were the product's.
	Version string `yaml:"version"`

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
	if err := r.check(); err != nil {
		return nil, err
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
