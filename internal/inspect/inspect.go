// Package inspect answers what a build script asks about a folder of modules.
//
// The load is the same one a run does at startup, so everything it refuses
// would have stopped the program too. What is left is to say what was found, or
// to write one module's translation template — the two things --inspect and
// --strings answer with, on stdout and without drawing anything.
package inspect

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/murkl/oak/internal/i18n"
	"github.com/murkl/oak/internal/spec"
)

// Report says what a folder holds — what the product is called, and every
// module it offers or the one that was named — without touching anything.
//
// base is where the runtime's own catalogs come from, which a module's are laid
// over to work out how much of it a language actually covers.
func Report(w io.Writer, rt *spec.Runtime, mods []*spec.Module, base fs.FS) error {
	reportRuntime(w, rt)
	unread, drifted := 0, 0
	for _, mod := range mods {
		d, err := report(w, mod, base)
		if err != nil {
			return err
		}
		drifted += d
		n, err := reportUnread(w, mod)
		if err != nil {
			return err
		}
		unread += n
	}
	// The two things here that are verdicts rather than descriptions, so this
	// fails where a build script runs it — see spec.Unread and drift. Both are
	// said at once: a run that reported them wants them all fixed, not the
	// first one found.
	var faults []string
	if unread > 0 {
		faults = append(faults, fmt.Sprintf("%d question(s) asked where nothing reads the answer", unread))
	}
	if drifted > 0 {
		faults = append(faults, fmt.Sprintf("%d translation(s) naming other {{VAR}} than the source", drifted))
	}
	if len(faults) > 0 {
		return errors.New(strings.Join(faults, "; "))
	}
	return nil
}

// reportUnread names every question this module asks under conditions no task
// that reads it can run under, and how many there were.
func reportUnread(w io.Writer, mod *spec.Module) (int, error) {
	unread, err := mod.Unread()
	if err != nil {
		return 0, err
	}
	for _, u := range unread {
		fmt.Fprintf(w, "  %-10s %s\n", "unread", u)
	}
	return len(unread), nil
}

// reportRuntime is what the product says about itself, printed.
func reportRuntime(w io.Writer, rt *spec.Runtime) {
	fmt.Fprintf(w, "%s\n", rt.File)
	fmt.Fprintf(w, "  title      %s\n", rt.Title)
	fmt.Fprintf(w, "  version    %s\n", rt.Version)
	fmt.Fprintf(w, "  accent     %s\n", rt.Accent)
	fmt.Fprintf(w, "  url        %s\n", rt.URL)
	fmt.Fprintf(w, "  logo       %d lines\n", len(strings.Split(strings.TrimRight(rt.Logo, "\n"), "\n")))
	fmt.Fprintf(w, "  modules    %s\n", strings.Join(rt.Modules, " "))
}

// report is what one module holds, printed.
func report(w io.Writer, mod *spec.Module, base fs.FS) (int, error) {
	required, secret, derived := 0, 0, 0
	for _, v := range mod.Vars {
		switch {
		case v.Derived():
			derived++
		case v.Secret():
			secret++
		}
		if v.Required {
			required++
		}
	}
	sources := catalogs(mod, base)
	langs := i18n.Discover(sources...)
	names := make([]string, len(langs))
	for i, l := range langs {
		names[i] = l.Code
	}
	fmt.Fprintf(w, "%s\n", filepath.Join(mod.Dir, spec.FileModule))
	fmt.Fprintf(w, "  title      %s\n", mod.UI.Title)
	// Only where it says something. A module on offer everywhere is the ordinary
	// case, and a line saying so on every one of them would drown the one that
	// does not.
	if !mod.Requires.Empty() {
		fmt.Fprintf(w, "  requires   %s\n", oneLine(mod.Requires))
	}
	fmt.Fprintf(w, "  variables  %d (%d required, %d secret, %d derived)\n", len(mod.Vars), required, secret, derived)
	fmt.Fprintf(w, "  presets    %d\n", len(mod.Presets))
	fmt.Fprintf(w, "  stages     %s\n", strings.Join(mod.Stages, " "))
	fmt.Fprintf(w, "  tasks      %d (%d checked)\n", len(mod.Tasks), checks(mod))
	if filled := hooks(mod); len(filled) > 0 {
		fmt.Fprintf(w, "  hooks      %s\n", strings.Join(filled, " "))
	}
	fmt.Fprintf(w, "  languages  %s\n", strings.Join(names, " "))

	// What the module's shell reaches for and nothing here answers. Not a
	// verdict — $HOME belongs on this line — but the only place a name that
	// used to arrive from Oak and no longer does is visible at all, since in
	// shell it is an empty string rather than an error. See spec.Unset.
	unset, err := mod.Unset()
	if err != nil {
		return 0, err
	}
	if len(unset) > 0 {
		fmt.Fprintf(w, "  unset      %s\n", strings.Join(unset, " "))
	}

	// What loaded and still says something that can never take effect. A
	// description rather than a verdict, so it is reported and the run goes on.
	for _, warning := range mod.Warnings {
		fmt.Fprintf(w, "  %-10s %s\n", "needs", warning)
	}

	// The order they run in is worked out rather than written down anywhere.
	for i, t := range mod.Tasks {
		fmt.Fprintf(w, "  %2d. %-10s %-20s %s\n", i+1, t.Stage(), t.ID(), checked(t))
	}

	// A catalog whose keys have drifted from the yaml shows up here as a
	// coverage that dropped, which is the only way a stale translation is
	// noticed.
	//
	// What a translation says with the {{VAR}} is its own business — German
	// puts them in another order - but which ones it says is not: one dropped
	// leaves the sentence naming no disk, and one misspelled is the same thing
	// with the typo out of sight in a file nobody rereads.
	drifted := 0
	msgs := mod.Messages()
	for _, l := range langs {
		if l.Code == i18n.SourceLang {
			continue
		}
		i18n.Activate(l.Code, sources...)
		done := 0
		for _, m := range msgs {
			if !i18n.Has(m.Text) {
				continue
			}
			done++
			for _, said := range drift(m.Text, i18n.T(m.Text)) {
				fmt.Fprintf(w, "  %-10s %s: %s\n", l.Code, said, oneSentence(m.Text))
				drifted++
			}
		}
		fmt.Fprintf(w, "  %-10s %d of %d strings translated\n", l.Code, done, len(msgs))
	}
	return drifted, nil
}

// drift says what a translation does with the source's {{VAR}} that it should
// not: the ones it leaves out and the ones it made up. Sets rather than lists,
// since the order they appear in is the translator's to choose.
func drift(source, translated string) []string {
	var said []string
	for _, name := range missing(spec.Names(source), spec.Names(translated)) {
		said = append(said, "translation drops {{"+name+"}}")
	}
	for _, name := range missing(spec.Names(translated), spec.Names(source)) {
		said = append(said, "translation adds {{"+name+"}}")
	}
	return said
}

// missing is every name in want that have does not have, once each and in the
// order want has them.
func missing(want, have []string) []string {
	var out []string
	for _, name := range want {
		if !slices.Contains(have, name) && !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	return out
}

// oneSentence is as much of a message as names it in a table: a report is read
// down its left edge, and a confirm text runs to five lines. Cut by character
// rather than by byte — the strings it cuts are the ones with the em dashes in
// them.
func oneSentence(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 48 {
		return string(r[:47]) + "\u2026"
	}
	return s
}

// oneLine is a piece of a module's shell as a report can print it: the file it
// lives in, or its first line with the rest marked as being there. A report is
// a table, and a module that wrote ten lines of check into its yaml must not
// push every other row off the page.
func oneLine(s spec.Script) string {
	if s.File != "" {
		return filepath.Base(s.File)
	}
	first, rest, cut := strings.Cut(strings.TrimSpace(s.Shell), "\n")
	if cut && strings.TrimSpace(rest) != "" {
		return first + " …"
	}
	return first
}

// hooks is which of the runtime's hooks this module fills, so one that is not
// being run because of a typo in a folder name is visible as one missing from
// this line.
func hooks(mod *spec.Module) []string {
	var out []string
	for _, name := range spec.Hooks {
		if n := len(mod.Hook(name)); n > 0 {
			out = append(out, fmt.Sprintf("%s(%d)", name, n))
		}
	}
	return out
}

// catalogs is every source of words a module has: the runtime's own, and the
// module's laid over them.
func catalogs(mod *spec.Module, base fs.FS) []fs.FS {
	out := []fs.FS{base}
	if mod.Locales != "" {
		out = append(out, os.DirFS(mod.Locales))
	}
	return out
}

// Template writes the translation template for one module: every word it says,
// each with its translation left empty, in the order it says them. Redirect it
// to locales/<name>.pot, and a catalog for a language is that file with the
// right-hand side filled in — by hand, or on a platform that speaks po.
func Template(w io.Writer, rt *spec.Runtime, mods []*spec.Module) error {
	// One template belongs to one module. Which of several is not something to
	// guess at, so it is named on the command line rather than picked here.
	if len(mods) > 1 {
		return fmt.Errorf("%d modules here — name one: %s", len(mods), strings.Join(rt.Modules, ", "))
	}
	mod := mods[0]
	msgs := mod.Messages()
	entries := make([]i18n.Entry, 0, len(msgs))
	for _, m := range msgs {
		entries = append(entries, i18n.Entry{Text: m.Text, Note: m.Note, Refs: m.Files})
	}
	return i18n.Template(w, mod.ID(), entries)
}

// checks is how many tasks say how to tell that they worked, which is what a
// run validates once it is over.
func checks(mod *spec.Module) int {
	n := 0
	for _, t := range mod.Tasks {
		if t.Checks() {
			n++
		}
	}
	return n
}

// checked marks a task that carries one, so the listing says which of them a
// run will look at afterwards.
func checked(t *spec.Task) string {
	if t.Checks() {
		return "test"
	}
	return ""
}
