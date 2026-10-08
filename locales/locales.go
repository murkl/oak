// Package locales holds Oak's own words in every language, compiled into the
// binary, one <code>.po per language filled in from the generated oak.pot
// (`make locales`). A module's words are translated in its own locales/ folder.
package locales

import "embed"

// The template is deliberately not embedded: it says nothing a running program
// needs, and every message in it is already in the binary as the source text.
//
//go:embed *.po
var FS embed.FS
