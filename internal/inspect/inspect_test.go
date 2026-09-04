package inspect

import (
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/murkl/oak/internal/i18n"
	"github.com/murkl/oak/internal/spec"
	"github.com/murkl/oak/locales"
)

// productDecl is a product with nothing to say about itself but its name: what
// it offers is the folders beside it.
const productDecl = "name: Test OS\n"

// writeModule writes the smallest module that will load, plus whatever extra
// files a test needs, and answers with the folder it put them in.
func writeModule(t *testing.T, declaration string, extra map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"installer.yaml":        declaration,
		"tasks/first/task.yaml": "name: First\nstage: go\n",
		"tasks/first/task.sh":   "true\n",
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

// around puts an oak.yaml over one module folder, so a module written on its
// own can be read the way a run reads one.
func around(t *testing.T, mod string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, spec.FileRuntime), []byte(productDecl), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, spec.DirModules), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(mod, filepath.Join(dir, spec.DirModules, "installer")); err != nil {
		t.Fatal(err)
	}
	return dir
}

// product loads a whole product the way a run does: the oak.yaml in a folder,
// and every module beside it.
func product(t *testing.T, dir string) (*spec.Runtime, []*spec.Module) {
	t.Helper()
	rt, err := spec.LoadRuntime(dir)
	if err != nil {
		t.Fatal(err)
	}
	mods := make([]*spec.Module, 0, len(rt.Modules))
	for _, name := range rt.Modules {
		mod, err := spec.Load(rt.Path(name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		mods = append(mods, mod)
	}
	return rt, mods
}

func TestTheTemplateOfAModuleHoldsEveryWordItSays(t *testing.T) {
	dir := around(t, writeModule(t, `
title: Installer
stages: [go]
variables:
  - name: HOST
    title: Host name
    description: |
      The name this machine has.

      It is the one the network knows it by.
`, nil))
	rt, mods := product(t, dir)
	var out strings.Builder
	if err := Template(&out, rt, mods); err != nil {
		t.Fatal(err)
	}
	if _, err := i18n.Parse([]byte(out.String())); err != nil {
		t.Fatalf("the template is not readable po: %v\n%s", err, out.String())
	}
	for _, want := range []string{
		`msgid "Installer"`,
		`msgid "Host name"`,
		// Paragraphs are written as lines, so a translator reads them as they
		// will be read.
		"msgid \"\"\n\"The name this machine has.\\n\"\n\"\\n\"\n\"It is the one the network knows it by.\"",
		// And every one of them says where it was read.
		"#: installer.yaml",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the template has no %q in it:\n%s", want, out.String())
		}
	}
}

func TestInspectingAModuleNamesEverythingItHolds(t *testing.T) {
	dir := around(t, writeModule(t, `
title: Installer
stages: [go]
variables:
  - name: HOST
    title: Host name
    required: true
  - name: PASSWORD
    title: Password
    type: secret
    required: true
`, map[string]string{
		"hooks/preflight.sh": "true\n",
		// The one task reads both answers, so the report is about what the
		// module holds rather than about a guard that disagrees — see
		// spec.Unread.
		"tasks/first/task.sh": "echo \"$HOST $PASSWORD\"\n",
	}))
	rt, mods := product(t, dir)
	var out strings.Builder
	if err := Report(&out, rt, mods, locales.FS); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"modules    installer",
		"title      Installer",
		"variables  2 (2 required, 1 secret)",
		"tasks      1",
		"hooks      preflight",
		"1. go         first",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the report does not say %q:\n%s", want, out.String())
		}
	}
}

// The version on screen is the product's own, so the report names the one a run
// would show rather than the binary's.
func TestTheReportNamesTheVersionTheProductDeclares(t *testing.T) {
	dir := around(t, writeModule(t, "title: T\nstages: [go]\n", nil))
	if err := os.WriteFile(filepath.Join(dir, spec.FileRuntime), []byte("name: Test OS\nversion: 1.4.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rt, mods := product(t, dir)
	var out strings.Builder
	if err := Report(&out, rt, mods, locales.FS); err != nil {
		t.Fatal(err)
	}
	if want := "version    1.4.0"; !strings.Contains(out.String(), want) {
		t.Errorf("the report does not say %q:\n%s", want, out.String())
	}
}

// A module that behaves is not a module that refuses to start, so this is the
// one thing this report has to say that loading it never will.
func TestInspectingRefusesAQuestionNothingReadsTheAnswerOf(t *testing.T) {
	mod := writeModule(t, `
title: Installer
stages: [go]
variables:
  - name: SPARE
    title: Spare
`, nil)
	if _, err := spec.Load(mod); err != nil {
		t.Fatalf("the module does not load, and it has to: %v", err)
	}
	rt, mods := product(t, around(t, mod))
	var out strings.Builder
	if err := Report(&out, rt, mods, locales.FS); err == nil {
		t.Fatal("a question nothing reads was reported as fine")
	} else if !strings.Contains(err.Error(), "nothing reads the answer") {
		t.Errorf("Report() = %v, want it to say nothing reads the answer", err)
	}
}

func TestOnlyTheHooksAModuleActuallyHasAreListed(t *testing.T) {
	mod, err := spec.Load(writeModule(t, "title: T\nstages: [go]\n", map[string]string{
		"hooks/preflight.sh": "true\n",
		"hooks/restart.sh":   "true\n",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(hooks(mod), " "); got != "preflight restart" {
		t.Errorf("hooks() = %q, want \"preflight restart\" in HookNames order", got)
	}
}

// One template belongs to one module, so a product holding several says which
// to name rather than picking one.
func TestATemplateOfSeveralModulesAtOnceIsRefused(t *testing.T) {
	dir := around(t, writeModule(t, "title: T\nstages: [go]\n", nil))
	second := writeModule(t, "title: R\nstages: [go]\n", nil)
	if err := os.Rename(second, filepath.Join(dir, spec.DirModules, "recovery")); err != nil {
		t.Fatal(err)
	}
	rt, mods := product(t, dir)
	var out strings.Builder
	if err := Template(&out, rt, mods); err == nil {
		t.Fatal("a template was written for two modules at once")
	}
}
