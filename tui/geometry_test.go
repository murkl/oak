package tui

import (
	"slices"
	"testing"
)

// A block is lifted to the golden section of the room it leaves rather than to
// its middle: of ten spare rows, four stand over it and six under it.
func TestARaisedBlockSitsAtTheGoldenSection(t *testing.T) {
	got := raised([]string{"a", "b"}, 12)
	want := []string{"", "", "", "", "a", "b"}
	if !slices.Equal(got, want) {
		t.Errorf("raised() = %q, want %q", got, want)
	}
}

// A block that already fills the height has no room to be lifted into, and one
// taller than it is not cut: shortening is the caller's decision.
func TestABlockWithNoRoomIsNotRaised(t *testing.T) {
	rows := []string{"a", "b", "c"}
	for _, height := range []int{3, 2} {
		if got := raised(rows, height); !slices.Equal(got, rows) {
			t.Errorf("raised(%d) = %q, want the block as it was", height, got)
		}
	}
}
