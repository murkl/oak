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

// The product's shell is found by its name beside oak.yaml and handed to every
// module of it.
func TestEveryModuleIsGivenTheProductsShell(t *testing.T) {
	dir := writeRuntime(t, testRuntime, "installer", "recovery")
	shared := filepath.Join(dir, FileRuntimeShell)
	if err := os.WriteFile(shared, []byte("shared() { :; }\n"), 0o600); err != nil {
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

	for _, mod := range mods {
		if mod.Shell != shared {
			t.Errorf("%s shell = %q, want the product's", mod.ID(), mod.Shell)
		}
	}
}

// A product without one hands its modules nothing extra.
func TestAProductWithoutAShellSharesNothing(t *testing.T) {
	rt, err := LoadRuntime(writeRuntime(t, testRuntime, "installer"))
	if err != nil {
		t.Fatal(err)
	}
	mods, err := rt.LoadModules()
	if err != nil {
		t.Fatal(err)
	}
	if mods[0].Shell != "" {
		t.Errorf("shell = %q, want none", mods[0].Shell)
	}
}

// A name the product's shell sets is answered for every module of it, the same
// as one a module sets itself.
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

// The product's shell runs for every module of it, so a name it reads for one of
// them is no gap in another. A name no module of the product answers still is.
func TestANameTheProductsShellReadsForAnotherModuleIsNotUnset(t *testing.T) {
	dir := writeRuntime(t, testRuntime, "installer", "recovery")
	if err := os.WriteFile(filepath.Join(dir, FileRuntimeShell), []byte("disk() { echo \"$DISK $GONE\"; }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	declare := "title: The installer\nstages: [go]\nvariables:\n  - name: DISK\n    type: text\n    title: Disk\n"
	if err := os.WriteFile(filepath.Join(dir, DirModules, "installer", FileModule), []byte(declare), 0o600); err != nil {
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

	unset, err := mods[1].Unset()

	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(unset, []string{"GONE"}) {
		t.Errorf("recovery unset = %v, want GONE alone: DISK is the installer's", unset)
	}
}

// The header's status is the product's for every module that says nothing of
// its own, and its words are in that module's template — a module's catalog is
// what the line is read through while the module is open.
func TestAModuleWithoutAStatusOfItsOwnTakesTheProducts(t *testing.T) {
	dir := writeRuntime(t, testRuntime+"status:\n  check: is_online()\n  every: 5\n  pass: Online\n  fail: Offline\n", "installer")
	writeShell(t, dir, "is_online() { return 0; }\n")
	rt, err := LoadRuntime(dir)
	if err != nil {
		t.Fatal(err)
	}
	mods, err := rt.LoadModules()
	if err != nil {
		t.Fatal(err)
	}
	st := mods[0].Status
	if st == nil || st.Check != "is_online" || st.Words(true) != "Online" || st.Interval().Seconds() != 5 {
		t.Fatalf("status = %+v, want the product's", st)
	}
	var files []string
	for _, m := range mods[0].Messages() {
		if m.Text == "Offline" {
			files = m.Files
		}
	}
	if want := []string{"../../" + FileRuntime}; !slices.Equal(files, want) {
		t.Errorf("Offline is said in %v, want %v", files, want)
	}
}

// A module that declares one has that one, and none of the product's.
func TestAModulesOwnStatusReplacesTheProducts(t *testing.T) {
	dir := writeRuntime(t, testRuntime+"status:\n  check: is_online()\n  pass: Online\n", "installer")
	writeShell(t, dir, "is_online() { return 0; }\nis_up() { return 0; }\n")
	decl := filepath.Join(dir, DirModules, "installer", FileModule)
	if err := os.WriteFile(decl, []byte("title: The installer\nstages: [go]\nstatus:\n  check: is_up()\n  fail: Down\n"), 0o600); err != nil {
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
	st := mods[0].Status
	if st.Check != "is_up" || st.Words(true) != "" || st.Words(false) != "Down" || st.Interval().Seconds() != statusEvery {
		t.Errorf("status = %+v, want the module's own and nothing of the product's", st)
	}
}

// A status is read with its check, so one without is refused where it was
// written rather than showing nothing and saying nothing about why.
func TestAStatusWithoutACheckIsRefused(t *testing.T) {
	_, err := LoadRuntime(writeRuntime(t, testRuntime+"status:\n  pass: Online\n", "installer"))
	if err == nil || !strings.Contains(err.Error(), "check is what the status is read with") {
		t.Errorf("err = %v, want a status without a check refused", err)
	}
}

// writeShell puts the product's oak.sh beside its declaration.
func writeShell(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, FileRuntimeShell), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// A function the yaml calls is held to the oak.sh it would be called out of,
// so a typo is refused at startup, naming the line that made it, rather than
// being a command not found in the middle of a run.
func TestAFunctionTheYamlCallsIsHeldToTheProductsShell(t *testing.T) {
	dir := writeRuntime(t, testRuntime, "installer")
	decl := filepath.Join(dir, DirModules, "installer", FileModule)
	body := "title: The installer\nstages: [go]\nvariables:\n  - name: DISK\n    type: list\n    title: Disk\n    options-from: list_disk()\n"
	if err := os.WriteFile(decl, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	writeShell(t, dir, "list_disks() {\n    lsblk\n}\n")
	rt, err := LoadRuntime(dir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.LoadModules()
	want := "installer: module.yaml: DISK: options-from: list_disk() is not a function in oak.sh"
	if err == nil || err.Error() != want {
		t.Errorf("err = %v, want %q", err, want)
	}
}

// Either way bash opens a function is one, and the call runs it by name.
func TestAFunctionOfTheProductsShellIsCalledByName(t *testing.T) {
	dir := writeRuntime(t, testRuntime, "installer")
	decl := filepath.Join(dir, DirModules, "installer", FileModule)
	body := "title: The installer\nstages: [go]\nvariables:\n" +
		"  - name: DISK\n    type: list\n    title: Disk\n    options-from: list_disks()\n" +
		"  - name: ZONE\n    type: text\n    title: Zone\n    prefill: guess_zone()\n"
	if err := os.WriteFile(decl, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	writeShell(t, dir, "list_disks() { lsblk; }\nfunction guess_zone {\n    echo UTC\n}\n")
	rt, err := LoadRuntime(dir)
	if err != nil {
		t.Fatal(err)
	}
	mods, err := rt.LoadModules()
	if err != nil {
		t.Fatal(err)
	}
	if got := mods[0].Var("DISK").OptionsFrom; got != "list_disks" {
		t.Errorf("options-from = %q, want the function called by name", got)
	}
	if got := mods[0].Var("ZONE").Prefill; got != "guess_zone" {
		t.Errorf("prefill = %q, want the function called by name", got)
	}
}

// The product's own status is held to the same shell, before any module loads.
func TestTheProductsStatusIsHeldToItsShell(t *testing.T) {
	dir := writeRuntime(t, testRuntime+"status:\n  check: is_online()\n", "installer")
	_, err := LoadRuntime(dir)
	want := "oak.yaml: status: check: is_online() is not a function in oak.sh"
	if err == nil || err.Error() != want {
		t.Errorf("err = %v, want %q", err, want)
	}
}

// An action is a module's own. One beside oak.yaml would be a second place to
// look a name up in, and an older product that has one is told where its work
// goes instead of losing it without a word.
func TestAnActionsFolderBesideTheProductIsRefused(t *testing.T) {
	dir := writeRuntime(t, testRuntime, "installer")
	if err := os.MkdirAll(filepath.Join(dir, DirActions, "restart"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := LoadRuntime(dir)

	if err == nil || !strings.Contains(err.Error(), "an action is a module's own") {
		t.Errorf("err = %v, want the product's actions/ refused", err)
	}
}

// A picture is drawn as written, or as the function of oak.sh it names prints
// it, once, while the product loads. A module's icon is read the same way.
func TestAPictureIsPrintedByTheFunctionItNames(t *testing.T) {
	dir := writeRuntime(t, testRuntime+"logo: logo_tux()\nicon: |\n  /\\\n", "installer")
	writeShell(t, dir, "logo_tux() { printf 'TUX\\n'; }\nicon_tux() { printf '<>\\n'; }\n")
	decl := filepath.Join(dir, DirModules, "installer", FileModule)
	if err := os.WriteFile(decl, []byte("title: The installer\nstages: [go]\nicon: icon_tux()\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rt, err := LoadRuntime(dir)
	if err != nil {
		t.Fatal(err)
	}
	if rt.Logo != "TUX\n" {
		t.Errorf("logo = %q, want what logo_tux printed", rt.Logo)
	}
	if rt.Icon != "/\\\n" {
		t.Errorf("icon = %q, want it as written", rt.Icon)
	}
	mods, err := rt.LoadModules()
	if err != nil {
		t.Fatal(err)
	}
	if got := mods[0].UI.Icon; got != "<>\n" {
		t.Errorf("module icon = %q, want what icon_tux printed", got)
	}
}

// A picture that cannot be drawn stops the start, rather than a page.
func TestAPictureThatCannotBeDrawnStopsTheStart(t *testing.T) {
	for name, c := range map[string]struct{ shell, want string }{
		"not defined":    {"true\n", "oak.yaml: icon: icon_tux() is not a function in oak.sh"},
		"failing":        {"icon_tux() { false; }\n", "oak.yaml: icon: icon_tux() failed: exit status 1"},
		"printing blank": {"icon_tux() { echo; }\n", "oak.yaml: icon: icon_tux() printed nothing"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := writeRuntime(t, testRuntime+"icon: icon_tux()\n", "installer")
			writeShell(t, dir, c.shell)
			_, err := LoadRuntime(dir)
			if err == nil || err.Error() != c.want {
				t.Errorf("err = %v, want %q", err, c.want)
			}
		})
	}
}
