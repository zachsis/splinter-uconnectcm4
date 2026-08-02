package dashboard

// Keyboard dispatch. These functions are pure (they act only on the Model and
// the Controls), with no terminal I/O, so they live outside the linux-only
// terminal driver and are unit-testable on any platform.

// rateStepMs is how much each +/- keypress changes the rotation interval.
const rateStepMs = 25

// handleInput dispatches the keys in one read buffer, decoding CSI escape
// sequences (arrows, PgUp/PgDn, Home/End) and single-byte keys. It returns how
// many bytes it consumed and whether to quit. A trailing *incomplete* escape
// sequence is left unconsumed so the caller can prepend it to the next read —
// otherwise a split arrow key (ESC ending one read, "[A" the next) would leak
// its bytes to single-key handlers like `a` or `D`.
func handleInput(data []byte, m *Model, c Controls) (consumed int, quit bool) {
	i := 0
	for i < len(data) {
		if data[i] == 0x1b { // ESC: maybe a CSI sequence
			adv, incomplete := parseEsc(data[i:], m)
			if incomplete {
				return i, false // carry data[i:] for the next read
			}
			i += adv
			continue
		}
		if handleSingle(data[i], m, c) {
			return i + 1, true
		}
		i++
	}
	return i, false
}

// parseEsc interprets a segment beginning with ESC. It returns how many bytes to
// advance, or incomplete=true when the segment is (or might be) the start of an
// unfinished CSI sequence that should be carried until more bytes arrive.
func parseEsc(seg []byte, m *Model) (advance int, incomplete bool) {
	if len(seg) < 2 {
		return 0, true // lone ESC at the end — more bytes may be coming
	}
	if seg[1] != '[' {
		return 1, false // ESC + non-'[' is a standalone ESC (unbound); consume just it
	}
	// CSI: scan for the final byte (0x40–0x7E). Params/intermediates precede it.
	for j := 2; j < len(seg); j++ {
		if b := seg[j]; b >= 0x40 && b <= 0x7e {
			applyCSI(seg[2:j+1], m)
			return j + 1, false
		}
	}
	return 0, true // no final byte yet — incomplete, carry
}

// applyCSI dispatches a CSI body (parameter bytes followed by the final byte).
func applyCSI(csi []byte, m *Model) {
	if len(csi) == 0 {
		return
	}
	final := csi[len(csi)-1]
	params := string(csi[:len(csi)-1])
	switch final {
	case 'A': // up
		m.Scroll(-1)
	case 'B': // down
		m.Scroll(1)
	case 'H': // Home
		m.ScrollToEnd(true)
	case 'F': // End
		m.ScrollToEnd(false)
	case '~': // tilde-terminated: PgUp/PgDn/Home/End
		switch params {
		case "1", "7":
			m.ScrollToEnd(true)
		case "4", "8":
			m.ScrollToEnd(false)
		case "5":
			m.ScrollPage(-1)
		case "6":
			m.ScrollPage(1)
		}
	}
	// 'C'/'D' (right/left) and any unknown final are consumed and ignored.
}

// handleSingle applies one single-byte key. Returns true to quit.
func handleSingle(b byte, m *Model, c Controls) bool {
	switch b {
	case 'q', 'Q', 0x03: // 0x03 = Ctrl-C (ISIG is off)
		if c.OnQuit != nil {
			c.OnQuit()
		}
		return true
	case '+', '=':
		if c.Rate != nil {
			m.SetRate(c.Rate.Adjust(-rateStepMs)) // shorter interval = faster
		}
	case '-', '_':
		if c.Rate != nil {
			m.SetRate(c.Rate.Adjust(+rateStepMs))
		}
	case 't', 'T':
		m.CycleTheme()
	case 'l', 'L':
		if c.Learn != nil {
			c.Learn.Request()
		}
	case 'a', 'A':
		if c.Apple != nil {
			m.SetAppleMode(c.Apple.Cycle())
		}
	case 's', 'S':
		if c.Trackers != nil {
			m.SetTrackers(c.Trackers.Toggle())
		}
	case ' ':
		if c.Broadcast != nil {
			m.SetPaused(c.Broadcast.Toggle())
		}
	case 'd', 'D':
		if c.Debug != nil {
			m.SetDebug(c.Debug.Toggle(), c.Debug.Path())
		}
	case '?':
		m.ToggleHelp()
	case '\t':
		m.CycleFocus()
	case 'j':
		m.Scroll(1)
	case 'k':
		m.Scroll(-1)
	case 'g':
		m.ScrollToEnd(true)
	case 'G':
		m.ScrollToEnd(false)
	}
	return false
}
