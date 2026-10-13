package spec

import (
	"strings"
	"testing"
)

// The words for a status a script stops itself with are held to what a script
// can choose and to the answers they name, wherever they are declared.
func TestErrorsRefuseWhatNoScriptCanMean(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"no error at all", unit("go", "do", "title: Do\nerrors:\n  0: Fine.\n"), "errors: 0: a script chooses a status from 1 to 125"},
		{"a status bash keeps for itself", unit("go", "do", "title: Do\nerrors:\n  127: Gone.\n"), "errors: 127: a script chooses a status from 1 to 125"},
		{"words that say nothing", unit("go", "do", "title: Do\nerrors:\n  3: ' '\n"), "errors: 3: says nothing"},
		{"an answer nobody gives", unit("go", "do", "title: Do\nerrors:\n  3: '{{GHOST}} is gone.'\n"), "errors: 3: {{GHOST}} is not a variable"},
		{"a module's status out of range", map[string]string{FileModule: head("errors:\n  200: Late.\n")}, "module.yaml: errors: 200"},
		{"an action handed the terminal", units(
			map[string]string{FileModule: head("rules:\n  on-settings: [o]\n")},
			action("o", "title: O\ntty: true\nerrors:\n  3: No.\n"),
		), "errors: an action with tty runs without the trap"},
		{"an action that runs by itself", units(
			map[string]string{FileModule: head("rules:\n  start-if: [o]\n")},
			action("o", "title: O\nerror: No.\nerrors:\n  3: No.\n"),
		), "errors: it runs by itself where it is named"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Load(module(t, c.files))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want it to say %q", err, c.want)
			}
		})
	}
}

// A task's own words come first, the module's cover every status it has none
// for, and a status neither covers has none. The first paragraph heads it.
func TestTheWordsForAStatusAreTheUnitsThenTheModules(t *testing.T) {
	sp, err := Load(module(t, units(
		map[string]string{FileModule: head("variables:\n  - name: DISK\n    type: text\n    title: Disk\nerrors:\n  3: |\n    Offline\n\n    Connect\n    {{DISK}} again.\n  4: Module four.\n")},
		unit("go", "do", "title: Do\nerrors:\n  4: Task four.\n"),
	)))
	if err != nil {
		t.Fatal(err)
	}
	own := sp.Tasks[0].Errors
	get := func(string) string { return "/dev/sda" }

	if h, b, ok := sp.Told(own, 3, get); !ok || h != "Offline" || b != "Connect /dev/sda again." {
		t.Errorf("3 = %q, %q, %v, want the module's words, filled in and reflowed", h, b, ok)
	}
	if h, _, ok := sp.Told(own, 4, get); !ok || h != "Task four." {
		t.Errorf("4 = %q, %v, want the task's own words", h, ok)
	}
	if _, _, ok := sp.Told(own, 5, get); ok {
		t.Error("5 has words, and nobody wrote any")
	}
}

// Every one of them is a sentence a translator gets, said where it was written.
func TestTheWordsForAStatusAreTranslated(t *testing.T) {
	sp, err := Load(module(t, units(
		map[string]string{FileModule: head("variables:\n  - name: DISK\n    type: text\n    title: Disk\nerrors:\n  3: Module words.\n")},
		unit("go", "do", "title: Do\nerrors:\n  4: Task words.\n"),
	)))
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]string{}
	for _, m := range sp.Messages() {
		found[m.Text] = m.Files[0] + " " + m.Note
	}
	for text, want := range map[string]string{
		"Module words.": "module.yaml told where a script stops itself with exit 3",
		"Task words.":   "tasks/@go/do/task.yaml told where the step stops itself with exit 4",
	} {
		if !strings.HasPrefix(found[text], want) {
			t.Errorf("%q is listed as %q, want %q", text, found[text], want)
		}
	}
}
