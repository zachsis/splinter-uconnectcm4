package dashboard

import (
	"fmt"
	"strings"
	"time"
)

var blocks = []rune("▁▂▃▄▅▆▇█")

// Sparkline renders the last `width` samples as unicode block characters, scaled
// to the window's max, right-aligned (newest at the right).
func Sparkline(samples []int, width int) string {
	if width <= 0 {
		return ""
	}
	if len(samples) > width {
		samples = samples[len(samples)-width:]
	}
	max := 1
	for _, v := range samples {
		if v > max {
			max = v
		}
	}
	var b strings.Builder
	for i := 0; i < width-len(samples); i++ {
		b.WriteByte(' ')
	}
	for _, v := range samples {
		idx := v * (len(blocks) - 1) / max
		if idx < 0 {
			idx = 0
		}
		if idx >= len(blocks) {
			idx = len(blocks) - 1
		}
		b.WriteRune(blocks[idx])
	}
	return b.String()
}

// blockGraph renders samples as a `rows`-tall vertical bar graph using block
// glyphs (▁..█), right-aligned so the newest sample is at the right. It returns
// `rows` lines, top row first. Each column's value is scaled to the window max
// and drawn across rows*8 vertical levels.
func blockGraph(samples []int, width, rows int) []string {
	if rows < 1 {
		rows = 1
	}
	if width < 1 {
		width = 1
	}
	if len(samples) > width {
		samples = samples[len(samples)-width:]
	}
	max := 1
	for _, v := range samples {
		if v > max {
			max = v
		}
	}
	levels := rows * 8
	pad := width - len(samples)

	lines := make([]string, rows)
	for r := rows - 1; r >= 0; r-- { // r = 0 is the bottom row
		var b strings.Builder
		for i := 0; i < pad; i++ {
			b.WriteByte(' ')
		}
		base := r * 8
		for _, v := range samples {
			eighths := v * levels / max
			switch {
			case eighths >= base+8:
				b.WriteRune('█')
			case eighths <= base:
				b.WriteByte(' ')
			default:
				b.WriteRune(blocks[eighths-base-1]) // 1..7 -> ▁..▇
			}
		}
		lines[rows-1-r] = b.String() // top row first
	}
	return lines
}

// companyLabel maps the vendor IDs splinter emits to friendly names for display.
func companyLabel(id uint16) string {
	switch id {
	case 0x0075:
		return "Samsung"
	case 0x00E0:
		return "Google"
	case 0x009E:
		return "Bose"
	case 0x0087:
		return "Garmin"
	case 0x012D:
		return "Sony"
	case 0x0157:
		return "Huami"
	case 0x0059:
		return "Nordic"
	case 0x0171:
		return "Amazon"
	default:
		return fmt.Sprintf("0x%04x", id)
	}
}

// rateStats returns current, peak, and average from a rate history window.
func rateStats(h []int) (cur, peak int, avg float64) {
	if len(h) == 0 {
		return 0, 0, 0
	}
	sum := 0
	for _, v := range h {
		sum += v
		if v > peak {
			peak = v
		}
	}
	return h[len(h)-1], peak, float64(sum) / float64(len(h))
}

func fmtDur(d time.Duration) string {
	s := int(d.Seconds())
	return fmt.Sprintf("%02d:%02d:%02d", s/3600, (s%3600)/60, s%60)
}

func macStr(a [6]byte) string {
	return fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X", a[0], a[1], a[2], a[3], a[4], a[5])
}

// crowdTable renders the vendor histogram as an htop-style table (sorted desc):
// aligned name column, a proportional bar, and the numeric count. It returns at
// most maxRows vendor lines plus a "… +N more" line when truncated. vendors must
// already be sorted by count descending.
func crowdTable(vendors []VendorCount, width, maxRows int) []string {
	if maxRows < 1 {
		maxRows = 1
	}
	if len(vendors) == 0 {
		return []string{"(warming up…)"}
	}
	shown := vendors
	truncated := 0
	if len(shown) > maxRows {
		truncated = len(shown) - maxRows
		shown = shown[:maxRows]
	}

	top := shown[0].Count
	if top < 1 {
		top = 1
	}
	nameW := 0
	for _, v := range shown {
		if n := len(companyLabel(v.ID)); n > nameW {
			nameW = n
		}
	}
	barMax := width - nameW - 10
	if barMax < 4 {
		barMax = 4
	}
	if barMax > 24 {
		barMax = 24
	}

	lines := make([]string, 0, len(shown)+1)
	for _, v := range shown {
		bars := v.Count * barMax / top
		if bars < 1 {
			bars = 1
		}
		lines = append(lines, fmt.Sprintf("%-*s %-*s %d", nameW, companyLabel(v.ID), barMax, strings.Repeat("█", bars), v.Count))
	}
	if truncated > 0 {
		lines = append(lines, fmt.Sprintf("… +%d more", truncated))
	}
	return lines
}

// RenderFrame builds the dashboard frame text for a snapshot and terminal size.
// It emits only visible content (no cursor/screen control) so it is unit-testable.
func RenderFrame(s Snapshot, width, height int) string {
	if width < 40 {
		width = 40
	}
	cur, peak, avg := rateStats(s.RateHist)
	failPct := 0.0
	if s.Total+s.FailsCum > 0 {
		failPct = 100 * float64(s.FailsCum) / float64(s.Total+s.FailsCum)
	}
	sparkW := width - 24
	if sparkW > histLen {
		sparkW = histLen // no point in a field wider than the history we keep
	}
	if sparkW < 10 {
		sparkW = 10
	}

	t := s.Theme
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", paint(t.Header,
		fmt.Sprintf("splinterd — %s · adv %dms · dwell %dms", s.Mode, s.AdvMs, s.RotateMs)))
	fmt.Fprintf(&b, "  %s %s     %s %d     Bluetooth hci0 (exclusive)\n\n",
		paint(t.Label, "uptime"), fmtDur(s.Uptime), paint(t.Label, "total"), s.Total)
	fmt.Fprintf(&b, "  %s   %s   peak %d  avg %.1f\n",
		paint(t.Label, "rate"), paint(t.Value, fmt.Sprintf("%d/s", cur)), peak, avg)
	graphRows := 4
	if height > 0 && height < 18 {
		graphRows = 2 // short screen: keep it compact
	}
	for _, line := range blockGraph(s.RateHist, sparkW, graphRows) {
		fmt.Fprintf(&b, "  %s\n", paint(t.Spark, line))
	}
	fmt.Fprintf(&b, "  %s  %s  %s\n\n",
		paint(t.Label, "fails"), paint(t.Warn, Sparkline(s.FailHist, sparkW)),
		paint(t.Warn, fmt.Sprintf("%.1f%%", failPct)))
	if s.HaveLast {
		name := s.LastName
		if name == "" {
			name = "(nameless)"
		}
		fmt.Fprintf(&b, "  %s    %s  %s  %s\n\n", paint(t.Label, "now"),
			macStr(s.LastAddr), paint(t.Value, fmt.Sprintf("%q", name)), paint(t.Label, companyLabel(s.LastID)))
	}
	fmt.Fprintf(&b, "  %s\n", paint(t.Label, "crowd"))
	crowdRows := 6
	if height > 0 {
		crowdRows = height - graphRows - 13 // leave room for the fixed lines
		if crowdRows < 2 {
			crowdRows = 2
		}
		if crowdRows > 8 {
			crowdRows = 8
		}
	}
	for _, line := range crowdTable(s.Vendors, width-4, crowdRows) {
		fmt.Fprintf(&b, "    %s\n", paint(t.Value, line))
	}
	b.WriteString("\n  " + paint(t.Dim, "+/- rate  ·  t theme  ·  q/Ctrl-C quit") + "\n")
	return b.String()
}
