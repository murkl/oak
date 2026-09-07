package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every clock in the interface goes through after, which is what lets a test
// run the whole thing on no time at all. One tea.Tick written straight into a
// page would still work and still draw correctly — it would only be invisible
// to that, and the suite would go back to sitting out every animation it has.
// So the rule is checked rather than remembered.
func TestEveryClockGoesThroughAfter(t *testing.T) {
	sources, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range sources {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(body), "\n") {
			// The one place it is named: after is tea.Tick.
			if strings.Contains(line, "var after = tea.Tick") {
				continue
			}
			if strings.Contains(line, "tea.Tick(") {
				t.Errorf("%s:%d sets a clock of its own — use after:\n\t%s",
					path, i+1, strings.TrimSpace(line))
			}
		}
	}
}
