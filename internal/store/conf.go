package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/murkl/oak/internal/i18n"
)

// The answer file is shell rather than yaml: `KEY='value' # what it is` reads
// in any editor on a bare live medium and is the shape a script gets its
// variables in, so nothing is translated either way.

// Load reads the answer file over the values already held; a missing file is a
// first run. A key nothing declares any more is passed over, and a value that
// breaks its rules is kept as written for Missing to ask again.
func (s *Store) Load() error {
	raw, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for line := range strings.SplitSeq(string(raw), "\n") {
		name, value, ok := parseLine(line)
		if !ok {
			continue
		}
		v := s.mod.Var(name)
		if v == nil || v.Secret() || v.Deferred() {
			continue
		}
		s.val[name] = value
	}
	return nil
}

// Exists reports whether this machine has answered anything yet. It is the one
// question asked before the interface opens: an answer file is what turns a
// first run into a return.
func (s *Store) Exists() bool {
	_, err := os.Stat(s.path)
	return err == nil
}

// Save writes every answer worth keeping in declaration order, each with its
// description as a trailing comment, so the file reads like the questions.
// Secrets, derived and deferred answers and an option's page are left out: none
// of them is an answer the next run should read back.
func (s *Store) Save() error {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n", i18n.T("Answers for %s. Can be edited by hand.", s.mod.Name()))
	group := ""
	for _, v := range s.mod.Vars {
		if v.Secret() || v.Derived() || v.Deferred() {
			continue
		}
		if v.Group != group {
			group = v.Group
			if group != "" {
				fmt.Fprintf(&b, "\n# %s\n", i18n.T(group))
			}
		}
		// The title, not the description: a trailing comment is a label for the
		// line it sits on, and this file reads as a list of names and values.
		// What each one is for is a paragraph, and it is on the page that asks
		// the question.
		entry(&b, v.Name, s.val[v.Name], v.Label())
	}
	return write(s.path, b.String())
}

// Reset drops every answer back to its declared start and deletes the file, so
// what follows is a first run. The log stays, since what was done to the
// machine is still true of it.
func (s *Store) Reset() error {
	for _, v := range s.mod.Declared() {
		s.val[v.Name] = v.Default.String()
	}
	if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// write puts an answer file on disk whole and in one step, so an interrupted
// run never leaves a half-file. 0600, since a module may ask for something the
// rest of the machine has no business reading.
func write(path, content string) error {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	tmp := path + ".new"
	if err := os.WriteFile(tmp, []byte(content), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// entry writes one line: the name, the value quoted so any character survives,
// and what the value is for.
func entry(b *strings.Builder, name, value, desc string) {
	fmt.Fprintf(b, "%s=%s", name, quote(value))
	if desc = strings.TrimSpace(desc); desc != "" {
		fmt.Fprintf(b, " # %s", desc)
	}
	b.WriteByte('\n')
}

// quote wraps a value in single quotes, the one shell quoting that has no
// escapes inside it: everything is literal until the next quote, and a quote
// itself is written by closing, escaping it, and opening again.
func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// parseLine reads one `KEY=value` line back, in the three shapes a person might
// have left it in: single-quoted, double-quoted, or bare. Anything else - a
// comment, a blank line, a line with no name in front of the equals - is not an
// answer and is passed over.
func parseLine(line string) (name, value string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	name, rest, found := strings.Cut(line, "=")
	name = strings.TrimSpace(name)
	if !found || name == "" {
		return "", "", false
	}
	rest = strings.TrimLeft(rest, " \t")
	switch {
	case strings.HasPrefix(rest, "'"):
		return name, unquoteSingle(rest[1:]), true
	case strings.HasPrefix(rest, `"`):
		value, _, _ = strings.Cut(rest[1:], `"`)
		return name, value, true
	}
	// Bare: the value runs to the first space, since anything after it is a
	// trailing comment or a second word nobody meant as part of the value.
	value, _, _ = strings.Cut(rest, " ")
	return name, strings.TrimSpace(value), true
}

// unquoteSingle reads a single-quoted shell string back, including the
// close-escape-open dance that puts a quote inside one.
func unquoteSingle(s string) string {
	var b strings.Builder
	for {
		i := strings.IndexByte(s, '\'')
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		b.WriteString(s[:i])
		rest := s[i+1:]
		if !strings.HasPrefix(rest, `\''`) {
			return b.String()
		}
		b.WriteByte('\'')
		s = rest[3:]
	}
}
