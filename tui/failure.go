package tui

import (
	"strings"

	"github.com/murkl/oak/internal/exec"
	"github.com/murkl/oak/internal/logging"

	"github.com/charmbracelet/lipgloss"
)

// One failure, drawn one way, wherever it comes from — the system check that
// refused to start, or a stage that died halfway through. A person reading it
// is not having a good day, and the last thing they need is two different
// shapes of bad news to learn.
//
// The order is what somebody actually needs, in that order: **where** — which
// module, which unit, which file and line, which command, which exit code —
// and then **what it said** on its way out, in the words of the tool itself.
//
// Where first, because that is the fixed part: the same handful of rows every
// time, read at a glance and in the same place on every failure. What a tool
// said is as long as the tool felt like being, and a paragraph of somebody
// else's prose above the rows would push them off the top of the page it is
// being read on.
//
// Nothing is folded away and nothing has to be pressed for.
func renderFailure(err error, width int) string {
	var b strings.Builder
	f, ok := err.(*exec.Failure)
	if !ok {
		b.WriteString(failStyle.Render(wrapped(err.Error(), width)))
		return b.String() + logNote(width)
	}

	// The label column is as wide as the widest label plus a gap, so the values
	// stand in one column — the same rule every other pair in this interface
	// lines up by.
	fields := f.Fields()
	labelW := 0
	for _, kv := range fields {
		labelW = max(labelW, len(kv.Label))
	}
	for _, kv := range fields {
		label := mutedStyle.Render(kv.Label + strings.Repeat(" ", labelW-len(kv.Label)))
		room := width - labelW - gapM
		value := truncate(kv.Value, room)
		if kv.Path {
			value = truncateStart(kv.Value, room)
		}
		b.WriteString(label + field(strings.Repeat(" ", gapM)) + softStyle.Render(value) + "\n")
	}
	// What the script itself said, which is the whole of what this page shows
	// of its output — the rest is in the log, and the line under it says where.
	if msg := strings.TrimSpace(f.Stderr); msg != "" {
		b.WriteString("\n" + failStyle.Render(wrapped(msg, width)) + "\n")
	}
	return b.String() + logNote(width)
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
// interface admits there is more output than it showed — and it is a path, not
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
// mid-word where there is not — for the one line here that is a path rather
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

// wrapped is body text at the reading width, joined back into one block.
func wrapped(s string, width int) string {
	return strings.Join(wrap(s, bodyWidth(width)), "\n")
}
