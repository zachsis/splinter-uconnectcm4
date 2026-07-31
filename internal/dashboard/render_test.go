package dashboard

import (
	"strings"
	"testing"
)

func TestSparkline(t *testing.T) {
	// No samples: width spaces, no blocks.
	if got := Sparkline(nil, 5); got != "     " {
		t.Errorf("empty sparkline = %q, want 5 spaces", got)
	}
	// Full range maps low->▁ and high->█.
	s := []rune(Sparkline([]int{0, 1, 2, 3, 4, 5, 6, 7}, 8))
	if s[0] != '▁' || s[7] != '█' {
		t.Errorf("range sparkline = %q, want ▁..█", string(s))
	}
	// Wider than samples: left-padded to width.
	if got := []rune(Sparkline([]int{7}, 4)); len(got) != 4 || got[3] != '█' {
		t.Errorf("padded sparkline = %q, want width 4 ending █", string(got))
	}
	// More samples than width: keep the newest `width`.
	if got := []rune(Sparkline([]int{1, 2, 3, 4, 5}, 3)); len(got) != 3 {
		t.Errorf("clamped sparkline len = %d, want 3", len(got))
	}
}

func TestRenderFrame(t *testing.T) {
	m := New("dense", 100, 200)
	m.Decoy([6]byte{0xC0, 0x11, 0x22, 0x33, 0x44, 0x55}, 0x0075, "Galaxy Buds")
	m.Rate(5, 0)
	m.Rate(6, 1)

	f := RenderFrame(m.Snapshot(), 90, 24)
	for _, want := range []string{
		"splinterd", "dense", "adv 100ms", "dwell 200ms",
		"rate", "fails", "Galaxy Buds", "Samsung", "crowd", "+/- rate", "quit",
	} {
		if !strings.Contains(f, want) {
			t.Errorf("frame missing %q:\n%s", want, f)
		}
	}
	// fail% should reflect 1 fail out of (2 total + 1 fail).
	if !strings.Contains(f, "%") {
		t.Errorf("frame missing fail percentage:\n%s", f)
	}
}

func TestRenderFrameNarrow(t *testing.T) {
	m := New("paced", 100, 250)
	m.Rate(4, 0)
	// Must not panic and must still render at a tiny width.
	f := RenderFrame(m.Snapshot(), 20, 10)
	if !strings.Contains(f, "splinterd") {
		t.Errorf("narrow frame missing header:\n%s", f)
	}
}
