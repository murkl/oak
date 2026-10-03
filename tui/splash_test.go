package tui

import (
	"strings"
	"testing"
)

// run takes a splash all the way to its last frame, which is where everything
// on it has arrived.
func run(m *splashModel) {
	for !m.advance() {
	}
}

// The one version on the splash is the product's: the wordmark above already
// says whose it is, and the corner of every page behind it says the same number.
func TestTheSplashShowsTheProductsVersionUnderTheWordmark(t *testing.T) {
	m := newSplash("OAK", "1.2.3")
	run(m)

	view := m.View(60, 20)
	if !strings.Contains(view, "1.2.3") {
		t.Errorf("the splash does not say its version:\n%s", view)
	}
	if strings.Contains(view, "powered by") {
		t.Errorf("the splash says who drew it:\n%s", view)
	}
}

// A product that names no version has nothing to put there.
func TestTheSplashOfAProductWithNoVersionSaysNothingUnderTheWordmark(t *testing.T) {
	m := newSplash("OAK", "")
	run(m)

	if got := strings.TrimSpace(m.signature()); got != "" {
		t.Errorf("signature = %q, want none", got)
	}
}

// The version is the last thing to arrive, so it never sweeps in alongside the
// wordmark it belongs under.
func TestTheVersionArrivesAfterTheWordmark(t *testing.T) {
	m := newSplash("OAK", "1.2.3")

	if got := m.signature(); got != "" {
		t.Errorf("signature at the start = %q, want none", got)
	}
	m.elapsed = sweepFor - animEvery
	if got := m.signature(); got != "" {
		t.Errorf("signature a frame before the wordmark has settled = %q, want none", got)
	}
	m.elapsed = sweepFor
	if got := m.signature(); !strings.Contains(got, "1.2.3") {
		t.Errorf("signature with the wordmark just settled = %q, want the version", got)
	}
}

// Once the wordmark is in, nothing on the splash moves or changes colour until
// the page replaces it: the version appears and that is all. A fade here is
// what looks wrong on a console drawn over a framebuffer.
func TestTheSplashStandsStillOnceTheWordmarkIsIn(t *testing.T) {
	m := newSplash("OAK", "1.2.3")
	m.elapsed = sweepFor
	settled := m.View(60, 20)

	for !m.advance() {
		if got := m.View(60, 20); got != settled {
			t.Fatalf("the splash changed at %v:\n%s\n---\n%s", m.elapsed, settled, got)
		}
	}
}

// A key cuts the splash short, and nothing is left to run out.
func TestSkippingTheSplashEndsItAtOnce(t *testing.T) {
	m := newSplash("OAK", "1.2.3")
	m.skip()

	if !m.over() {
		t.Error("the splash is still running after it was skipped")
	}
}
