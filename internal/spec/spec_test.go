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
		FileModule:           head("variables:\n  - name: DISK\n    title: Disk\n    required: true\n"),
		"tasks/do/task.yaml": "stage: go\ntitle: Do it\n",
		"tasks/do/task.sh":   "echo hi\n",
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
// to is a line in its yaml, the way a module writes one.
func unit(stage, id, yaml string) map[string]string {
	at := "tasks/" + id
	return map[string]string{
		at + "/task.yaml": "stage: " + stage + "\n" + yaml,
		at + "/task.sh":   "echo " + id + "\n",
	}
}

// hook is one step of a hook, as the two files it is made of. Hooks live in a
// folder of their own, and every file in one says so: hook.yaml and hook.sh.
func hook(name, id, yaml string) map[string]string {
	at := DirHooks + "/" + name + "/" + id
	return map[string]string{
		at + "/" + FileHook:       yaml,
		at + "/" + FileHookScript: "echo " + id + "\n",
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
confirm: Erasing {{DISK}}.
stages: [go, done]
variables:
  - name: DISK
    title: Disk
    required: true
presets:
  - title: Start
    options:
      - title: Full
        values:
          DISK: /dev/sda
`,
		"tasks/reboot/task.yaml": "stage: done\ntitle: Reboot\nconfirm: Restart now?\nquits: true\n",
		"tasks/reboot/task.sh":   "echo bye\n",
	})
	sp, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if sp.UI.Title != "Test Installer" {
		t.Errorf("title = %q", sp.UI.Title)
	}
	if got := sp.ConfirmText(func(string) string { return "/dev/sda" }); got != "Erasing /dev/sda." {
		t.Errorf("confirm = %q", got)
	}
	if len(sp.Presets) != 1 || sp.Presets[0].Options[0].Values["DISK"] != "/dev/sda" {
		t.Errorf("presets = %+v", sp.Presets)
	}
	if len(sp.Tasks) != 2 {
		t.Fatalf("tasks = %d, want 2", len(sp.Tasks))
	}
	if !strings.HasSuffix(sp.Tasks[0].Work().File, filepath.Join("do", FileTaskScript)) {
		t.Errorf("script = %q", sp.Tasks[0].Work().File)
	}
	last := sp.Tasks[1]
	if !last.Quits || !last.Confirms() {
		t.Errorf("reboot = %+v, want it to ask and to quit", last)
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
			// The default task moves into the first stage: this test owns the
			// whole list.
			"tasks/do/task.yaml": "stage: first\ntitle: Do\n",
			"tasks/do/task.sh":   "echo do\n",
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
			name:  "a task naming no stage at all",
			files: map[string]string{"tasks/half/task.yaml": "title: Half\n", "tasks/half/task.sh": "echo\n"},
			want:  "stage is required",
		},
		{
			name:  "a hook folder that is not one of the runtime's",
			files: hook(HookMark+"nowhere", "do", "title: Do\n"),
			want:  "no such hook",
		},
		{
			name:  "a hook left under tasks/",
			files: map[string]string{"tasks/@preflight/root/hook.yaml": "title: Root\nexecute: \"true\"\n"},
			want:  "which lives under " + DirHooks,
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
			files: map[string]string{"tasks/half/task.sh": "echo\n"},
			want:  FileTask,
		},
		{
			name:  "a task that says nothing about what it does",
			files: map[string]string{"tasks/half/task.yaml": "stage: go\ntitle: Half\n"},
			want:  "no " + FileTaskScript,
		},
		{
			name: "a task saying twice what it does",
			files: map[string]string{
				"tasks/half/task.yaml": "stage: go\ntitle: Half\nexecute: echo hi\n",
				"tasks/half/task.sh":   "echo hi\n",
			},
			want: "a task runs one thing",
		},
		{
			name:  "a script naming a file that is not there",
			files: map[string]string{"tasks/half/task.yaml": "stage: go\ntitle: Half\nexecute: ./gone.sh\n"},
			want:  "no such script",
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

// Nothing declares the hooks: a folder under one of their names is the
// declaration, and any other name carrying the mark is a typo rather than
// something to ignore.
func TestHooksAreFoundByTheirFolder(t *testing.T) {
	sp, err := Load(module(t, units(
		hook(HookPreflight, "root", "title: Running as root\n"),
		hook(HookRestart, "reboot", "title: Reboot\n"),
	)))
	if err != nil {
		t.Fatal(err)
	}
	if got := sp.Hook(HookPreflight); len(got) != 1 || got[0].ID() != "root" {
		t.Errorf("preflight = %+v, want the one step in it", got)
	}
	if got := sp.Hook(HookRestart); len(got) != 1 || got[0].Work().File == "" {
		t.Errorf("restart = %+v, want the step and the file it runs", got)
	}
	if got := sp.Hook(HookShutdown); len(got) != 0 {
		t.Errorf("shutdown = %+v, want none", got)
	}
	// A hook runs at its own moment, so nothing in it is part of the work: the
	// run is the one task the module has of its own.
	if len(sp.Tasks) != 1 || sp.Tasks[0].ID() != "do" {
		t.Errorf("tasks = %+v, want only the module's own work", sp.Tasks)
	}
	if !sp.Leaves() {
		t.Error("Leaves() = false, want true: there is a restart hook")
	}
}

// A step of a hook knows which hook it is in, so a failure in one can say so.
func TestAHookStepNamesItsHook(t *testing.T) {
	sp, err := Load(module(t, hook(HookPreflight, "root", "title: Root\n")))
	if err != nil {
		t.Fatal(err)
	}
	step := sp.Hook(HookPreflight)[0]
	if step.Hook() != HookPreflight {
		t.Errorf("Hook() = %q, want %q", step.Hook(), HookPreflight)
	}
	if sp.Tasks[0].Hook() != "" {
		t.Errorf("a task's Hook() = %q, want none", sp.Tasks[0].Hook())
	}
}

// Every step of a hook runs in order, so a module may split a check into the
// several things it actually checks.
func TestAHookRunsEveryStepInIt(t *testing.T) {
	sp, err := Load(module(t, units(
		hook(HookPreflight, "root", "title: Root\n"),
		hook(HookPreflight, "network", "title: Network\nneeds: [root]\n"),
	)))
	if err != nil {
		t.Fatal(err)
	}
	steps := sp.Hook(HookPreflight)
	if len(steps) != 2 || steps[0].ID() != "root" || steps[1].ID() != "network" {
		t.Fatalf("preflight = %+v, want root then network", steps)
	}
}

// Most of what a task may say has nothing to answer to in a hook: it is never
// listed, offered, reported on or checked afterwards. Saying it anyway is a
// line that can never take effect.
func TestAHookStepRefusesWhatItCannotMean(t *testing.T) {
	for key, line := range map[string]string{
		"stage":      "stage: go\n",
		"test":       "test: \"true\"\n",
		"conditions": "conditions: DISK != none\n",
		"confirm":    "confirm: Really?\n",
		"report":     "report: Done\n",
		"quits":      "quits: true\n",
		"tty":        "tty: true\n",
	} {
		t.Run(key, func(t *testing.T) {
			_, err := Load(module(t, hook(HookPreflight, "check", "title: Check\n"+line)))
			if err == nil || !strings.Contains(err.Error(), key) {
				t.Errorf("err = %v, want it to name %s", err, key)
			}
		})
	}
}

func TestAModuleWithoutHooksHasNone(t *testing.T) {
	sp, err := Load(module(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range Hooks {
		if got := sp.Hook(name); len(got) != 0 {
			t.Errorf("%s = %+v, want none", name, got)
		}
	}
	if sp.Leaves() {
		t.Error("Leaves() = true, want false: nothing says how to leave")
	}
}

// A task's own proof that the work took is a second script beside the first,
// found the same way: written in the yaml, named by it, or simply lying there
// under the name Oak knows it by.
func TestATaskFindsItsCheckTheWayItFindsItsWork(t *testing.T) {
	sp, err := Load(module(t, units(
		map[string]string{
			"tasks/inline/task.yaml": "stage: go\ntitle: Inline\nexecute: echo hi\ntest: test -e /\n",
			"tasks/beside/task.yaml": "stage: go\ntitle: Beside\n",
			"tasks/beside/task.sh":   "echo hi\n",
			"tasks/beside/test.sh":   "test -e /\n",
		},
	)))
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]*Task{}
	for _, task := range sp.Tasks {
		by[task.ID()] = task
	}
	if got := by["inline"].Check(); got.Shell != "test -e /" {
		t.Errorf("inline check = %+v, want the shell it wrote", got)
	}
	if got := by["beside"].Check(); !strings.HasSuffix(got.File, FileTest) {
		t.Errorf("beside check = %+v, want the %s beside it", got, FileTest)
	}
	// The default task declares none, and a module is checked only where
	// something says how.
	if by["do"].Checks() {
		t.Errorf("do = %+v, want no check", by["do"].Check())
	}
	if !sp.Checks() {
		t.Error("Checks() = false, want true: two tasks say how to tell")
	}
}

// Saying it twice is two answers to one question, exactly as it is for the work
// itself.
func TestATaskCannotSayTwiceHowItIsChecked(t *testing.T) {
	_, err := Load(module(t, map[string]string{
		"tasks/half/task.yaml": "stage: go\ntitle: Half\ntest: test -e /\n",
		"tasks/half/task.sh":   "echo hi\n",
		"tasks/half/test.sh":   "test -e /\n",
	}))
	if err == nil || !strings.Contains(err.Error(), "a task runs one thing") {
		t.Errorf("err = %v, want it to refuse two checks", err)
	}
}

// A task says what it does in its own yaml or in the file beside it, and the
// two are told apart because a failure in a file names the file.
func TestATaskRunsItsFileOrTheShellItsYamlWrote(t *testing.T) {
	sp, err := Load(module(t, units(
		map[string]string{
			"tasks/inline/task.yaml": "stage: go\ntitle: Inline\nexecute: echo hi\n",
			"tasks/named/task.yaml":  "stage: go\ntitle: Named\nexecute: ./other.sh\n",
			"tasks/named/other.sh":   "echo other\n",
		},
	)))
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]*Task{}
	for _, task := range sp.Tasks {
		by[task.ID()] = task
	}
	if got := by["inline"].Work(); got.File != "" || got.Shell != "echo hi" {
		t.Errorf("inline = %q / %q, want the shell it wrote", got.File, got.Shell)
	}
	if got := by["named"].Work(); !strings.HasSuffix(got.File, "other.sh") || got.Shell != "" {
		t.Errorf("named = %q / %q, want the file it named", got.File, got.Shell)
	}
	if got := by["do"].Work(); !strings.HasSuffix(got.File, FileTaskScript) {
		t.Errorf("do = %q, want the %s beside it", got.File, FileTaskScript)
	}
}

// module.sh and locales/ are found the same way, so a module turns them on by
// having them and off by not.
func TestTheModuleShellAndLocalesAreFoundBesideTheDeclaration(t *testing.T) {
	sp, err := Load(module(t, map[string]string{
		FileShell:             "helper() { echo hi; }\n",
		DirLocales + "/de.po": "msgid \"English\"\nmsgstr \"Deutsch\"\n",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(sp.Shell, FileShell) {
		t.Errorf("shell = %q", sp.Shell)
	}
	if !strings.HasSuffix(sp.Locales, DirLocales) {
		t.Errorf("locales = %q", sp.Locales)
	}

	if sp, err = Load(module(t, nil)); err != nil {
		t.Fatal(err)
	}
	if sp.Shell != "" || sp.Locales != "" {
		t.Errorf("shell = %q, locales = %q, want neither", sp.Shell, sp.Locales)
	}
}

// The sentence read on the way out is the module's, so it is translated like
// everything else it says.
func TestConsoleIsTranslatable(t *testing.T) {
	sp, err := Load(module(t, map[string]string{
		FileModule: head("console: Type installer to start again.\n"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Test Installer", "Type installer to start again.", "Do it"}
	if got := texts(sp); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("Messages() = %v\nwant %v", got, want)
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
			name:  "an offer opening on an answer it does not have",
			files: unit("go", "do", "title: Do\nconfirm: Really?\ndefault: maybe\n"),
			want:  "yes or no",
		},
		{
			name:  "an answer to an offer that was never made",
			files: unit("go", "do", "title: Do\ndefault: no\n"),
			want:  "no confirm for it to answer",
		},
		{
			name:  "a preset filling in a variable nobody declared",
			files: map[string]string{FileModule: head("presets:\n  - title: P\n    options:\n      - title: O\n        values:\n          NOPE: x\n")},
			want:  "no such variable",
		},
		{
			name:  "a preset page with nothing to choose on it",
			files: map[string]string{FileModule: head("presets:\n  - title: P\n")},
			want:  "no options",
		},
		{
			name:  "a preset option with no title",
			files: map[string]string{FileModule: head("presets:\n  - title: P\n    options:\n      - description: nothing\n")},
			want:  "title is required",
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
			files: map[string]string{FileModule: head("variables:\n  - name: DISK\n    title: D\n    type: bool\n    values: [a, b]\n")},
			want:  "has no values of its own",
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
			name:  "both a list and a command for the same question",
			files: map[string]string{FileModule: head("variables:\n  - name: DISK\n    title: D\n    values: [a]\n    command: ls\n")},
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
			files: map[string]string{FileModule: head("run: Installation\n")},
			want:  "run is not a key here — a module is named once, by its title",
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
			name:  "asks on a free text value, which is not a question the frame can put mid-run",
			files: unit("go", "do", "title: Do\nasks: DISK\n"),
			want:  "no answers to choose from",
		},
		{
			name: "asks on a secret, which is already asked at the only safe moment",
			files: units(
				map[string]string{FileModule: head("variables:\n  - name: PW\n    title: P\n    type: secret\n")},
				unit("go", "do", "title: Do\nasks: PW\n"),
			),
			want: "is a secret",
		},
		{
			name:  "a value shown on a page that does not exist",
			files: unit("go", "do", "title: Do\nshows: DISK\n"),
			want:  "no report for it to appear on",
		},
		{
			name:  "a report showing a variable nobody declared",
			files: unit("go", "do", "title: Do\nreport: Done\nshows: NOPE\n"),
			want:  "no such variable",
		},
		{
			name: "a report showing a secret",
			files: units(
				map[string]string{FileModule: head("variables:\n  - name: PW\n    title: Password\n    type: secret\n")},
				unit("go", "do", "title: Do\nreport: Done\nshows: PW\n"),
			),
			want: "is a secret",
		},
		{
			name:  "a starting point asking for a variable nobody declared",
			files: map[string]string{FileModule: head("presets:\n  - title: P\n    options:\n      - title: O\n        asks: NOPE\n")},
			want:  "no such variable",
		},
		{
			name:  "a starting point with shell and nothing to run it on",
			files: map[string]string{FileModule: head("presets:\n  - title: P\n    options:\n      - title: O\n        apply: echo hi\n")},
			want:  "no asks for it to work from",
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
			FileModule:           head("variables:\n  - name: DESKTOP\n    title: Desktop\n    type: bool\n"),
			"tasks/do/task.yaml": "stage: go\ntitle: Always\n",
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

// A shell field takes either the shell itself or the file it lives in, and the
// whole rule is the ./ in front. Getting this wrong either way is silent: a
// path run as a command, or a command looked for as a file.
func TestShellFieldsTellCodeFromFiles(t *testing.T) {
	dir := module(t, map[string]string{
		FileModule: head(`
variables:
  - name: DISK
    title: Disk
    command: ./data/disks.sh
    prefill: lsblk -dno PATH | head -n1
`),
		"data/disks.sh": "lsblk\n",
	})
	sp, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	v := sp.Var("DISK")
	if !strings.HasPrefix(v.Command, "source ") || !strings.Contains(v.Command, "disks.sh") {
		t.Errorf("command = %q, want it to source the file", v.Command)
	}
	if v.Prefill != "lsblk -dno PATH | head -n1" {
		t.Errorf("prefill = %q, want it left as written", v.Prefill)
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
confirm: Careful.
presets:
  - title: Setup
    description: What kind.
    options:
      - title: Full
        description: Everything.
variables:
  - name: DISK
    title: Disk
    description: Where it goes.
    group: Storage
    error: Pick one.
`),
		"tasks/do/task.yaml": "stage: go\ntitle: Do it\nconfirm: Really?\n",
	})
	sp, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Test Installer", "Careful.", "Setup", "What kind.", "Full", "Everything.", "Disk", "Where it goes.", "Storage", "Pick one.", "Do it", "Really?"}
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
	if m := sp.Messages()[len(sp.Messages())-1]; m.Files[0] != "tasks/do/task.yaml" {
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
		FileModule:           head("variables:\n  - name: DESKTOP\n    title: D\n    type: bool\n  - name: DRIVER\n    title: G\n"),
		"tasks/do/task.yaml": "stage: go\ntitle: Driver\nconditions:\n  - DESKTOP == true\n  - DRIVER != none\n",
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

// A value a task shows and one a starting point asks for are both values the
// opening run of questions has no business asking: the first does not exist yet
// and the second stands for nothing once it has been used. Nothing declares
// that — being named is the declaration.
func TestBeingNamedIsWhatDefersAValue(t *testing.T) {
	dir := module(t, units(
		map[string]string{
			FileModule: head(`presets:
  - title: P
    options:
      - title: O
        asks: SOURCE
        apply: echo hi
variables:
  - name: DISK
    title: Disk
    required: true
  - name: LINK
    title: Shared at
  - name: SOURCE
    title: Configuration code
`),
		},
		unit("go", "do", "title: Do\nreport: Done\nshows: LINK\n"),
	))
	sp, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"LINK", "SOURCE"} {
		if !sp.Var(name).Deferred() {
			t.Errorf("%s is not deferred", name)
		}
	}
	if sp.Var("DISK").Deferred() {
		t.Error("an ordinary question was deferred")
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
