package store

import (
	"fmt"
	"os"
	"strings"

	"github.com/murkl/oak/internal/i18n"
	"github.com/murkl/oak/internal/spec"
)

// The names Oak keeps its own answers under. Neither is a module's variable and
// neither ever reaches a script: what a script does is the same in every
// language, and whether a run looks at its own work afterwards is the runtime's
// business rather than the module's.
const (
	LangVar     = "OAK_LANG"
	ValidateVar = "OAK_VALIDATE"
)

// Preferences is what belongs to the runtime rather than to any of its modules:
// the words all of them are read in, and whether a run checks its own work as
// it goes.
//
// Neither can live in a module's answer file. The language is settled before a
// module has been chosen, and both hold for every module a product offers — so
// they are kept beside them, in a file of the same shape, read the same way,
// holding the things that are not a module's business.
type Preferences struct {
	path     string
	lang     string
	validate bool
}

// NewPreferences reads the runtime's own answers, or comes back on the defaults
// where there are none — which is what a first run looks like.
//
// Validation is on unless the file says otherwise. A run that checks its own
// work is what somebody who has never thought about it wants; the setting is
// there for whoever has.
func NewPreferences(path string) *Preferences {
	p := &Preferences{path: path, validate: true}
	raw, err := os.ReadFile(path)
	if err != nil {
		return p
	}
	for line := range strings.SplitSeq(string(raw), "\n") {
		name, value, ok := parseLine(line)
		if !ok {
			continue
		}
		switch name {
		case LangVar:
			p.lang = value
		case ValidateVar:
			p.validate = value != spec.BoolFalse
		}
	}
	return p
}

// Lang is the language settled last time, empty where none was.
func (p *Preferences) Lang() string { return p.lang }

// Validates reports whether a run checks its own work as it goes.
func (p *Preferences) Validates() bool { return p.validate }

// Neither records anything on disk: the caller decides when the file is
// written, the same way a module's answers are.
func (p *Preferences) SetLang(code string)  { p.lang = code }
func (p *Preferences) SetValidates(on bool) { p.validate = on }

// Save writes the runtime's answers, whole and in one step, the way a module's
// are written.
func (p *Preferences) Save() error {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n", i18n.T("Answers kept for every module. Can be edited by hand."))
	entry(&b, LangVar, p.lang, i18n.T("Interface language"))
	entry(&b, ValidateVar, truth(p.validate), i18n.T("Check every step on the machine it was done to"))
	return write(p.path, b.String())
}

// truth is a bool as the answer file spells one, which is as a script reads it.
func truth(on bool) string {
	if on {
		return spec.BoolTrue
	}
	return spec.BoolFalse
}
