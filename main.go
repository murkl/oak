// Command oak draws an interface for programs that live beside it as files.
//
// On its own the binary does nothing: it draws an interface, asks questions,
// keeps the answers and runs shell in order, reporting where it broke. What is
// asked and what the shell does is a folder of yaml and scripts.
//
// One of those folders is a module, and they sit together in modules/ beside
// the binary. oak.yaml beside them says what the product they add up to is
// called and what it looks like; each module says the rest for itself. Nothing
// about any particular operating system is compiled in, so the same binary
// drives a different product by sitting next to a different oak.yaml and a
// different set of modules.
//
// The command line is five options and nothing else. Three are about a run:
// --version says what this binary is, --module opens one of the folders
// outright, and --debug hands every script DEBUG=true so a run can be watched
// without it touching anything. Two are about the folder rather than the run,
// for whoever is writing one: --inspect loads it the way a run does and reports
// what it holds, and --strings writes a module's translation template. What a
// product may declare is in the yaml beside the binary, never here.
package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/murkl/oak/internal/i18n"
	"github.com/murkl/oak/internal/inspect"
	"github.com/murkl/oak/internal/logging"
	"github.com/murkl/oak/internal/runner"
	"github.com/murkl/oak/internal/spec"
	"github.com/murkl/oak/internal/store"
	"github.com/murkl/oak/locales"
	"github.com/murkl/oak/tui"
)

// version is set by the build (see the Makefile). It is this binary's own and
// never a product's: what a product calls its own build is in its oak.yaml.
var version = "dev"

// The answers and the log live beside whoever started the program, never inside
// a module — which may be a read-only medium or a mounted image. A module's are
// named after the module, so two of them started from the same folder keep
// their own; Oak's own answers are named after Oak, beside them, and hold what
// is settled before any module has been chosen.
const (
	// program is what this binary is called: what it answers --version with,
	// and what its own answers are named after.
	program = "oak"

	confExt = ".conf"
	logExt  = ".log"

	runtimeConf = program + confExt
)

// The whole command line. Every one of them is spelled out in full and given
// its value with an equals sign, because a machine being installed is a machine
// somebody is reading a line off a screen onto, and there is nothing here worth
// abbreviating.
const (
	flagDebug   = "--debug"
	flagVersion = "--version"
	flagModule  = "--module"
	flagInspect = "--inspect"
	flagStrings = "--strings"
)

func main() {
	if err := start(os.Args[1:]); err != nil {
		die(err)
	}
}

// start reads the command line and does what it says.
func start(args []string) error {
	// A language before anything else, so even the message saying there is
	// nothing here to run is in one. Only the runtime's own catalogs exist this
	// early.
	i18n.Activate(i18n.Match(locale(), codes(i18n.Discover(locales.FS))), locales.FS)

	cmd, err := parse(args)
	if err != nil {
		return err
	}
	// Answered before anything is loaded: a version is what this binary is,
	// which is true of a binary standing on its own with no product beside it.
	if cmd.version {
		fmt.Println(program, version)
		return nil
	}

	rt, err := spec.LoadRuntime("")
	if err != nil {
		return err
	}
	mods, err := rt.LoadModules()
	if err != nil {
		return err
	}
	mods, err = narrow(rt, mods, cmd.module)
	if err != nil {
		return err
	}
	// Two questions about the folder rather than the run. Both are asked by
	// whoever is writing a product and never by a machine being installed, so
	// they answer on stdout and the interface is never drawn.
	switch {
	case cmd.inspect:
		return inspect.Report(os.Stdout, rt, mods, locales.FS)
	case cmd.strings:
		return inspect.Template(os.Stdout, rt, mods)
	}
	return run(rt, mods, cmd.debug)
}

// command is a command line, read.
type command struct {
	// module is the module it named, or empty where it named none — which is
	// the question the interface then asks. The two below narrow to it as well:
	// a report is about every module unless one was named, and a template
	// belongs to exactly one.
	module  string
	debug   bool
	version bool
	inspect bool
	strings bool
}

// parse reads one. Which module names exist is not decided here but by what is
// in modules/, so a name nobody declared is refused by narrow with everything
// on offer under it — and adding a module stays a folder rather than a change
// here.
func parse(args []string) (command, error) {
	var c command
	for _, arg := range args {
		name, value, valued := strings.Cut(arg, "=")
		switch {
		case name == flagDebug && !valued:
			c.debug = true
		case name == flagVersion && !valued:
			c.version = true
		case name == flagInspect && !valued:
			c.inspect = true
		case name == flagStrings && !valued:
			c.strings = true
		case name == flagModule && value != "":
			if c.module != "" {
				return c, fmt.Errorf("%s", i18n.T("One module at a time: %s or %s.", c.module, value))
			}
			c.module = value
		case name == flagModule:
			return c, fmt.Errorf("%s", i18n.T("%s needs the name of a module: %s=<id>.", flagModule, flagModule))
		default:
			return c, fmt.Errorf("%s\n%s",
				i18n.T("%q is not something this program takes.", arg),
				i18n.T("It takes %s.", strings.Join([]string{flagDebug, flagVersion, flagModule + "=<id>", flagInspect, flagStrings}, ", ")))
		}
	}
	return c, nil
}

// narrow cuts a run down to the module the command line named, or leaves every
// one of them where it named none — which is the question the interface then
// asks. A name no folder answers to is said so, with everything on offer under
// it.
func narrow(rt *spec.Runtime, mods []*spec.Module, id string) ([]*spec.Module, error) {
	if id == "" {
		return mods, nil
	}
	for _, mod := range mods {
		if mod.ID() == id {
			return []*spec.Module{mod}, nil
		}
	}
	return nil, fmt.Errorf("%s\n%s",
		i18n.T("No module called %s.", id),
		i18n.T("This one offers %s.", strings.Join(rt.Modules, ", ")))
}

func run(rt *spec.Runtime, mods []*spec.Module, debug bool) error {
	// The language is asked before a module is opened, so it is offered in every
	// language any of them speaks and their catalogs are laid over the runtime's
	// until one has been. Opening a module narrows them to its own.
	sources := catalogs(mods...)
	langs := i18n.Discover(sources...)

	lang := saved()
	i18n.Activate(language(lang.Code(), langs), sources...)

	opening := &tui.Opening{
		Runtime: rt, Modules: mods, Lang: lang, Langs: langs, Sources: sources,
		Oak: version,
	}
	return tui.Run(opening, func(mod *spec.Module) (*tui.Program, error) {
		return open(mod, debug)
	})
}

// saved is the runtime's own answers: the language, kept for every module.
//
// It sits in the folder a module's answers sit in, so that one folder holds one
// product's files and nothing else.
func saved() *store.Language {
	beside, err := filepath.Abs(".")
	if err != nil {
		beside = "."
	}
	return store.NewLanguage(filepath.Join(beside, runtimeConf))
}

// open makes one module runnable: the answers it keeps and the file they
// survive in, the log, and the runner that joins the module to the answers.
// Everything here is named after the module, which is why none of it happens
// before one has been chosen.
func open(mod *spec.Module, debug bool) (*tui.Program, error) {
	// What this module's answers and log are called, after the module itself.
	conf, err := filepath.Abs(mod.ID() + confExt)
	if err != nil {
		return nil, err
	}

	st := store.New(mod, conf, debug)
	if err := st.Load(); err != nil {
		return nil, err
	}

	// Opened before the first page of this module is drawn — and only now,
	// because where it goes follows where the answers go.
	if err := logging.Init(filepath.Join(filepath.Dir(conf), mod.ID()+logExt)); err != nil {
		return nil, err
	}
	logging.Info("%s", mod.UI.Title)

	// This module's catalogs are laid over the runtime's, so a module may reword
	// anything. The language itself is the runtime's and is already settled.
	sources := catalogs(mod)
	langs := i18n.Discover(sources...)
	i18n.Activate(i18n.Current(), sources...)

	// The answers survived a restart in the file; their effect on the live system
	// did not.
	rn := runner.New(mod, st)
	rn.Settle()

	return &tui.Program{Module: mod, Store: st, Runner: rn, Langs: langs, Sources: sources}, nil
}

// catalogs is every source of words a run has: the runtime's own, and each
// module's laid over them. A module that declares none simply speaks the
// runtime's.
//
// Several modules at once is what the language page and the page asking which
// to open need, since none of them is the one this run is about yet.
func catalogs(mods ...*spec.Module) []fs.FS {
	out := []fs.FS{locales.FS}
	for _, mod := range mods {
		if mod.Locales != "" {
			out = append(out, os.DirFS(mod.Locales))
		}
	}
	return out
}

// language settles which one to speak: the stored choice if it is still on
// offer, otherwise whatever this machine's own locale comes closest to, and
// otherwise the language everything is written in.
func language(saved string, langs []i18n.Lang) string {
	all := codes(langs)
	if saved != "" && slices.Contains(all, saved) {
		return saved
	}
	if code := i18n.Match(locale(), all); code != "" {
		return code
	}
	return i18n.SourceLang
}

// locale is what this machine says it speaks, in the order POSIX gives those
// answers.
func locale() string {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return ""
}

func codes(langs []i18n.Lang) []string {
	out := make([]string, len(langs))
	for i, l := range langs {
		out[i] = l.Code
	}
	return out
}

// die reports on stderr and in the log, then leaves. The one thing not shown
// inside the interface, because everything that gives the interface its name,
// its colours and its words is in what could not be read.
func die(err error) {
	msg := strings.TrimRight(err.Error(), "\n")
	logging.Error("%s", msg)
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}
