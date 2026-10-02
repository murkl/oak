package spec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// module writes a minimal but complete module and returns its path. Each
// test starts from a working one and breaks exactly the thing it is about, so a
// failure names the rule that was broken rather than a missing file three rules
// earlier.
//
// A file whose body is empty is left out, which is how a test removes one.
func module(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	base := map[string]string{
		FileModule:               head("variables:\n  - name: DISK\n    title: Disk\n    required: true\n"),
		"tasks/@go/do/task.yaml": "title: Do it\n",
		"tasks/@go/do/task.sh":   "echo hi\n",
	}
	for name, body := range files {
		base[name] = body
	}
	for name, body := range base {
		if body == "" {
			continue
		}
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

// head is a declaration with the two keys every module needs, and whatever
// the test is actually about after them.
func head(body string) string { return "title: Test Installer\nstages: [go]\n" + body }

// unit is one task folder, as the two files it is made of. The stage it belongs
// to is the folder above it, the way a module lays one out.
func unit(stage, id, yaml string) map[string]string {
	at := DirTasks + "/" + Stage(stage) + "/" + id
	return map[string]string{
		at + "/task.yaml": yaml,
		at + "/task.sh":   "echo " + id + "\n",
	}
}

// action is one action folder, as the two files it is made of.
func action(id, yaml string) map[string]string {
	at := DirActions + "/" + id
	return map[string]string{
		at + "/" + FileAction:       yaml,
		at + "/" + FileActionScript: "echo " + id + "\n",
	}
}

// units merges several unit() results into the map a module is written from.
func units(all ...map[string]string) map[string]string {
	out := map[string]string{}
	for _, m := range all {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

func TestLoadReadsAWholeTree(t *testing.T) {
	dir := module(t, map[string]string{
		FileModule: `
title: Test Installer
stages: [go, done]
variables:
  - name: DISK
    title: Disk
    required: true
presets:
  - title: Full
    values:
      DISK: /dev/sda
`,
		"tasks/@done/reboot/task.yaml": "title: Reboot\nconfirm: Restart now?\n",
		"tasks/@done/reboot/task.sh":   "echo bye\n",
	})
	sp, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if sp.UI.Title != "Test Installer" {
		t.Errorf("title = %q", sp.UI.Title)
	}
	if len(sp.Presets) != 1 || sp.Presets[0].Values["DISK"] != "/dev/sda" {
		t.Errorf("presets = %+v", sp.Presets)
	}
	if len(sp.Tasks) != 2 {
		t.Fatalf("tasks = %d, want 2", len(sp.Tasks))
	}
	if !strings.HasSuffix(string(sp.Tasks[0].Work()), filepath.Join("do", FileTaskScript)) {
		t.Errorf("script = %q", sp.Tasks[0].Work())
	}
	last := sp.Tasks[1]
	if !last.Confirms() {
		t.Errorf("reboot = %+v, want it to ask", last)
	}
	if got := last.Question(func(string) string { return "" }); got != "Restart now?" {
		t.Errorf("question = %q", got)
	}
}

// The order is worked out from what each task says about itself, and it is
// the one thing about a module nobody writes down. Getting it wrong means
// installing onto a disk that has not been partitioned yet.
func TestOrderFollowsStagesThenNeeds(t *testing.T) {
	dir := module(t, units(
		map[string]string{
			FileModule: "title: T\nstages: [first, second]\n",
			// The default task's stage is gone from the list, so its folder goes
			// with it and this test owns the whole run.
			"tasks/@go/do/task.yaml":    "",
			"tasks/@go/do/task.sh":      "",
			"tasks/@first/do/task.yaml": "title: Do\n",
			"tasks/@first/do/task.sh":   "echo do\n",
		},
		unit("first", "zulu", "title: Zulu\n"),
		unit("first", "alpha", "title: Alpha\nneeds: [zulu]\n"),
		unit("second", "later", "title: Later\n"),
	))
	sp, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, e := range sp.Tasks {
		ids = append(ids, e.ID())
	}
	// do and zulu are independent and keep folder order; alpha needs zulu, so
	// it waits for it even though its name would have put it first; later is in
	// the second stage and comes after all of them.
	want := "do|zulu|alpha|later"
	if strings.Join(ids, "|") != want {
		t.Errorf("order = %v, want %v", ids, strings.Split(want, "|"))
	}
}

// `needs` orders tasks across one stage and the stages order the rest, so a
// need reaching into another stage says nothing that has not been said. It is
// dropped with a word about it rather than refused: a module that behaves is
// not a module that refuses to start.
func TestANeedReachingIntoAnotherStageIsAWarning(t *testing.T) {
	dir := module(t, units(
		map[string]string{FileModule: "title: T\nstages: [go, later]\n"},
		unit("later", "after", "title: After\nneeds: [do]\n"),
	))
	sp, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(sp.Warnings) != 1 || !strings.Contains(sp.Warnings[0], "needs do, which is in go") {
		t.Fatalf("warnings = %v, want one naming the stage the need is in", sp.Warnings)
	}
	// And it is dropped, so nothing downstream tries to wait for it.
	for _, task := range sp.Tasks {
		if task.ID() == "after" && len(task.Needs) != 0 {
			t.Errorf("needs = %v, want none left", task.Needs)
		}
	}
}

func TestOrderRefusesWhatCannotBeWalked(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{
			name:  "a task naming a stage nothing declared",
			files: unit("nowhere", "do", "title: Do\n"),
			want:  "no such stage",
		},
		{
			name:  "a task lying straight under tasks/, in no stage at all",
			files: map[string]string{"tasks/half/task.yaml": "title: Half\n", "tasks/half/task.sh": "echo\n"},
			want:  "a task lies in the folder of its stage",
		},
		{
			name:  "a hooks folder from an older Oak",
			files: map[string]string{"hooks/@preflight/root/hook.yaml": "title: Root\nscript: \"true\"\n"},
			want:  "the runtime runs no hooks — each is an action now",
		},
		{
			name:  "an action named the way an older Oak read it",
			files: map[string]string{FileModule: head("requires: [o]\n")},
			want:  "requires is not a key here — it is a rule now: under rules:, as start-if",
		},
		{
			name:  "a shell of the module's own, the way an older Oak read one",
			files: map[string]string{"module.sh": "helper() { :; }\n"},
			want:  "module.sh: a module has no shell of its own — what its scripts share",
		},
		{
			name:  "a need pointing at nothing",
			files: unit("go", "do", "title: Do\nneeds: [ghost]\n"),
			want:  "needs unknown task",
		},
		{
			// The ring itself, because a list of what is left would leave
			// whoever reads it to work it out by hand.
			name: "tasks waiting for each other",
			files: units(
				unit("go", "do", "title: Do\nneeds: [other]\n"),
				unit("go", "other", "title: Other\nneeds: [do]\n"),
			),
			want: "wait on each other: do → other → do",
		},

		{
			name:  "a task folder with no yaml in it",
			files: map[string]string{"tasks/@go/half/task.sh": "echo\n"},
			want:  FileTask,
		},
		{
			name:  "a task that says nothing about what it does",
			files: map[string]string{"tasks/@go/half/task.yaml": "title: Half\n"},
			want:  "no " + FileTaskScript,
		},
		{
			name: "a task writing its shell into the yaml",
			files: map[string]string{
				"tasks/@go/half/task.yaml": "title: Half\nscript: echo hi\n",
				"tasks/@go/half/task.sh":   "echo hi\n",
			},
			want: "a task does its work in the task.sh beside it",
		},
		{
			name: "a task writing its test into the yaml",
			files: map[string]string{
				"tasks/@go/half/task.yaml": "title: Half\ntest: test -e /\n",
				"tasks/@go/half/task.sh":   "echo hi\n",
			},
			want: "a task is tested by the test.sh beside it",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(module(t, tc.files))
			if err == nil {
				t.Fatalf("loaded a module with %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

// What a machine has to be for this module to be offered on it is an action,
// named under the module's rules.
func TestAModuleNamesWhatItIsOfferedOn(t *testing.T) {
	sp, err := Load(module(t, units(
		map[string]string{FileModule: head("rules:\n  offer-if: [live]\n")},
		action("live", "title: A live image\nerror: This runs from a live image.\n"),
	)))
	if err != nil {
		t.Fatal(err)
	}
	if got := sp.Named(sp.Rules.OfferIf); len(got) != 1 || got[0].ID() != "live" {
		t.Errorf("offered = %v, want the action the module named", got)
	}

	// A module that demands nothing is on offer everywhere.
	sp, err = Load(module(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(sp.Rules.OfferIf) != 0 {
		t.Errorf("offer-if = %v, want nothing", sp.Rules.OfferIf)
	}
}

// Nothing lists the actions: a folder under actions/ is one. Where each runs is
// the rule that names it, in the order it names them.
func TestActionsAreNamedWhereTheyRun(t *testing.T) {
	sp, err := Load(module(t, units(
		map[string]string{FileModule: head("rules:\n  start-if: [root, internet]\n  on-settings: [wlan]\n  on-leave: [restart]\n")},
		action("root", "title: Running as root\nerror: Log in as root.\n"),
		action("internet", "title: Internet\nerror: There is no internet.\nrules:\n  on-failure: wlan\n"),
		action("wlan", "title: Wireless network\nrules:\n  offer-if: [card]\n"),
		action("card", "title: A wireless card\n"),
		action("restart", "title: Restart\n"),
	)))
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, a := range sp.Named(sp.Rules.StartIf) {
		ids = append(ids, a.ID())
	}
	if strings.Join(ids, " ") != "root internet" {
		t.Errorf("start-if = %v, want them in the order the module named them", ids)
	}
	if a := sp.Action("internet"); a.OnFailure != "wlan" {
		t.Errorf("internet on failure opens %q, want wlan", a.OnFailure)
	}
	if a := sp.Action("wlan"); len(a.OfferIf) != 1 || a.OfferIf[0] != "card" {
		t.Errorf("wlan is offered if %v, want the card", a.OfferIf)
	}
	if !strings.HasSuffix(string(sp.Action("restart").Work()), filepath.Join("restart", FileActionScript)) {
		t.Errorf("restart runs %q, want the %s beside its yaml", sp.Action("restart").Work(), FileActionScript)
	}
	if !sp.Leaves() {
		t.Error("Leaves() = false, want true: an action stands on the way out")
	}
}

func TestAModuleWithoutActionsHasNone(t *testing.T) {
	sp, err := Load(module(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(sp.Actions) != 0 {
		t.Errorf("actions = %v, want none", sp.Actions)
	}
	if sp.Leaves() {
		t.Error("Leaves() = true, want false: nothing says how to leave")
	}
}

// An action's page is a question like the module's own, held to the same rules
// and sharing its names — but it is not one of the module's questions: never
// asked on the way in and never on the settings page. The answer its report
// shows is declared by being named there.
func TestAnActionsPageIsDeclaredBesideTheModulesOwn(t *testing.T) {
	sp, err := Load(module(t, units(
		map[string]string{FileModule: head("rules:\n  on-settings: [wlan, share]\nvariables:\n  - name: DISK\n    title: Disk\n")},
		action("wlan", `
title: Wireless network
variable:
  name: WLAN_SSID
  title: Network
  options-from: ./networks.sh
`),
		map[string]string{"actions/wlan/networks.sh": "echo Home\n"},
		action("share", "title: Share\nreport: Shared\nshows: LINK\n"),
	)))
	if err != nil {
		t.Fatal(err)
	}
	if len(sp.Vars) != 1 || sp.Vars[0].Name != "DISK" {
		t.Errorf("Vars = %v, want only the module's own question", sp.Vars)
	}
	if got := len(sp.Declared()); got != 3 {
		t.Errorf("Declared() = %d variables, want the question, the page and the code", got)
	}
	if sp.Var("LINK") == nil {
		t.Error("LINK is not a variable, want it declared by the report that shows it")
	}
	ssid := sp.Var("WLAN_SSID")
	if ssid == nil || !strings.Contains(ssid.OptionsFrom, filepath.Join("wlan", "networks.sh")) {
		t.Errorf("WLAN_SSID = %+v, want its list read from beside the action's yaml", ssid)
	}
}

// What an action says has to be something that can take effect where it is
// run, and every name has to be an action. Each of these loads into something
// that never runs, or asks where nobody is asked.
func TestAnActionRefusesWhatCannotTakeEffect(t *testing.T) {
	row := func(yaml string) map[string]string {
		return units(map[string]string{FileModule: head("rules:\n  on-settings: [o]\n")}, action("o", yaml))
	}
	required := func(yaml string) map[string]string {
		return units(map[string]string{FileModule: head("rules:\n  start-if: [o]\n")}, action("o", "error: No.\n"+yaml))
	}
	cases := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"no title", row("description: O\n"), "title is required"},
		{"nothing that runs", map[string]string{FileModule: head("rules:\n  on-settings: [o]\n"), "actions/o/action.yaml": "title: O\n"}, "no " + FileActionScript},
		{"a name that is no action", map[string]string{FileModule: head("rules:\n  start-if: [ghost]\n")}, "rules: start-if: no such action: ghost"},
		{"a failure opening no action", row("title: O\nrules:\n  on-failure: ghost\n"), "rules: on-failure: no such action: ghost"},
		{"an action nothing names", action("o", "title: O\n"), "nothing names it, so it never runs"},
		{"a required action that asks", required("title: O\nvariable:\n  name: X\n  title: X\n"), "it runs by itself where it is named, so it has no page"},
		{"a required action that reports", required("title: O\nreport: Done\n"), "it runs by itself where it is named, so it has no page"},
		{"a required action that does not say why", units(map[string]string{FileModule: head("rules:\n  start-if: [o]\n")}, action("o", "title: O\n")), "error: it runs by itself in front of the work"},
		{"a way out that asks", units(map[string]string{FileModule: head("rules:\n  on-leave: [o]\n")}, action("o", "title: O\nvariable:\n  name: X\n  title: X\n")), "a way out asks nothing"},
		{"a failure opening itself", row("title: O\nrules:\n  on-failure: o\n"), "cannot put itself right"},
		{"actions that wait on each other", units(
			map[string]string{FileModule: head("rules:\n  on-settings: [a]\n")},
			action("a", "title: A\nrules:\n  offer-if: [b]\n"),
			action("b", "title: B\nrules:\n  on-failure: a\n"),
		), "actions that wait on each other: a → b → a"},
		{"a page asked first", row("title: O\nvariable:\n  name: X\n  title: X\n  first: true\n"), "a page is asked when its action is opened"},
		{"a page in a group", row("title: O\nvariable:\n  name: X\n  title: X\n  group: G\n"), "a page is never on the settings page"},
		{"a page worked out", row("title: O\nvariable:\n  name: X\n  title: X\n  value-from: x()\n"), "a value worked out is not"},
		{"a page asked mid-run", row("title: O\nvariable:\n  name: X\n  title: X\n  type: deferred\n  options: [a]\n"), "type: deferred is asked by a task mid-run"},
		{"a page named like a question", units(
			map[string]string{FileModule: head("rules:\n  on-settings: [o]\nvariables:\n  - name: X\n    title: X\n")},
			action("o", "title: O\nvariable:\n  name: X\n  title: X\n"),
		), "X is declared twice"},
		{"a code named like a question", units(
			map[string]string{FileModule: head("rules:\n  on-settings: [o]\nvariables:\n  - name: X\n    title: X\n")},
			action("o", "title: O\nreport: Done\nshows: X\n"),
		), "X is declared twice"},
		{"a page guarded by nothing", row("title: O\nvariable:\n  name: X\n  title: X\n  conditions: NOPE == y\n"), "no such variable: NOPE"},
		{"two pages", row("title: O\nreport: Done\ntty: true\n"), "an action has one page"},
		{"a question and a report", row("title: O\nreport: Done\nvariable:\n  name: X\n  title: X\n"), "an action has one page"},
		{"several questions", row("title: O\nvariables:\n  - name: X\n    title: X\n"), "a second question is a second action named under its rules: on-failure"},
		{"a code with no report to stand on", row("title: O\nshows: X\n"), "there is no report for it to appear on"},
		{"a script written into the yaml", row("title: O\nscript: echo hi\n"), "an action in the action.sh beside it"},
		{"a yes or no before it runs", row("title: O\nconfirm: Sure?\n"), "an action is agreed to by choosing its row"},
		{"a fail naming no answer", row("title: O\nerror: Nothing on {{NOPE}}.\n"), "{{NOPE}} is not a variable of this module"},
		{"an options folder from an older Oak", map[string]string{"options/wlan/option.yaml": "title: W\n"}, "an option is an action now"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(module(t, tc.files))
			if err == nil {
				t.Fatalf("loaded a module with %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

// A task's own proof that the work took is the test.sh beside its task.sh, and
// a task without one is simply not checked.
func TestATaskIsCheckedByTheTestBesideIt(t *testing.T) {
	sp, err := Load(module(t, units(
		map[string]string{
			"tasks/@go/beside/task.yaml": "title: Beside\n",
			"tasks/@go/beside/task.sh":   "echo hi\n",
			"tasks/@go/beside/test.sh":   "test -e /\n",
		},
	)))
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]*Task{}
	for _, task := range sp.Tasks {
		by[task.ID()] = task
	}
	if got := by["beside"].Check(); !strings.HasSuffix(string(got), filepath.Join("beside", FileTest)) {
		t.Errorf("beside check = %q, want the %s beside it", got, FileTest)
	}
	if by["do"].Checks() {
		t.Errorf("do = %q, want no check", by["do"].Check())
	}
	if !sp.Checks() {
		t.Error("Checks() = false, want true: a task says how to tell")
	}
}

// locales/ is found beside the declaration, so a module turns its catalogs on
// by having them and off by not.
func TestLocalesAreFoundBesideTheDeclaration(t *testing.T) {
	sp, err := Load(module(t, map[string]string{
		DirLocales + "/de.po": "msgid \"English\"\nmsgstr \"Deutsch\"\n",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(sp.Locales, DirLocales) {
		t.Errorf("locales = %q", sp.Locales)
	}

	if sp, err = Load(module(t, nil)); err != nil {
		t.Fatal(err)
	}
	if sp.Locales != "" {
		t.Errorf("locales = %q, want none", sp.Locales)
	}
}

// The word for starting the work is the module's, so it is translated like
// everything else it says, and a block scalar is read as the one line a catalog
// looks it up by.
func TestTheWordForStartingIsTranslatable(t *testing.T) {
	sp, err := Load(module(t, map[string]string{
		FileModule: head("start-title: |\n  Install\n"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Test Installer", "Install", "Do it"}
	if got := texts(sp); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("Messages() = %v\nwant %v", got, want)
	}
	if got := sp.Start(); got != "Install" {
		t.Errorf("Start() = %q, want %q", got, "Install")
	}
}

// So is the word for its settings, the same way.
func TestTheWordForTheSettingsIsTranslatable(t *testing.T) {
	sp, err := Load(module(t, map[string]string{
		FileModule: head("settings-title: |\n  Configuration\n"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Test Installer", "Configuration", "Do it"}
	if got := texts(sp); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("Messages() = %v\nwant %v", got, want)
	}
	if got := sp.Settings(); got != "Configuration" {
		t.Errorf("Settings() = %q, want %q", got, "Configuration")
	}
}

// Every one of these is an authoring mistake that must be caught while the module
// is being opened. The alternative — loading anyway — is a task that
// silently never runs on somebody's machine, which is the failure this whole
// check exists to prevent.
func TestLoadRefuses(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{
			name:  "a condition naming a variable nobody declared",
			files: unit("go", "do", "title: Do\nconditions: NOPE == true\n"),
			want:  "no such variable",
		},
		{
			name:  "a condition that is not three words",
			files: unit("go", "do", "title: Do\nconditions: DISK\n"),
			want:  "bad condition",
		},
		{
			name:  "a task with no title",
			files: unit("go", "do", "needs: []\n"),
			want:  "title is required",
		},
		{
			name:  "an offer opening on no, the way an older Oak read it",
			files: unit("go", "do", "title: Do\nconfirm: Really?\ndefault: no\n"),
			want:  "default is not a key here — a task's confirm opens on yes",
		},
		{
			name:  "a preset filling in a variable nobody declared",
			files: map[string]string{FileModule: head("presets:\n  - title: P\n    values:\n      NOPE: x\n")},
			want:  "no such variable",
		},
		{
			name:  "a preset with no title",
			files: map[string]string{FileModule: head("presets:\n  - description: nothing\n")},
			want:  "presets: 1: title is required",
		},
		{
			name:  "presets grouped on a page of their own, the way an older Oak read them",
			files: map[string]string{FileModule: head("presets:\n  - title: P\n    options:\n      - title: O\n")},
			want:  "options is not a key here — a starting point stands under presets: itself",
		},
		{
			name:  "two variables of the same name",
			files: map[string]string{FileModule: head("variables:\n  - name: DISK\n    title: A\n  - name: DISK\n    title: B\n")},
			want:  "declared twice",
		},
		{
			name:  "a variable with no title",
			files: map[string]string{FileModule: head("variables:\n  - name: DISK\n")},
			want:  "title is required",
		},
		{
			name:  "a type nobody has heard of",
			files: map[string]string{FileModule: head("variables:\n  - name: DISK\n    title: D\n    type: colour\n")},
			want:  "unknown type",
		},
		{
			name:  "a bool with values of its own",
			files: map[string]string{FileModule: head("variables:\n  - name: DISK\n    title: D\n    type: bool\n    options: [a, b]\n")},
			want:  "has no options of its own",
		},
		{
			name:  "a filter setting nobody has heard of",
			files: map[string]string{FileModule: head("variables:\n  - name: DISK\n    title: D\n    options: [a, b]\n    filter: hidden\n")},
			want:  "unknown filter",
		},
		{
			name:  "a filter on a question with no list to narrow",
			files: map[string]string{FileModule: head("variables:\n  - name: DISK\n    title: D\n    filter: open\n")},
			want:  "box to type in",
		},
		{
			name:  "a filter on a question asked first, which carries its box either way",
			files: map[string]string{FileModule: head("variables:\n  - name: DISK\n    title: D\n    options: [a, b]\n    first: true\n    filter: open\n")},
			want:  "says nothing here",
		},
		{
			name:  "a secret asked first, which is a question that would never be asked",
			files: map[string]string{FileModule: head("variables:\n  - name: PW\n    title: P\n    type: secret\n    first: true\n")},
			want:  "cannot also be asked first",
		},
		{
			name:  "a secret with a default, which would be a stored password",
			files: map[string]string{FileModule: head("variables:\n  - name: PW\n    title: P\n    type: secret\n    default: hunter2\n")},
			want:  "cannot have a default",
		},
		{
			name:  "a secret worked out rather than typed",
			files: map[string]string{FileModule: head("variables:\n  - name: PW\n    title: P\n    type: secret\n    value-from: x()\n")},
			want:  "never worked out",
		},
		{
			name:  "existing on a question that is not a password at all",
			files: map[string]string{FileModule: head("variables:\n  - name: DISK\n    title: D\n    existing: true\n")},
			want:  "only a secret is either",
		},
		{
			name:  "a check on an answer the settings page shows and its pattern holds",
			files: map[string]string{FileModule: head("variables:\n  - name: DISK\n    title: D\n    check: x()\n")},
			want:  "any other answer is held to its pattern",
		},
		{
			name:  "the keys the runtime now says for itself",
			files: map[string]string{FileModule: head("console: Type installer.\nconfirm: Careful.\n")},
			want:  "console is not a key here — the row that leaves to the console is the runtime's own",
		},
		{
			name:  "the word for starting said the way an older Oak read it",
			files: map[string]string{FileModule: head("action: Install\n")},
			want:  "the word for starting the work is start",
		},
		{
			name: "a task's offer naming a variable nothing declares",
			files: units(map[string]string{FileModule: head("variables:\n  - name: DISK\n    title: D\n")},
				unit("go", "a", "title: A\nconfirm: Erase {{DSIK}}?\n")),
			want: "{{DSIK}} is not a variable of this module",
		},
		{
			name: "a task's report naming a variable nothing declares",
			files: units(map[string]string{FileModule: head("variables:\n  - name: DISK\n    title: D\n")},
				unit("go", "a", "title: A\nreport: It went to {{DSIK}}.\n")),
			want: "{{DSIK}} is not a variable of this module",
		},
		{
			name:  "a question both worked out and suggested, which is asked and not asked at once",
			files: map[string]string{FileModule: head("variables:\n  - name: X\n    title: X\n    value-from: a()\n    prefill: b()\n")},
			want:  "a question is asked or it is not",
		},
		{
			name:  "a derived answer asked first, which is a question that is never asked",
			files: map[string]string{FileModule: head("variables:\n  - name: X\n    title: X\n    value-from: a()\n    first: true\n")},
			want:  "cannot be asked first",
		},
		{
			name:  "both a list and a function for the same question",
			files: map[string]string{FileModule: head("variables:\n  - name: DISK\n    title: D\n    options: [a]\n    options-from: ls()\n")},
			want:  "two answers to the same question",
		},
		{
			name:  "a pattern that is not a pattern",
			files: map[string]string{FileModule: head("variables:\n  - name: DISK\n    title: D\n    pattern: '['\n")},
			want:  "pattern",
		},
		{
			name:  "a key that is a typo, silently ignored by a lesser reader",
			files: map[string]string{FileModule: head("variables:\n  - name: DISK\n    title: D\n    requird: true\n")},
			want:  "requird is not a key here",
		},
		{
			name:  "a key that is a typo in a task",
			files: unit("go", "do", "title: Do\nquites: true\n"),
			want:  "quites is not a key here",
		},
		{
			// A file written for an older Oak fails on the key itself and is
			// told what to write instead, so the refusal is the whole of what
			// somebody needs in order to fix it.
			name:  "a key a product used to be able to declare",
			files: map[string]string{FileModule: head("blind: true\n")},
			want:  "blind is not a key here — a question asked first opens its filter by itself",
		},
		{
			// The two keys this layout retired, each pointing at where what
			// they said now lives.
			name:  "a task still naming its stage",
			files: unit("go", "do", "title: Do\nstage: go\n"),
			want:  "stage is not a key here — a task lies in the folder of its stage",
		},
		{
			name:  "a task still saying execute",
			files: unit("go", "do", "title: Do\nexecute: echo hi\n"),
			want:  "execute is not a key here — a task does its work in the task.sh beside it",
		},
		{
			name:  "a language tied to a variable nobody declared",
			files: map[string]string{FileModule: head("language: NOPE\nvariables:\n  - name: DISK\n    title: D\n")},
			want:  "no such variable",
		},
		{
			// The answer file is handed to every script under this name, so a
			// module that declared it would be overwriting the one thing it
			// answers questions back through.
			name:  "the answer file's own name, redeclared",
			files: map[string]string{FileModule: head("variables:\n  - name: " + ConfVar + "\n    title: C\n")},
			want:  "belongs to the runtime",
		},
		{
			// It is handed to every script by the runtime, so a module that
			// declared it would be asking a question nothing reads the answer of.
			name:  "the switch that simulates a run, redeclared",
			files: map[string]string{FileModule: head("variables:\n  - name: " + DebugVar + "\n    title: D\n")},
			want:  "belongs to the runtime",
		},
		{
			name:  "no stages at all",
			files: map[string]string{FileModule: "title: T\nstages: []\n"},
			want:  "no stages",
		},
		{
			name:  "a stage listed twice",
			files: map[string]string{FileModule: "title: T\nstages: [go, go]\n"},
			want:  "listed twice",
		},
		{
			name:  "asks naming a variable nobody declared",
			files: unit("go", "do", "title: Do\nasks: NOPE\n"),
			want:  "no such variable",
		},
		{
			name:  "asks on a question asked on the way in, which says nothing of being deferred",
			files: unit("go", "do", "title: Do\nasks: DISK\n"),
			want:  "asks: DISK is asked on the way in — a question asked mid-run says type: deferred",
		},
		{
			name: "a deferred question that is free text, which is not one the frame can put mid-run",
			files: units(
				map[string]string{FileModule: head("variables:\n  - name: PICK\n    title: P\n    type: deferred\n")},
				unit("go", "do", "title: Do\nasks: PICK\n"),
			),
			want: "a question asked mid-run is a list",
		},
		{
			name:  "a deferred question no task asks, which would be asked nowhere",
			files: map[string]string{FileModule: head("variables:\n  - name: PICK\n    title: P\n    type: deferred\n    options: [a, b]\n")},
			want:  "PICK: type deferred is asked by a task under asks:, and no task asks it",
		},
		{
			name: "a deferred question asked first",
			files: units(
				map[string]string{FileModule: head("variables:\n  - name: PICK\n    title: P\n    type: deferred\n    first: true\n    options: [a, b]\n")},
				unit("go", "do", "title: Do\nasks: PICK\n"),
			),
			want: "first: a deferred question is asked by its task, mid-run",
		},
		{
			name: "a deferred question in a group, which no settings page ever shows",
			files: units(
				map[string]string{FileModule: head("variables:\n  - name: PICK\n    title: P\n    type: deferred\n    group: G\n    options: [a, b]\n")},
				unit("go", "do", "title: Do\nasks: PICK\n"),
			),
			want: "group: a deferred question is never on the settings page",
		},
		{
			name: "a deferred question worked out",
			files: units(
				map[string]string{FileModule: head("variables:\n  - name: PICK\n    title: P\n    type: deferred\n    options: [a, b]\n    value-from: a()\n")},
				unit("go", "do", "title: Do\nasks: PICK\n"),
			),
			want: "value-from: a deferred question is asked",
		},
		{
			name:  "a question's list the way an older Oak named it",
			files: map[string]string{FileModule: head("variables:\n  - name: X\n    title: X\n    values: [a]\n")},
			want:  "values is not a key here — a question's list is options",
		},
		{
			name:  "what prints a list the way an older Oak named it",
			files: map[string]string{FileModule: head("variables:\n  - name: X\n    title: X\n    command: x()\n")},
			want:  "command is not a key here — what prints a question's list is options-from",
		},
		{
			name:  "a value worked out the way an older Oak named it",
			files: map[string]string{FileModule: head("variables:\n  - name: X\n    title: X\n    answer: x()\n")},
			want:  "answer is not a key here — a value worked out instead of asked is value-from",
		},
		{
			name:  "a task the run goes on past, the way an older Oak named it",
			files: unit("go", "do", "title: Do\noptional: true\n"),
			want:  "optional is not a key here — a task the run goes on past when it fails says allow-failure",
		},
		{
			name:  "an action's no the way an older Oak named it",
			files: units(map[string]string{FileModule: head("rules:\n  start-if: [o]\n")}, action("o", "title: O\nfail: No.\n")),
			want:  "fail is not a key here — what a no from an action means is its error",
		},
		{
			name:  "a row on the settings page the way an older Oak named it",
			files: map[string]string{FileModule: head("rules:\n  settings: [o]\n")},
			want:  "settings is not a key here — it is a row on the settings page: under rules:, as on-settings",
		},
		{
			name:  "the header's status read the way an older Oak named it",
			files: map[string]string{FileModule: head("status:\n  script: x()\n")},
			want:  "script is not a key here — a task does its work in the task.sh beside it, an action in the action.sh beside it, and the header's status reads check",
		},
		{
			name:  "a menu of rows, the way an older Oak placed them",
			files: map[string]string{FileModule: head("rules:\n  menu: [o]\n")},
			want:  "menu is not a key here — its rows stand on the settings page: under rules:, as on-settings",
		},
		{
			name:  "a task showing a code, the way an older Oak drew one",
			files: unit("go", "do", "title: Do\nreport: Done\nshows: DISK\n"),
			want:  "shows is not a key here — a code is drawn by an action",
		},
		{
			name:  "a starting point fetched the way an older Oak fetched one",
			files: map[string]string{FileModule: head("presets:\n  - title: P\n    asks: DISK\n")},
			want:  "asks is not a key here — a starting point that is fetched names the action that fetches it",
		},
		{
			name:  "a starting point opening an action that is not there",
			files: map[string]string{FileModule: head("presets:\n  - title: P\n    action: ghost\n")},
			want:  "no such action: ghost",
		},
		{
			name: "a starting point both written out and fetched",
			files: units(
				map[string]string{FileModule: head("variables:\n  - name: DISK\n    title: Disk\npresets:\n  - title: P\n    action: fetch\n    values:\n      DISK: /dev/sda\n")},
				action("fetch", "title: Fetch\n"),
			),
			want: "written out in values or fetched by an action, not both",
		},
		{
			name:  "the network said the way an older Oak read it",
			files: map[string]string{FileModule: head("network:\n  wlan: true\n")},
			want:  "network is not a key here — a wireless network is an action under actions/",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(module(t, tc.files))
			if err == nil {
				t.Fatalf("loaded a module with %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestConditionsDecideWhatBelongs(t *testing.T) {
	dir := module(t, units(
		map[string]string{
			FileModule:               head("variables:\n  - name: DESKTOP\n    title: Desktop\n    type: bool\n"),
			"tasks/@go/do/task.yaml": "title: Always\n",
		},
		unit("go", "with", "title: Only with a desktop\nconditions: DESKTOP == true\n"),
		unit("go", "without", "title: Only without one\nconditions: DESKTOP != true\n"),
	))
	sp, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ desktop, want string }{
		{"true", "Only with a desktop"},
		{"false", "Only without one"},
		{"", "Only without one"},
	} {
		// Only the one variable answers, the way a store does: everything else,
		// the mode this run is in included, is empty.
		get := func(name string) string {
			if name == "DESKTOP" {
				return tc.desktop
			}
			return ""
		}
		var names []string
		for _, e := range sp.Tasks {
			if e.Applies(get) {
				names = append(names, e.Title)
			}
		}
		want := []string{"Always", tc.want}
		if strings.Join(names, "|") != strings.Join(want, "|") {
			t.Errorf("DESKTOP=%q ran %v, want %v", tc.desktop, names, want)
		}
	}
}

// A shell field names a function of oak.sh or a file beside the yaml, and the
// two are told apart by how they are written: name() is a call, ./ a path.
func TestShellFieldsNameAFunctionOrAFile(t *testing.T) {
	dir := module(t, map[string]string{
		FileModule: head(`
variables:
  - name: DISK
    title: Disk
    options-from: ./data/disks.sh
    prefill: first_disk()
`),
		"data/disks.sh": "lsblk\n",
	})
	sp, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	v := sp.Var("DISK")
	if !strings.HasPrefix(v.OptionsFrom, "source ") || !strings.Contains(v.OptionsFrom, "disks.sh") {
		t.Errorf("options-from = %q, want it to source the file", v.OptionsFrom)
	}
	if v.Prefill != "first_disk" {
		t.Errorf("prefill = %q, want the function called by name", v.Prefill)
	}
}

// Shell written into the yaml itself is read by no linter and has no line to
// point at, so every field that names shell refuses it.
func TestShellWrittenIntoTheYamlIsRefused(t *testing.T) {
	for _, field := range []string{"options-from", "prefill", "apply", "value-from"} {
		for _, expr := range []string{"echo a", "first_disk", "first_disk ()", "./disks", "./missing.sh", "'./two words.sh'"} {
			t.Run(field+" "+expr, func(t *testing.T) {
				dir := module(t, map[string]string{
					FileModule: head("variables:\n  - name: X\n    title: X\n    " + field + ": " + expr + "\n"),
					"disks":    "lsblk\n",
				})
				_, err := Load(dir)
				if err == nil || !strings.Contains(err.Error(), "X: "+field+": ") {
					t.Errorf("err = %v, want %s refused", err, expr)
				}
			})
		}
	}
}

func TestReflowKeepsOnlyTheBreaksThatWereMeant(t *testing.T) {
	dir := module(t, map[string]string{
		FileModule: head("variables:\n  - name: DISK\n    title: Disk\n    description: |\n      One sentence\n      wrapped by an editor.\n\n      A second paragraph.\n"),
	})
	sp, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := "One sentence wrapped by an editor.\n\nA second paragraph."
	if got := sp.Var("DISK").Description; got != want {
		t.Errorf("description = %q, want %q", got, want)
	}
}

func TestExpandFillsInAnswers(t *testing.T) {
	get := func(name string) string {
		return map[string]string{"DISK": "/dev/sda"}[name]
	}
	for _, tc := range []struct{ in, want string }{
		{"Erasing {{DISK}}.", "Erasing /dev/sda."},
		{"Erasing {{ DISK }}.", "Erasing /dev/sda."},
		{"Nothing to fill in.", "Nothing to fill in."},
		{"{{DISK}} and {{DISK}}", "/dev/sda and /dev/sda"},
		// A name nothing answers is left empty rather than left as its own
		// braces, which would put the machinery on screen.
		{"On {{UNKNOWN}}.", "On ."},
		{"An {{unclosed", "An {{unclosed"},
	} {
		if got := Expand(tc.in, get); got != tc.want {
			t.Errorf("Expand(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// Every answer is a string in the end, but nobody writes `default: "true"`.
func TestScalarReadsWhateverShapeItWasWrittenIn(t *testing.T) {
	dir := module(t, map[string]string{
		FileModule: head("variables:\n  - name: A\n    title: A\n    default: true\n  - name: B\n    title: B\n    default: 8\n  - name: C\n    title: C\n    default: pc105\n"),
	})
	sp, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"A": "true", "B": "8", "C": "pc105"} {
		if got := sp.Var(name).Default.String(); got != want {
			t.Errorf("%s default = %q, want %q", name, got, want)
		}
	}
}

func TestStringsIsEveryWordTheTreeSays(t *testing.T) {
	dir := module(t, map[string]string{
		FileModule: head(`
presets:
  - title: Full
    description: Everything.
variables:
  - name: DISK
    title: Disk
    description: Where it goes.
    group: Storage
    error: Pick one.
`),
		"tasks/@go/do/task.yaml": "title: Do it\nconfirm: Really?\n",
	})
	sp, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Test Installer", "Full", "Everything.", "Disk", "Where it goes.", "Storage", "Pick one.", "Do it", "Really?"}
	got := texts(sp)
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("Messages() = %v\nwant %v", got, want)
	}

	// A translator gets the words out of the module they belong to, so every one
	// of them says where it was read and what it is.
	for _, m := range sp.Messages() {
		if len(m.Files) == 0 || m.Note == "" {
			t.Errorf("%q has no origin: %+v", m.Text, m)
		}
	}
	if m := sp.Messages()[len(sp.Messages())-1]; m.Files[0] != "tasks/@go/do/task.yaml" {
		t.Errorf("%q was read from %v, want the task it is in", m.Text, m.Files)
	}
}

// texts is what the module says, without where it says it.
func texts(sp *Module) []string {
	var out []string
	for _, m := range sp.Messages() {
		out = append(out, m.Text)
	}
	return out
}

// The declaration has one name, so a folder either holds a module or does not.
func TestAModuleWithoutADeclarationIsRefused(t *testing.T) {
	_, err := Load(module(t, map[string]string{FileModule: ""}))
	if err == nil || !strings.Contains(err.Error(), FileModule) {
		t.Errorf("err = %v, want it to name %s", err, FileModule)
	}
}

// One guard is a line and several are a list, and every one of them has to
// hold: a row that belongs under two unrelated circumstances is two rows.
func TestSeveralConditionsAllHaveToHold(t *testing.T) {
	dir := module(t, map[string]string{
		FileModule:               head("variables:\n  - name: DESKTOP\n    title: D\n    type: bool\n  - name: DRIVER\n    title: G\n"),
		"tasks/@go/do/task.yaml": "title: Driver\nconditions:\n  - DESKTOP == true\n  - DRIVER != none\n",
	})
	sp, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		desktop, driver string
		want            bool
	}{
		{"true", "mesa", true},
		{"true", "none", false},
		{"false", "mesa", false},
		{"false", "none", false},
	} {
		get := func(name string) string {
			return map[string]string{"DESKTOP": tc.desktop, "DRIVER": tc.driver}[name]
		}
		if got := sp.Tasks[0].Applies(get); got != tc.want {
			t.Errorf("DESKTOP=%s DRIVER=%s applies = %v, want %v", tc.desktop, tc.driver, got, tc.want)
		}
	}
}

func TestConditionsRefuseAnythingButAConditionOrAListOfThem(t *testing.T) {
	_, err := Load(module(t, unit("go", "do", "title: Do\nconditions:\n  DISK: yes\n")))
	if err == nil || !strings.Contains(err.Error(), "conditions takes a condition") {
		t.Errorf("err = %v", err)
	}
}

// A value a task asks for mid-run is one the opening run of questions has no
// business asking: it does not exist yet. The variable says so itself, so the
// declaration read from the top tells it from the questions asked on the way
// in.
func TestADeferredQuestionIsLeftForItsTask(t *testing.T) {
	dir := module(t, units(
		map[string]string{
			FileModule: head(`variables:
  - name: DISK
    title: Disk
    required: true
  - name: SNAPSHOT
    title: Snapshot
    type: deferred
    options: [a, b]
`),
		},
		unit("go", "do", "title: Do\nasks: SNAPSHOT\n"),
	))
	sp, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !sp.Var("SNAPSHOT").Deferred() {
		t.Error("SNAPSHOT is not deferred")
	}
	if sp.Var("DISK").Deferred() {
		t.Error("an ordinary question was deferred")
	}
}

// A starting point that is fetched rather than written out opens an action,
// whose page asks for what it fetches.
func TestAStartingPointMayBeFetchedByAnAction(t *testing.T) {
	sp, err := Load(module(t, units(
		map[string]string{FileModule: head("presets:\n  - title: Online\n    action: fetch\n")},
		action("fetch", "title: Fetch\nvariable:\n  name: SOURCE\n  title: Code\n"),
	)))
	if err != nil {
		t.Fatal(err)
	}
	o := sp.Presets[0]
	if !o.Fetches() || sp.Action(o.Action) == nil {
		t.Errorf("preset = %+v, want it to open the action it names", o)
	}
}

// The first paragraph of a report is its headline, the way the first block of
// the opening logo is its eyebrow — one idiom, and nothing extra to declare.
func TestAReportsFirstParagraphIsItsHeadline(t *testing.T) {
	dir := module(t, unit("go", "do", "title: Do\nreport: |\n  Installed on {{DISK}}\n\n  And here is what that means.\n"))
	sp, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	answers := func(string) string { return "/dev/sda" }
	headline, body := sp.Tasks[0].ReportText(answers)
	if headline != "Installed on /dev/sda" {
		t.Errorf("headline = %q", headline)
	}
	if body != "And here is what that means." {
		t.Errorf("body = %q", body)
	}
	// A report of one paragraph is a headline and nothing else.
	if !sp.Tasks[0].Reports() {
		t.Error("a task with a report says it has none")
	}
}

// A task may say its output is its progress, and the load keeps that on that
// task alone.
func TestATaskMayDeclareItsOutputItsProgress(t *testing.T) {
	sp, err := Load(module(t, unit("go", "fetch", "title: Fetch\nprogress: true\n")))
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range sp.Tasks {
		if task.Progress != (task.ID() == "fetch") {
			t.Errorf("%s: Progress = %v, want it only where it was declared", task.ID(), task.Progress)
		}
	}
}

// A task may say the result stands without it, and the load keeps that on that
// task alone.
func TestATaskMayAllowItsOwnFailure(t *testing.T) {
	sp, err := Load(module(t, unit("go", "theme", "title: Theme\nallow-failure: true\n")))
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range sp.Tasks {
		if task.AllowFailure != (task.ID() == "theme") {
			t.Errorf("%s: AllowFailure = %v, want it only where it was declared", task.ID(), task.AllowFailure)
		}
	}
}
