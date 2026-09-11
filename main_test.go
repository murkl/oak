package main

import (
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/murkl/oak/internal/i18n"
	"github.com/murkl/oak/internal/spec"
)

// writeModule writes the smallest module that will load, plus whatever extra
// files a test needs, and answers with the folder it put them in.
func writeModule(t *testing.T, declaration string, extra map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		spec.FileModule:             declaration,
		"tasks/@go/first/task.yaml": "title: First\n",
		"tasks/@go/first/task.sh":   "true\n",
	}
	maps.Copy(files, extra)
	for name, body := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// runtimeDecl is a product with nothing to say about itself but its name: what
// it offers is the folders beside it, so a test that is not about the runtime
// writes no more than this.
const runtimeDecl = "title: Test OS\n"

// runtime lays a whole product out: an oak.yaml over a modules folder, each
// module in it the smallest one that will load.
func runtime(t *testing.T, declaration string, modules ...string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, spec.FileRuntime), []byte(declaration), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range modules {
		put(t, dir, name, writeModule(t, "title: "+name+"\nstages: [go]\n", nil))
	}
	return dir
}

// around puts an oak.yaml over one module folder, so a module written on
// its own can be read the way the program reads one.
func around(t *testing.T, mod string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, spec.FileRuntime), []byte(runtimeDecl), 0o600); err != nil {
		t.Fatal(err)
	}
	put(t, dir, "installer", mod)
	return dir
}

// put moves a module folder into the place a runtime looks for it.
func put(t *testing.T, dir, name, mod string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, spec.DirModules), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(mod, filepath.Join(dir, spec.DirModules, name)); err != nil {
		t.Fatal(err)
	}
}

// product loads a whole product the way a run does: the oak.yaml in a
// folder, and every module beside it.
func product(t *testing.T, dir string) (*spec.Runtime, []*spec.Module) {
	t.Helper()
	rt, err := spec.LoadRuntime(dir)
	if err != nil {
		t.Fatal(err)
	}
	mods, err := rt.LoadModules()
	if err != nil {
		t.Fatal(err)
	}
	return rt, mods
}

func loaded(t *testing.T, dir string) *spec.Module {
	t.Helper()
	mod, err := spec.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return mod
}

func TestTheMachineLocaleIsReadInPosixOrder(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  map[string]string
		want string
	}{
		{"LC_ALL beats the rest", map[string]string{"LC_ALL": "de_DE.UTF-8", "LC_MESSAGES": "fr_FR", "LANG": "it_IT"}, "de_DE.UTF-8"},
		{"LC_MESSAGES beats LANG", map[string]string{"LC_MESSAGES": "fr_FR", "LANG": "it_IT"}, "fr_FR"},
		{"LANG is the last word", map[string]string{"LANG": "it_IT"}, "it_IT"},
		{"an empty value is no answer", map[string]string{"LC_ALL": "", "LANG": "it_IT"}, "it_IT"},
		{"nothing set is no answer", nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
				t.Setenv(key, "")
				os.Unsetenv(key)
			}
			for key, value := range tc.env {
				t.Setenv(key, value)
			}
			if got := locale(); got != tc.want {
				t.Errorf("locale() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestASavedLanguageBeatsTheMachineLocale(t *testing.T) {
	t.Setenv("LC_ALL", "fr_FR.UTF-8")
	langs := []i18n.Lang{{Code: "en"}, {Code: "de"}, {Code: "fr"}}
	if got := language("de", langs); got != "de" {
		t.Errorf("language() = %q, want de — a stored choice is somebody having said so", got)
	}
}

func TestAMachineLocaleSettlesTheLanguageWhenNothingWasSaved(t *testing.T) {
	t.Setenv("LC_ALL", "de_AT.UTF-8")
	langs := []i18n.Lang{{Code: "en"}, {Code: "de"}}
	if got := language("", langs); got != "de" {
		t.Errorf("language() = %q, want de — de_AT is German", got)
	}
}

func TestALanguageNoLongerOnOfferFallsBackToTheMachineLocale(t *testing.T) {
	t.Setenv("LC_ALL", "de_DE.UTF-8")
	langs := []i18n.Lang{{Code: "en"}, {Code: "de"}}
	if got := language("fr", langs); got != "de" {
		t.Errorf("language() = %q, want de — a saved code no catalog answers to is not an answer", got)
	}
}

func TestAnUntranslatableMachineLeavesTheSourceLanguage(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	langs := []i18n.Lang{{Code: "en"}, {Code: "de"}}
	if got := language("", langs); got != i18n.SourceLang {
		t.Errorf("language() = %q, want %q", got, i18n.SourceLang)
	}
}

// Every module is read at startup, so one that will not load is a message
// before anything is offered rather than a row that fails when it is chosen.
func TestEveryModuleOfARuntimeIsRead(t *testing.T) {
	dir := runtime(t, runtimeDecl, "installer", "recovery")

	rt, err := spec.LoadRuntime(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := rt.LoadModules()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].UI.Title != "installer" || got[1].UI.Title != "recovery" {
		t.Errorf("load() = %d modules, want both of them, in name order", len(got))
	}
}

// Naming one is the question of which to open, already answered — so the run
// narrows to it, whatever else was read alongside it.
func TestNamingAModuleNarrowsTheRunToIt(t *testing.T) {
	rt, mods := product(t, runtime(t, runtimeDecl, "installer", "recovery"))

	got, err := narrow(rt, mods, "recovery")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].UI.Title != "recovery" {
		t.Fatalf("narrow() = %v, want only the one that was named", got)
	}
	all, err := narrow(rt, mods, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Error("a run that named no module was narrowed anyway")
	}
}

// Whether a name is a module is settled by what is beside the binary at the
// moment it is given, which is what keeps the list of them out of this program.
func TestAModuleNobodyDeclaredIsRefusedByName(t *testing.T) {
	rt, mods := product(t, runtime(t, runtimeDecl, "installer"))

	_, err := narrow(rt, mods, "manager")
	if err == nil {
		t.Fatal("a module nobody declared was opened")
	}
	if !strings.Contains(err.Error(), "installer") {
		t.Errorf("error = %q, want it to say what is on offer", err)
	}
}

func TestARuntimeHoldingABrokenModuleWillNotStart(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, spec.FileRuntime), []byte(runtimeDecl), 0o600); err != nil {
		t.Fatal(err)
	}
	put(t, dir, "installer", writeModule(t, "title: T\nstages: [go]\n", nil))
	put(t, dir, "recovery", writeModule(t, "title: T\n", nil))
	rt, err := spec.LoadRuntime(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.LoadModules(); err == nil {
		t.Fatal("a runtime with a module that declares no stages was read as sound")
	}
}

func TestAModuleWithNoCatalogsSpeaksTheRuntimesOwn(t *testing.T) {
	mod := loaded(t, writeModule(t, "title: T\nstages: [go]\n", nil))
	if got := len(catalogs(mod)); got != 1 {
		t.Errorf("catalogs() has %d sources, want 1 — the runtime's alone", got)
	}
}

func TestAModulesCatalogsAreLaidOverTheRuntimes(t *testing.T) {
	mod := loaded(t, writeModule(t, "title: T\nstages: [go]\n", map[string]string{
		"locales/de.po": "msgid \"Back\"\nmsgstr \"Zurück\"\n",
	}))
	sources := catalogs(mod)
	if len(sources) != 2 {
		t.Fatalf("catalogs() has %d sources, want 2", len(sources))
	}
	codes := codes(i18n.Discover(sources...))
	if strings.Join(codes, " ") != "en de" {
		t.Errorf("languages = %v, want [en de]", codes)
	}
}

func TestAModuleThatWillNotLoadSaysWhatIsWrongWithIt(t *testing.T) {
	dir := around(t, writeModule(t, "stages: [go]\n", nil)) // no title
	rt, err := spec.LoadRuntime(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.LoadModules(); err == nil {
		t.Fatal("a module with no title loaded; it must not")
	}
}

// The whole of what a command line says: five options, each spelled out, and
// nothing else on the line at all.
func TestACommandLineIsFiveOptionsAndNothingElse(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want command
	}{
		{name: "nothing at all"},
		{name: "a module named outright", args: []string{"--module=installer"}, want: command{module: "installer"}},
		{name: "in front of the module", args: []string{"--debug", "--module=installer"}, want: command{module: "installer", debug: true}},
		{name: "behind it", args: []string{"--module=installer", "--debug"}, want: command{module: "installer", debug: true}},
		{name: "the version on its own", args: []string{"--version"}, want: command{version: true}},
		{name: "two of them at once", args: []string{"--debug", "--version"}, want: command{debug: true, version: true}},
		{name: "a report on the whole product", args: []string{"--inspect"}, want: command{inspect: true}},
		{name: "a report on one module", args: []string{"--inspect", "--module=installer"}, want: command{module: "installer", inspect: true}},
		{name: "one module's template", args: []string{"--strings", "--module=installer"}, want: command{module: "installer", strings: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parse(tc.args)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("parse(%v) = %+v, want %+v", tc.args, got, tc.want)
			}
		})
	}
}

func TestACommandLineThatCannotBeReadIsRefused(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"two modules", []string{"--module=installer", "--module=recovery"}, "One module at a time"},
		{"a module with no name", []string{"--module"}, "needs the name of a module"},
		{"a module with an empty name", []string{"--module="}, "needs the name of a module"},
		{"a word that is not an option", []string{"installer"}, "is not something this program takes"},
		{"an option nobody has", []string{"--report"}, "is not something this program takes"},
		{"a value where none is taken", []string{"--debug=true"}, "is not something this program takes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parse(tc.args)
			if err == nil {
				t.Fatal("the line was read as sound")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

// --version is the binary answering for itself, so it says which binary: a
// number on its own beside a product's own number says nothing about whose it
// is. It is written the way a release names its own files, name and version as
// one word.
func TestTheVersionNamesTheProgramItBelongsTo(t *testing.T) {
	out := stdout(t, func() {
		if err := start([]string{"--version"}); err != nil {
			t.Fatal(err)
		}
	})
	if want := program + "-" + version + "\n"; out != want {
		t.Errorf("--version printed %q, want %q", out, want)
	}
}

// stdout is whatever a call wrote to it.
func stdout(t *testing.T, call func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	was := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = was }()

	call()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// The one wire between the command line and a script: --debug reaches every one
// of them as DEBUG, and a run without it hands over nothing at all.
func TestDebugOnTheCommandLineReachesEveryScript(t *testing.T) {
	for _, debug := range []bool{false, true} {
		mod := loaded(t, writeModule(t, "title: T\nstages: [go]\n", nil))
		t.Chdir(t.TempDir())

		p, err := open(mod, debug)
		if err != nil {
			t.Fatal(err)
		}
		env := strings.Join(p.Store.Env(), "\n")
		if got := strings.Contains(env, spec.DebugVar+"="+spec.BoolTrue); got != debug {
			t.Errorf("open(debug=%v) hands a script DEBUG=true: %v", debug, got)
		}
	}
}

// offeringRuntime is a product whose modules disagree about which machine they
// belong on: one always, one never, and one that says nothing at all.
func offeringRuntime(t *testing.T) (*spec.Runtime, []*spec.Module) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, spec.FileRuntime), []byte(runtimeDecl), 0o600); err != nil {
		t.Fatal(err)
	}
	put(t, dir, "here", writeModule(t, "title: here\nstages: [go]\noffered: \"true\"\n", nil))
	put(t, dir, "elsewhere", writeModule(t, "title: elsewhere\nstages: [go]\n"+
		"offered: |\n  echo \"not this machine\" >&2\n  exit 1\n", nil))
	put(t, dir, "anywhere", writeModule(t, "title: anywhere\nstages: [go]\n", nil))
	return product(t, dir)
}

// A module says for itself which machines it belongs on, and one that says no
// is not on the list somebody is asked to choose from. One that says nothing
// belongs everywhere, which is what keeps the key optional.
func TestOnlyTheModulesThisMachineBelongsToAreOffered(t *testing.T) {
	_, mods := offeringRuntime(t)

	got, err := offered(mods, false)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, mod := range got {
		names = append(names, mod.UI.Title)
	}
	want := "anywhere here"
	if strings.Join(names, " ") != want {
		t.Errorf("offered() = %v, want %s", names, want)
	}
}

// A simulated run is read on whatever machine somebody happens to be at, and
// narrowing it to what that machine is would hide the pages they opened it for.
func TestASimulatedRunIsOfferedEveryModule(t *testing.T) {
	_, mods := offeringRuntime(t)

	got, err := offered(mods, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(mods) {
		t.Errorf("offered(debug) = %d modules, want all %d of them", len(got), len(mods))
	}
}

// Named outright, a module that does not belong here is refused in its own
// words: the sentence it wrote is the whole of what is worth saying, and an
// exit status in front of it only gets in the way.
func TestAModuleNamedOutrightIsRefusedInItsOwnWords(t *testing.T) {
	rt, mods := offeringRuntime(t)

	one, err := narrow(rt, mods, "elsewhere")
	if err != nil {
		t.Fatal(err)
	}
	_, err = offered(one, false)
	if err == nil {
		t.Fatal("a module that does not belong on this machine was opened anyway")
	}
	if err.Error() != "not this machine" {
		t.Errorf("error = %q, want what the module said", err)
	}
}

// Nothing to open is not an empty list to look at: every module said why, and
// all of it is read at once.
func TestAMachineNoModuleBelongsOnIsToldByEveryOneOfThem(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, spec.FileRuntime), []byte(runtimeDecl), 0o600); err != nil {
		t.Fatal(err)
	}
	put(t, dir, "one", writeModule(t, "title: one\nstages: [go]\n"+
		"offered: |\n  echo \"needs a live image\" >&2\n  exit 1\n", nil))
	put(t, dir, "two", writeModule(t, "title: two\nstages: [go]\n"+
		"offered: |\n  echo \"needs a plugged-in device\" >&2\n  exit 1\n", nil))
	_, mods := product(t, dir)

	_, err := offered(mods, false)
	if err == nil {
		t.Fatal("a machine no module belongs on was let through")
	}
	for _, want := range []string{"needs a live image", "needs a plugged-in device"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to hold %q", err, want)
		}
	}
}

// The shell it is decided by is the module's own, so a check reads as a
// sentence rather than as a line of test flags — and the one place that names
// the rule is the module it belongs to.
func TestTheOfferedCheckIsGivenTheModulesOwnShell(t *testing.T) {
	dir := around(t, writeModule(t, "title: shelled\nstages: [go]\noffered: belongs_here\n",
		map[string]string{spec.FileShell: "belongs_here() { return 0; }\n"}))
	_, mods := product(t, dir)

	got, err := offered(mods, false)
	if err != nil {
		t.Fatalf("a module whose check its own shell answers was refused: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("offered() = %d modules, want the one", len(got))
	}
}

// The check is shell a module wrote, and it is written next to its tasks and
// its hooks — where `return 0` is how a guard says yes. Shell that means one
// thing there and another here would be a trap laid for whoever writes the next
// module.
func TestTheOfferedCheckMaySayYesTheWayEveryOtherGuardDoes(t *testing.T) {
	dir := around(t, writeModule(t, "title: returning\nstages: [go]\n"+
		"offered: |\n  [ -n \"$HOME\" ] && return 0\n  echo no home >&2\n  exit 1\n", nil))
	_, mods := product(t, dir)

	if _, err := offered(mods, false); err != nil {
		t.Fatalf("a check that answered with `return 0` was read as a refusal: %v", err)
	}
}
