package store

import (
	"fmt"
	"os"
	"strings"

	"github.com/murkl/oak/internal/i18n"
)

// LangVar is what the language is called in Oak's own answer file. It is not a
// module's variable and never reaches a script: what a script does is the same
// in every language.
const LangVar = "OAK_LANG"

// Language is the one answer that belongs to the runtime rather than to any of
// its modules: the words all of them are read in.
//
// It cannot live in a module's answer file, because it is settled before a
// module has been chosen — and it should not be asked twice on a machine that
// has already said so. So it is kept beside them, in a file of the same shape
// and read the same way, holding the one thing that is not a module's business.
type Language struct {
	path string
	code string
}

// NewLanguage reads the runtime's own answers, or comes back empty where there
// are none — which is what a first run looks like.
func NewLanguage(path string) *Language {
	l := &Language{path: path}
	raw, err := os.ReadFile(path)
	if err != nil {
		return l
	}
	for line := range strings.SplitSeq(string(raw), "\n") {
		if name, value, ok := parseLine(line); ok && name == LangVar {
			l.code = value
		}
	}
	return l
}

// Code is the language settled last time, empty where none was.
func (l *Language) Code() string { return l.code }

// Set records a language. It does not save — the caller decides when the file
// is written.
func (l *Language) Set(code string) { l.code = code }

// Save writes the runtime's answers, whole and in one step, the way a module's
// are written.
func (l *Language) Save() error {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n", i18n.T("Answers kept for every module. Can be edited by hand."))
	entry(&b, LangVar, l.code, i18n.T("Interface language"))
	return write(l.path, b.String())
}
