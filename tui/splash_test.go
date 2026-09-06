package tui

import (
	"strings"
	"testing"

	"github.com/murkl/oak/internal/i18n"
)

// run takes a splash all the way to its last frame, which is where everything
// on it has arrived.
func run(m *splashModel) {
	for !m.advance() {
	}
}

// The one version on the splash is Oak's own: the product's build is in the
// corner of every page behind it, and the wordmark above already says the name.
func TestTheSplashSignsOffWithOaksOwnVersion(t *testing.T) {
	i18n.Use(i18n.SourceLang)

	m := newSplash("OAK", "1.2.3")
	run(m)

	if want := "powered by oak 1.2.3"; !strings.Contains(m.View(60, 20), want) {
		t.Errorf("the splash does not say %q:\n%s", want, m.View(60, 20))
	}
}

// The sign-off is the last thing to arrive, so it never sweeps in alongside the
// wordmark it belongs under.
func TestTheSignOffArrivesAfterTheWordmark(t *testing.T) {
	m := newSplash("OAK", "1.2.3")

	if got := m.signShown(); got != 0 {
		t.Errorf("signShown at the start = %v, want 0", got)
	}
	m.elapsed = fadeFor
	if got := m.signShown(); got != 0 {
		t.Errorf("signShown with the wordmark just settled = %v, want 0", got)
	}
	run(m)
	if got := m.signShown(); got != 1 {
		t.Errorf("signShown at the end = %v, want 1", got)
	}
}
