package tui

import (
	"strings"

	"github.com/murkl/oak/internal/exec"
	"github.com/murkl/oak/internal/logging"
	"github.com/murkl/oak/internal/spec"

	"github.com/charmbracelet/lipgloss"
)

// renderFailure draws every failure one way: first where, the module, unit,
// file and line, command and exit code, in fixed rows, then what the tool said
// on its way out. Nothing is folded away or has to be pressed for. Where rows
// is above zero and it all would not fit, what the tool said loses its front:
// its last lines are where it says why it stopped.
func renderFailure(err error, width, rows int) string {
	if k, ok := err.(*known); ok {
		return k.view(width)
	}
	f, ok := err.(*exec.Failure)
	if !ok {
		return fitted(nil, wrap(err.Error(), bodyWidth(width)), logNote(width), rows)
	}

	// The label column is as wide as the widest label plus a gap, so the values
	// stand in one column - the same rule every other pair in this interface
	// lines up by.
	fields := f.Fields()
	labelW := 0
	for _, kv := range fields {
		labelW = max(labelW, len(kv.Label))
	}
	head := make([]string, 0, len(fields))
	for _, kv := range fields {
		label := mutedStyle.Render(kv.Label + strings.Repeat(" ", labelW-len(kv.Label)))
		room := width - labelW - gapM
		value := truncate(kv.Value, room)
		if kv.Path {
			value = truncateStart(kv.Value, room)
		}
		head = append(head, label+field(strings.Repeat(" ", gapM))+softStyle.Render(value))
	}
	// What the script itself said, which is the whole of what this page shows
	// of its output - the rest is in the log, and the line under it says where.
	var said []string
	if msg := strings.TrimSpace(f.Stderr); msg != "" {
		said = wrap(msg, bodyWidth(width))
	}
	return fitted(head, said, logNote(width), rows)
}

// fitted lays a failure out in its three blocks, a blank line between each,
// and cuts the front of the middle one to rows where there is a limit.
func fitted(head, said []string, note string, rows int) string {
	// Under the fields the blocks stand a blank line apart; a failure that is
	// only a sentence has the note straight under it.
	gap := 0
	if len(head) > 0 {
		gap = 1
	}
	notes := strings.Count(note, "\n")
	if rows > 0 && len(said) > 0 {
		room := rows - len(head) - notes - 2*gap
		switch {
		case room < 2:
			said = nil
		case room < len(said):
			said = append([]string{glyphs.spell.Replace("…")}, said[len(said)-room+1:]...)
		}
	}
	var b strings.Builder
	for _, line := range head {
		b.WriteString(line + "\n")
	}
	if len(said) > 0 {
		b.WriteString(strings.Repeat("\n", gap) + failStyle.Render(strings.Join(said, "\n")) + strings.Repeat("\n", gap))
	}
	return b.String() + note
}

// known is a failure the module saw coming: a script that stopped itself with a
// status the module has words for. It reads as those words alone. What broke
// stays in the log, for whoever is helping, and nothing is offered about it:
// the words say what to do.
type known struct {
	cause          error
	headline, body string
}

func (k *known) Error() string { return k.cause.Error() }
func (k *known) Unwrap() error { return k.cause }

// view is its words, the headline under the mark that says no.
func (k *known) view(width int) string {
	out := refusal(k.headline, width)
	if k.body != "" {
		out += "\n\n" + paragraph(k.body, width)
	}
	return out
}

// page is its words as the page a run or an action stops on.
func (k *known) page() *report { return newReport(k.headline, k.body, "").stop() }

// told is a failure in the module's words, where its script stopped itself with
// a status they cover - own first, then the module's - and err as it was
// everywhere else.
func (a *app) told(err error, own spec.Errors) error {
	f, ok := err.(*exec.Failure)
	if !ok || !f.Deliberate {
		return err
	}
	headline, body, ok := a.module.Told(own, f.Code, a.store.Get)
	if !ok {
		return err
	}
	return &known{cause: err, headline: headline, body: body}
}

// refusal is a sentence saying no, under the mark that says so: the first line
// beside the mark, the rest standing under it.
func refusal(text string, width int) string {
	lines := wrap(text, max(width-markW, 1))
	if len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(failStyle.Render(glyphs.fail) + field(" ") + boldStyle.Render(lines[0]))
	for _, line := range lines[1:] {
		b.WriteString("\n" + field("  ") + boldStyle.Render(line))
	}
	return b.String()
}

// said is a failure in its own words: what the tool printed on its way out,
// which is the half of a failure written for somebody to read. Empty where it
// said nothing, and the whole error where it is not one of ours.
func said(err error) string {
	if f, ok := err.(*exec.Failure); ok {
		return strings.TrimSpace(f.Stderr)
	}
	return err.Error()
}

// unitOf is what a failure happened to, where it knows.
func unitOf(err error) string {
	if f, ok := err.(*exec.Failure); ok {
		return f.Unit
	}
	return ""
}

// logNote says where everything that did not fit is. It is the one place the
// interface admits there is more output than it showed - and it is a path, not
// an offer to show it: a page of somebody else's build log inside this frame
// would be the frame breaking.
func logNote(width int) string {
	path := logging.Path()
	if path == "" {
		return ""
	}
	// Broken across lines rather than cut short: this is the one thing on the
	// page that has to survive being read out to somebody else, and half a path
	// is worth nothing.
	return "\n" + mutedStyle.Render(strings.Join(hardWrap(labelLogHint(path), width), "\n"))
}

// hardWrap breaks a string at the width, on a space where there is one and
// mid-word where there is not - for the one line here that is a path rather
// than a sentence.
func hardWrap(s string, width int) []string {
	var out []string
	for _, line := range wrap(s, width) {
		for lipgloss.Width(line) > width {
			cut := []rune(line)[:width]
			out = append(out, string(cut))
			line = string([]rune(line)[width:])
		}
		out = append(out, line)
	}
	return out
}
