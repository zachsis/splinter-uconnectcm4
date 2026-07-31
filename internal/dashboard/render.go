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

// crowd renders the vendor histogram as scaled bars within maxWidth columns.
func crowd(vs []VendorCount, maxWidth int) string {
	if len(vs) == 0 {
		return "(warming up…)"
	}
	top := vs[0].Count
	if top < 1 {
		top = 1
	}
	var b strings.Builder
	used := 0
	for _, v := range vs {
		bars := 1 + v.Count*6/top
		seg := fmt.Sprintf("%s %s %d  ", strings.Repeat("█", bars), companyLabel(v.ID), v.Count)
		if used+len(seg) > maxWidth {
			b.WriteString("…")
			break
		}
		b.WriteString(seg)
		used += len(seg)
	}
	return strings.TrimRight(b.String(), " ")
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
	fmt.Fprintf(&b, "  %s   %s  %s   peak %d  avg %.1f\n",
		paint(t.Label, "rate"), paint(t.Spark, Sparkline(s.RateHist, sparkW)),
		paint(t.Value, fmt.Sprintf("%d/s", cur)), peak, avg)
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
	fmt.Fprintf(&b, "  %s  %s\n\n", paint(t.Label, "crowd"), crowd(s.Vendors, width-9))
	b.WriteString("  " + paint(t.Dim, "+/- rate  ·  t theme  ·  q/Ctrl-C quit") + "\n")
	return b.String()
}
