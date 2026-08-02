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
	m.SetColor(false) // mono -> plain text for deterministic substring checks
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

func TestThemeColorAndCycle(t *testing.T) {
	m := New("paced", 100, 250)
	m.Rate(4, 0)

	// Color on (default, matrix) -> frame contains SGR color codes.
	if got := RenderFrame(m.Snapshot(), 80, 24); !strings.Contains(got, "\x1b[38;5;") {
		t.Errorf("colored frame missing SGR codes")
	}
	// Mono -> no color codes at all (byte-identical to plain text).
	m.SetColor(false)
	if got := RenderFrame(m.Snapshot(), 80, 24); strings.Contains(got, "\x1b[38") {
		t.Errorf("mono frame should have no color codes:\n%q", got)
	}
	// UseTheme validates names; CycleTheme advances.
	m.SetColor(true)
	if !m.UseTheme("amber") || m.UseTheme("bogus") {
		t.Errorf("UseTheme name handling wrong")
	}
	before := m.Snapshot().Theme.Name
	m.CycleTheme()
	if m.Snapshot().Theme.Name == before {
		t.Errorf("CycleTheme did not advance from %q", before)
	}
}

func TestBlockGraph(t *testing.T) {
	// Columns at the window max fill every row with full blocks.
	g := blockGraph([]int{5, 5, 5}, 3, 4)
	if len(g) != 4 {
		t.Fatalf("want 4 rows, got %d", len(g))
	}
	for _, line := range g {
		if line != "███" {
			t.Fatalf("max column should be all full blocks: %q", line)
		}
	}
	// Zero is blank across all rows.
	for _, line := range blockGraph([]int{0, 0}, 2, 3) {
		if strings.TrimRight(line, " ") != "" {
			t.Fatalf("zero should be blank: %q", line)
		}
	}
	// Right-aligned: fewer samples than width => leading spaces, newest at right.
	if p := blockGraph([]int{5}, 4, 1); p[0] != "   █" {
		t.Fatalf("padded graph = %q, want '   █'", p[0])
	}
	// A mid value fills the bottom row and only partially fills the top.
	m := blockGraph([]int{5, 3}, 2, 2) // col2: eighths = 3*16/5 = 9 -> bottom full, top ▁
	if r := []rune(m[len(m)-1]); r[1] != '█' {
		t.Errorf("bottom of mid column should be full: %q", m[len(m)-1])
	}
	if r := []rune(m[0]); r[1] == '█' || r[1] == ' ' {
		t.Errorf("top of mid column should be a partial block: %q", m[0])
	}
}

func TestCrowdTable(t *testing.T) {
	vs := []VendorCount{{0x0075, 33}, {0x0087, 12}, {0x012D, 8}, {0x0059, 5}, {0x0171, 3}}
	tbl := crowdTable(vs, 40, 3)
	if len(tbl) != 4 { // 3 rows + a "… +2 more" line
		t.Fatalf("want 3 rows + more-line, got %d: %v", len(tbl), tbl)
	}
	if !strings.Contains(tbl[0], "Samsung") || !strings.Contains(tbl[0], "33") {
		t.Errorf("first row wrong: %q", tbl[0])
	}
	if !strings.Contains(tbl[len(tbl)-1], "+2 more") {
		t.Errorf("truncation line wrong: %q", tbl[len(tbl)-1])
	}
	// The top vendor's bar is the longest.
	if b0, b1 := strings.Count(tbl[0], "█"), strings.Count(tbl[1], "█"); b0 <= b1 {
		t.Errorf("top bar (%d) should exceed second (%d)", b0, b1)
	}
	// Empty -> a warming-up placeholder, not a crash.
	if e := crowdTable(nil, 40, 3); len(e) != 1 || !strings.Contains(e[0], "warming") {
		t.Errorf("empty case: %v", e)
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
