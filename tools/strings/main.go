// Command strings writes one module's translation template to stdout.
//
// Every word the module says, in the order it says them, each with its
// translation left empty. A catalog for a language is that file with the
// right-hand side filled in.
//
//	go run ./tools/strings example setup > example/modules/setup/locales/setup.pot
package main

import (
	"fmt"
	"os"

	"github.com/murkl/oak/internal/inspect"
)

func main() {
	if len(os.Args) != 3 {
		fail(fmt.Errorf("usage: strings <product folder> <module>"))
	}
	rt, mods, err := inspect.Open(os.Args[1], os.Args[2])
	if err != nil {
		fail(err)
	}
	if err := inspect.Template(os.Stdout, rt, mods); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
