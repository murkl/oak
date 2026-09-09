package spec

import (
	"fmt"
	"strings"
)

// order settles what runs when inside one stage.
//
// Two rules, and no third: a task runs after every task of an earlier stage,
// and after whatever it named in `needs`. The stages are the run read from top
// to bottom and are settled by the folders the tasks sit in, so all that is
// left here is the order across one stage.
//
// Two tasks that no `needs` separates are independent. The same module always
// produces the same list — a run has to be repeatable — but which of the two
// comes first is not something to build on: it is what `needs` is for, and a
// folder renamed is a folder renamed and nothing else.
//
// Working it out here rather than keeping a list of steps somewhere means the
// two can never disagree: a folder added is a step added, and its place comes
// out of what it says about itself.
func order(tasks []*Task) ([]*Task, error) {
	waits := make(map[string]map[string]bool, len(tasks))
	for _, t := range tasks {
		waits[t.id] = map[string]bool{}
		for _, n := range t.Needs {
			if n == t.id {
				return nil, fmt.Errorf("%s: needs itself", t.id)
			}
			waits[t.id][n] = true
		}
	}

	out := make([]*Task, 0, len(tasks))
	done := map[string]bool{}
	for len(out) < len(tasks) {
		next := ready(tasks, waits, done)
		if next == nil {
			return nil, fmt.Errorf("tasks wait on each other: %s", strings.Join(cycle(tasks, waits, done), " → "))
		}
		out = append(out, next)
		done[next.id] = true
	}
	return out, nil
}

// ready is the next task that can run: the first one still waiting for nothing.
func ready(tasks []*Task, waits map[string]map[string]bool, done map[string]bool) *Task {
	for _, t := range tasks {
		if done[t.id] {
			continue
		}
		if satisfied(waits[t.id], done) {
			return t
		}
	}
	return nil
}

func satisfied(waits map[string]bool, done map[string]bool) bool {
	for id := range waits {
		if !done[id] {
			return false
		}
	}
	return true
}

// cycle is the ring the tasks that are left are waiting round, named in the
// order they wait: a → b → c → a. Nothing can run any more, so what is left
// holds at least one, and following any name into it arrives at it.
//
// A list of the ones still waiting would say the same thing and leave whoever
// reads it to work the ring out by hand, which is the whole of what there is to
// do about it.
func cycle(tasks []*Task, waits map[string]map[string]bool, done map[string]bool) []string {
	left := map[string]bool{}
	for _, t := range tasks {
		if !done[t.id] {
			left[t.id] = true
		}
	}
	// Any name still waiting leads into a ring: follow one need at a time until
	// somewhere is arrived at twice, and the ring is what came after it.
	var path []string
	seen := map[string]int{}
	for at := first(tasks, left); at != ""; {
		if i, round := seen[at]; round {
			return append(path[i:], at)
		}
		seen[at] = len(path)
		path = append(path, at)
		at = firstNeed(waits[at], left)
	}
	return path
}

// first is the task still waiting that comes first in the stage's own order, so
// the same module always reports the same ring.
func first(tasks []*Task, left map[string]bool) string {
	for _, t := range tasks {
		if left[t.id] {
			return t.id
		}
	}
	return ""
}

// firstNeed is one thing a task is still waiting for, by name so the walk is
// the same on every run.
func firstNeed(waits map[string]bool, left map[string]bool) string {
	best := ""
	for id := range waits {
		if left[id] && (best == "" || id < best) {
			best = id
		}
	}
	return best
}
