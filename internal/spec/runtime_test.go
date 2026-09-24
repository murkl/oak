package spec

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// writeRuntime lays a runtime out the way a build does: the declaration beside
// a modules folder, one folder in it per module.
func writeRuntime(t *testing.T, declaration string, modules ...string) string {
	t.Helper()
	dir := t.TempDir()
	if declaration != "" {
		if err := os.WriteFile(filepath.Join(dir, FileRuntime), []byte(declaration), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range modules {
		sub := filepath.Join(dir, DirModules, name)
		if err := os.MkdirAll(filepath.Join(sub, DirTasks, Stage("go"), "do"), 0o755); err != nil {
			t.Fatal(err)
		}
		write := func(path, body string) {
			if err := os.WriteFile(filepath.Join(sub, path), []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		write(FileModule, "title: The "+name+"\nstages: [go]\n")
		write(filepath.Join(DirTasks, Stage("go"), "do", FileTask), "title: Do it\n")
		write(filepath.Join(DirTasks, Stage("go"), "do", FileTaskScript), "echo hi\n")
	}
	return dir
}

const testRuntime = "title: Test OS\n"

func TestLoadRuntimeReadsWhatEveryModuleShares(t *testing.T) {
	dir := writeRuntime(t, "title: Test OS\naccent: \"#1793d1\"\nlogo: |\n  Test\n", "installer")

	rt, err := LoadRuntime(dir)
	if err != nil {
		t.Fatal(err)
	}
	if rt.Title != "Test OS" {
		t.Errorf("name = %q, want Test OS", rt.Title)
	}
	if rt.Accent != "#1793d1" {
		t.Errorf("accent = %q", rt.Accent)
	}
	if rt.Logo != "Test\n" {
		t.Errorf("logo = %q", rt.Logo)
	}
	if rt.File != filepath.Join(dir, FileRuntime) {
		t.Errorf("file = %q", rt.File)
	}
}

// The modules folder is the whole of what is on offer — so a module is added by
// dropping a folder in it and taken away by deleting one, with no list anywhere
// to keep in step.
func TestEveryFolderInTheModulesDirectoryIsOffered(t *testing.T) {
	dir := writeRuntime(t, testRuntime, "recovery", "installer")

	rt, err := LoadRuntime(dir)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(rt.Modules, " ") != "installer recovery" {
		t.Errorf("modules = %v, want every folder there, by name", rt.Modules)
	}
	if got := rt.Path("installer"); got != filepath.Join(dir, DirModules, "installer") {
		t.Errorf("Path() = %q, want the folder in %s", got, DirModules)
	}
}

// A binary with nothing beside it is not a product yet, and saying so is better
// than coming up blank.
func TestARuntimeWithNoDeclarationWillNotStart(t *testing.T) {
	dir := writeRuntime(t, "", "installer")

	if _, err := LoadRuntime(dir); err == nil {
		t.Fatal("a folder with no oak.yaml in it started anyway")
	}
}

func TestARuntimeThatOffersNothingWillNotStart(t *testing.T) {
	dir := writeRuntime(t, testRuntime)

	if _, err := LoadRuntime(dir); err == nil {
		t.Fatal("a runtime with no modules beside it was accepted")
	} else if !strings.Contains(err.Error(), DirModules) {
		t.Errorf("error = %q, want it to name the folder it looked in", err)
	}
}

func TestLoadRuntimeRefusesAnAccentThatIsNotAColour(t *testing.T) {
	dir := writeRuntime(t, "title: Test OS\naccent: blue\n", "installer")

	if _, err := LoadRuntime(dir); err == nil {
		t.Fatal("blue was accepted as a colour")
	} else if !strings.Contains(err.Error(), "accent must be") {
		t.Errorf("error = %q, want it to mention the accent", err)
	}
}

// The module's identity is its folder: what the page offering it is keyed on,
// what the command line names, and what its answers are kept under are one word.
func TestAModuleIsIdentifiedByItsFolder(t *testing.T) {
	dir := writeRuntime(t, testRuntime, "installer", "recovery")

	mod, err := Load(filepath.Join(dir, DirModules, "recovery"))
	if err != nil {
		t.Fatal(err)
	}
	if mod.ID() != "recovery" {
		t.Errorf("ID() = %q, want recovery", mod.ID())
	}
}

// The product's own shell is found by its name beside oak.yaml and handed to
// every module in front of the module's own, which may then build on it.
func TestEveryModuleIsGivenTheProductsShellBeforeItsOwn(t *testing.T) {
	dir := writeRuntime(t, testRuntime, "installer", "recovery")
	shared := filepath.Join(dir, FileRuntimeShell)
	if err := os.WriteFile(shared, []byte("shared() { :; }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	own := filepath.Join(dir, DirModules, "installer", FileShell)
	if err := os.WriteFile(own, []byte("own() { :; }\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	rt, err := LoadRuntime(dir)
	if err != nil {
		t.Fatal(err)
	}
	mods, err := rt.LoadModules()
	if err != nil {
		t.Fatal(err)
	}

	if got := strings.Join(mods[0].Shells(), " "); got != shared+" "+own {
		t.Errorf("installer shells = %q, want the product's, then its own", got)
	}
	if got := strings.Join(mods[1].Shells(), " "); got != shared {
		t.Errorf("recovery shells = %q, want the product's alone", got)
	}
}

// A product without one hands its modules nothing extra, the same as before
// the file existed.
func TestAProductWithoutAShellOfItsOwnSharesNothing(t *testing.T) {
	rt, err := LoadRuntime(writeRuntime(t, testRuntime, "installer"))
	if err != nil {
		t.Fatal(err)
	}
	mods, err := rt.LoadModules()
	if err != nil {
		t.Fatal(err)
	}
	if got := mods[0].Shells(); len(got) != 0 {
		t.Errorf("shells = %q, want none", got)
	}
}

// A name the product's shell sets is answered for every module of it, the same
// as one the module's own sets.
func TestANameTheProductsShellSetsIsNotUnset(t *testing.T) {
	dir := writeRuntime(t, testRuntime, "installer")
	if err := os.WriteFile(filepath.Join(dir, FileRuntimeShell), []byte("SHARED=yes\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	task := filepath.Join(dir, DirModules, "installer", DirTasks, Stage("go"), "do", FileTaskScript)
	if err := os.WriteFile(task, []byte("echo \"$SHARED\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rt, err := LoadRuntime(dir)
	if err != nil {
		t.Fatal(err)
	}
	mods, err := rt.LoadModules()
	if err != nil {
		t.Fatal(err)
	}

	unset, err := mods[0].Unset()

	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(unset, "SHARED") {
		t.Errorf("unset = %v, want SHARED answered by the product's shell", unset)
	}
}
