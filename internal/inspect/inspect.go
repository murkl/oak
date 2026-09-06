// Package inspect answers what a build script asks about a folder of modules.
//
// The load is the same one a run does at startup, so everything it refuses
// would have stopped the program too. What is left is to say what was found, or
// to write one module's translation template — the two things --inspect and
// --strings answer with, on stdout and without drawing anything.
package inspect

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
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
	unread := 0
	for _, mod := range mods {
		if err := report(w, mod, base); err != nil {
			return err
		}
		n, err := reportUnread(w, mod)
		if err != nil {
			return err
		}
		unread += n
	}
	// The one thing here that is a verdict rather than a description, so this
	// fails where a build script runs it — see spec.Unread.
	if unread > 0 {
		return fmt.Errorf("%d question(s) asked where nothing reads the answer", unread)
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
	fmt.Fprintf(w, "  logo       %d lines\n", len(strings.Split(strings.TrimRight(rt.Logo, "\n"), "\n")))
	fmt.Fprintf(w, "  modules    %s\n", strings.Join(rt.Modules, " "))
}

// report is what one module holds, printed.
func report(w io.Writer, mod *spec.Module, base fs.FS) error {
	required, secret := 0, 0
	for _, v := range mod.Vars {
		if v.Required {
			required++
		}
		if v.Secret() {
			secret++
		}
	}
	sources := catalogs(mod, base)
	langs := i18n.Discover(sources...)
	names := make([]string, len(langs))
	for i, l := range langs {
		names[i] = l.Code
	}
	fmt.Fprintf(w, "%s\n", filepath.Join(mod.Dir, mod.File))
	fmt.Fprintf(w, "  title      %s\n", mod.UI.Title)
	fmt.Fprintf(w, "  variables  %d (%d required, %d secret)\n", len(mod.Vars), required, secret)
	fmt.Fprintf(w, "  presets    %d\n", len(mod.Presets))
	fmt.Fprintf(w, "  stages     %s\n", strings.Join(mod.Stages, " "))
	fmt.Fprintf(w, "  tasks      %d\n", len(mod.Tasks))
	fmt.Fprintf(w, "  hooks      %s\n", strings.Join(hooks(mod), " "))
	fmt.Fprintf(w, "  languages  %s\n", strings.Join(names, " "))

	// What the module's shell reaches for and nothing here answers. Not a
	// verdict — $HOME belongs on this line — but the only place a name that
	// used to arrive from Oak and no longer does is visible at all, since in
	// shell it is an empty string rather than an error. See spec.Unset.
	unset, err := mod.Unset()
	if err != nil {
		return err
	}
	if len(unset) > 0 {
		fmt.Fprintf(w, "  unset      %s\n", strings.Join(unset, " "))
	}

	// The order they run in is worked out rather than written down anywhere.
	for i, t := range mod.Tasks {
		fmt.Fprintf(w, "  %2d. %-10s %s\n", i+1, t.Stage, t.ID())
	}

	// A catalog whose keys have drifted from the yaml shows up here as a
	// coverage that dropped, which is the only way a stale translation is
	// noticed.
	msgs := mod.Messages()
	for _, l := range langs {
		if l.Code == i18n.SourceLang {
			continue
		}
		i18n.Activate(l.Code, sources...)
		done := 0
		for _, m := range msgs {
			if i18n.Has(m.Text) {
				done++
			}
		}
		fmt.Fprintf(w, "  %-10s %d of %d strings translated\n", l.Code, done, len(msgs))
	}
	return nil
}

// hooks is which of them this module actually has, so one that is not being
// called because of a typo in its name is visible as one missing from this line.
func hooks(mod *spec.Module) []string {
	var out []string
	for _, name := range spec.HookNames {
		if mod.Hook(name) != "" {
			out = append(out, name)
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
