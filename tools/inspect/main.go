// Command inspect loads a product the way a run does and reports what it holds,
// without running any of it.
//
// Every task ordered, every condition resolved, every question checked against
// the tasks that read it — so a change to what a product may declare fails on a
// real product before it reaches anybody else. It is the check to put in a
// build script, and it is a tool rather than a flag on the binary because it is
// a question about a folder: whoever is writing a module asks it, and a machine
// being installed never does.
//
//	go run ./tools/inspect example              # every module of that product
//	go run ./tools/inspect example installer    # just that one
package main

import (
	"fmt"
	"os"

	"github.com/murkl/oak/internal/inspect"
	"github.com/murkl/oak/locales"
)

func main() {
	dir, id := "", ""
	switch len(os.Args) {
	case 3:
		id = os.Args[2]
		fallthrough
	case 2:
		dir = os.Args[1]
	default:
		fail(fmt.Errorf("usage: inspect <product folder> [module]"))
	}
	rt, mods, err := inspect.Open(dir, id)
	if err != nil {
		fail(err)
	}
	if err := inspect.Report(os.Stdout, rt, mods, locales.FS); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
