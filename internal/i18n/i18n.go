// Package i18n translates the words the program shows, with the English source
// string as the key, so a missing translation leaves readable English rather
// than a key. The catalogs are gettext po files from a generated pot, the
// runtime's and each module's merged at startup.
package i18n

import (
	"fmt"
	"strings"
)

// SourceLang is the language every msgid is written in. It has no catalog of
// its own - the template lists every message in it, and a message nothing
// translates is shown exactly as the code and the yaml wrote it.
const SourceLang = "en"

// Catalog is one language: what it calls itself, and what it has to say.
type Catalog struct {
	// Language is the name in the language itself - "Deutsch", not "German".
	// It is not a field of its own in the file: it is the translation of
	// LanguageName, so a catalog names its language the same way it says
	// everything else.
	Language string
	Messages map[string]string
}

// active is what is currently showing. Package state rather than a value passed
// down: every layer of the program says words, and threading a translator
// through all of them would put the argument in a hundred signatures to serve
// one process that only ever speaks one language at a time.
var (
	active = map[string]string{}
	lang   = SourceLang
)

// Use puts the program in a language, built from the catalogs given. Later
// catalogs win, so a module can reword the runtime, and nil ones are skipped.
func Use(code string, catalogs ...*Catalog) {
	lang = code
	active = map[string]string{}
	for _, c := range catalogs {
		if c == nil {
			continue
		}
		for k, v := range c.Messages {
			if v != "" {
				active[k] = v
			}
		}
	}
}

// Current is the code currently showing.
func Current() string { return lang }

// T translates, then formats, so a translation may move its placeholders where
// its sentence needs them. It may not reorder them among themselves: msgfmt's
// c-format check refuses Go's "%[2]d".
func T(msg string, a ...any) string {
	out := msg
	if t, ok := active[msg]; ok {
		out = t
	}
	if len(a) == 0 {
		return out
	}
	return fmt.Sprintf(out, a...)
}

// Has reports whether the active language has something of its own to say about
// a message. Not the same question as whether T changes it: a word a language
// happens to spell exactly as English does - a product name, "Kernel" - is
// translated, and answering otherwise would send somebody looking for a gap
// that is not there.
func Has(msg string) bool {
	_, ok := active[msg]
	return ok
}

// Match picks the best of the available languages for a locale as the
// environment writes it ("de_DE.UTF-8", "de", "C"): the exact code, then the
// part before the underscore. Empty means none fits and the caller stays on the
// source language.
func Match(locale string, available []string) string {
	locale = strings.ToLower(strings.TrimSpace(locale))
	if i := strings.IndexAny(locale, ".@"); i >= 0 {
		locale = locale[:i]
	}
	locale = strings.ReplaceAll(locale, "_", "-")
	if locale == "" || locale == "c" || locale == "posix" {
		return ""
	}
	for _, code := range available {
		if strings.EqualFold(code, locale) {
			return code
		}
	}
	base, _, _ := strings.Cut(locale, "-")
	for _, code := range available {
		if strings.EqualFold(code, base) {
			return code
		}
	}
	return ""
}
