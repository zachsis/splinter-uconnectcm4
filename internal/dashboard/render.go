package dashboard

import (
	"fmt"
	"strings"
	"time"

	"github.com/zachsis/helmofhades/internal/decoy"
	"github.com/zachsis/helmofhades/internal/verify"
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

// companyLabel maps a vendor ID to a friendly name (shared with the decoy/engine).
func companyLabel(id uint16) string { return decoy.CompanyLabel(id) }

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

// crowdRows renders the vendor histogram as htop-style rows (name · bar · count),
// one per vendor with no truncation (the caller scrolls). vendors must be sorted
// by count descending.
func crowdRows(vendors []VendorCount, width int) []string {
	if len(vendors) == 0 {
		return nil
	}
	top := vendors[0].Count
	if top < 1 {
		top = 1
	}
	nameW := 0
	for _, v := range vendors {
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
	lines := make([]string, 0, len(vendors))
	for _, v := range vendors {
		bars := v.Count * barMax / top
		if bars < 1 {
			bars = 1
		}
		lines = append(lines, fmt.Sprintf("%-*s %-*s %d", nameW, companyLabel(v.ID), barMax, strings.Repeat("█", bars), v.Count))
	}
	return lines
}

// deviceID returns the display ID and friendly label for an observed device.
func deviceID(o verify.Observation) (id, label string) {
	switch {
	case o.Tile:
		return "svc FEED", "Tile"
	case o.FastPair:
		return "svc FE2C", "Fast Pair"
	case o.HasMfg:
		return fmt.Sprintf("%#04x", o.CompanyID), companyLabel(o.CompanyID)
	default:
		return "—", "—"
	}
}

// learnedRows formats observed devices as rows: MAC · RSSI · ID · label · name.
// Columns are fixed-width, so unlike crowdRows this doesn't need the terminal
// width.
func learnedRows(devices []verify.Observation) []string {
	rows := make([]string, 0, len(devices))
	for _, o := range devices {
		id, label := deviceID(o)
		name := o.Name
		if name == "" {
			name = "—"
		}
		rssi := "   ?"
		if o.RSSI != 0 {
			rssi = fmt.Sprintf("%4d", o.RSSI)
		}
		rows = append(rows, fmt.Sprintf("%s %sdBm  %-8s %-10s %s", macStr(o.MAC), rssi, id, label, truncate(name, 14)))
	}
	return rows
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

// scrollView returns the display lines for one scrollable table: a title line
// (with a position indicator + ▲/▼ when scrollable) and the visible slice of
// rows starting at offset. focused marks the active table with a ▶ marker.
func scrollView(title string, rows []string, offset, budget int, focused bool, t Theme) []string {
	if budget < 1 {
		budget = 1
	}
	total := len(rows)
	if offset > total-budget {
		offset = total - budget
	}
	if offset < 0 {
		offset = 0
	}
	end := offset + budget
	if end > total {
		end = total
	}
	marker := "  "
	if focused {
		marker = paint(t.Value, "▶ ")
	}
	head := marker + paint(t.Label, title)
	if total > budget {
		head += paint(t.Dim, fmt.Sprintf("  [%d–%d/%d]", offset+1, end, total))
		if offset > 0 {
			head += paint(t.Dim, " ▲")
		}
		if end < total {
			head += paint(t.Dim, " ▼")
		}
	}
	out := []string{head}
	if total == 0 {
		return append(out, "    "+paint(t.Dim, "(none yet)"))
	}
	for _, r := range rows[offset:end] {
		out = append(out, "    "+paint(t.Value, r))
	}
	return out
}

func graphRowsFor(height int) int {
	if height > 0 && height < 18 {
		return 2
	}
	return 4
}

// TableRows is how many data rows EACH of the crowd/learned tables shows for the
// given terminal height. The render loop and the model's scroll clamping both
// use it so scrolling matches what's drawn.
func TableRows(height int) int {
	if height <= 0 {
		return 6
	}
	rem := height - (16 + graphRowsFor(height)) // fixed header/graph/status/footer lines
	if rem < 4 {
		rem = 4
	}
	r := rem / 2
	if r < 2 {
		r = 2
	}
	if r > 15 {
		r = 15
	}
	return r
}

// renderHelp draws the full-screen help overlay (the `?` key).
func renderHelp(s Snapshot, width, height int) string {
	t := s.Theme
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", paint(t.Header, "hohd dashboard — help"))
	keys := [][2]string{
		{"+ / -", "raise / lower the live rotation rate (dwell)"},
		{"t", "cycle color theme"},
		{"l", "learn: scan nearby BLE, bias decoys to the local mix"},
		{"a", "cycle Apple decoy mode (off → naive → nearby-info)"},
		{"s", "toggle service-data trackers (Tile / Fast Pair)"},
		{"Space", "stop / start broadcasting (keeps the adapter)"},
		{"Tab", "switch scroll focus: crowd ↔ learned tables"},
		{"↑ ↓ · PgUp PgDn · Home End", "scroll the focused table"},
		{"D", "toggle a debug log file in the current directory"},
		{"?", "show / hide this help"},
		{"q / Ctrl-C", "quit and restore the adapter to bluetoothd"},
	}
	for _, kv := range keys {
		fmt.Fprintf(&b, "  %s  %s\n", paint(t.Value, fmt.Sprintf("%-27s", kv[0])), paint(t.Label, kv[1]))
	}
	b.WriteString("\n  " + paint(t.Dim, "press ? or q to return") + "\n")
	return b.String()
}

// RenderFrame builds the dashboard frame text for a snapshot and terminal size.
// It emits only visible content (no cursor/screen control) so it is unit-testable.
func RenderFrame(s Snapshot, width, height int) string {
	if width < 40 {
		width = 40
	}
	if s.HelpOpen {
		return renderHelp(s, width, height)
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
	header := fmt.Sprintf("hohd — %s · adv %dms · dwell %dms", s.Mode, s.AdvMs, s.RotateMs)
	if s.AppleMode != "" && s.AppleMode != "off" {
		header += " · apple " + s.AppleMode
	}
	if s.TrackersOn {
		header += " · trackers on"
	}
	head := paint(t.Header, header)
	if s.Paused {
		head += " " + paint(t.Warn, "⏸ PAUSED")
	}
	if s.DebugOn {
		head += " " + paint(t.Warn, "⏺ REC")
	}
	fmt.Fprintf(&b, "%s\n\n", head)
	fmt.Fprintf(&b, "  %s %s     %s %d     Bluetooth hci0 (exclusive)\n\n",
		paint(t.Label, "uptime"), fmtDur(s.Uptime), paint(t.Label, "total"), s.Total)
	fmt.Fprintf(&b, "  %s   %s   peak %d  avg %.1f\n",
		paint(t.Label, "rate"), paint(t.Value, fmt.Sprintf("%d/s", cur)), peak, avg)
	for _, line := range blockGraph(s.RateHist, sparkW, graphRowsFor(height)) {
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
		fmt.Fprintf(&b, "  %s    %s  %s  %s\n", paint(t.Label, "now"),
			macStr(s.LastAddr), paint(t.Value, fmt.Sprintf("%q", name)), paint(t.Label, companyLabel(s.LastID)))
	}
	if s.LearnActive {
		fmt.Fprintf(&b, "  %s  %s\n", paint(t.Warn, "learn"), "scanning ambient devices…")
	} else if s.LearnLine != "" {
		fmt.Fprintf(&b, "  %s  %s\n", paint(t.Label, "learn"), s.LearnLine)
	}
	b.WriteString("\n")

	budget := TableRows(height)
	for _, line := range scrollView("crowd", crowdRows(s.Vendors, width-6), s.CrowdScroll, budget, s.Focus == FocusCrowd, t) {
		b.WriteString(line + "\n")
	}
	b.WriteString("\n")
	for _, line := range scrollView("learned devices", learnedRows(s.Learned), s.LearnScroll, budget, s.Focus == FocusLearned, t) {
		b.WriteString(line + "\n")
	}
	b.WriteString("\n  " + paint(t.Dim, "? help · space pause · tab/↑↓ scroll · t theme · l learn · q quit") + "\n")
	return b.String()
}
