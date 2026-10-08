package spec

import "strings"

// Expand fills {{VAR}} in a sentence from the answers given, a notation apart
// from the $VAR of the shell beside it. A name nothing answers is left empty,
// which Module.checkText rules out when the module loads.
func Expand(s string, get func(string) string) string {
	if !strings.Contains(s, "{{") {
		return s
	}
	var b strings.Builder
	scan(s, func(text, name string) {
		b.WriteString(text)
		if name != "" {
			b.WriteString(get(name))
		}
	})
	return b.String()
}

// Names is every {{VAR}} a sentence asks for, in the order it asks for them.
func Names(s string) []string {
	var names []string
	scan(s, func(_, name string) {
		if name != "" {
			names = append(names, name)
		}
	})
	return names
}

// scan walks a sentence, handing over each run of plain text together with the
// name that follows it, and the tail on its own under an empty name. One
// reading of the notation, so what gets filled in and what gets checked can
// never be two different things.
func scan(s string, each func(text, name string)) {
	for {
		i := strings.Index(s, "{{")
		if i < 0 {
			break
		}
		j := strings.Index(s[i:], "}}")
		if j < 0 {
			break
		}
		each(s[:i], strings.TrimSpace(s[i+2:i+j]))
		s = s[i+j+2:]
	}
	each(s, "")
}
