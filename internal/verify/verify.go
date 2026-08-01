// Package verify analyzes observed BLE advertisements to confirm splinterd's
// decoy crowd matches the expected behavior: enough distinct devices, a spread
// of vendor IDs, and none of the popup-triggering formats splinterd must never
// emit. The parsing and analysis are pure so they are unit-tested without radio.
package verify

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/zachsis/splinter-uconnectcm4/internal/decoy"
)

// Observation is one advertising report seen by the scanner.
type Observation struct {
	MAC         [6]byte
	Connectable bool
	CompanyID   uint16 // valid only when HasMfg
	HasMfg      bool
	Name        string
	FastPair    bool // carried a Google Fast Pair (0xFE2C) service-data block
	AppleFindMy bool // carried an Apple Find My / offline-finding (0x12) message
}

// ParseAdvData walks a raw advertising payload and extracts the fields the
// harness cares about. Malformed structures are skipped rather than erroring —
// the scanner sees real-world traffic, not just splinterd's.
func ParseAdvData(data []byte) (companyID uint16, hasMfg bool, name string, fastPair bool, appleFindMy bool) {
	for i := 0; i < len(data); {
		l := int(data[i])
		if l == 0 || i+1+l > len(data) {
			break
		}
		typ := data[i+1]
		body := data[i+2 : i+1+l]
		switch typ {
		case 0x08, 0x09: // shortened / complete local name
			name = string(body)
		case 0xFF: // manufacturer specific data
			if len(body) >= 2 {
				companyID = uint16(body[0]) | uint16(body[1])<<8
				hasMfg = true
				if companyID == decoy.CompanyApple && hasAppleMsgType(body[2:], decoy.AppleFindMyType) {
					appleFindMy = true
				}
			}
		case 0x16: // service data - 16-bit UUID
			if len(body) >= 2 {
				uuid := uint16(body[0]) | uint16(body[1])<<8
				if uuid == decoy.ServiceGoogleFastPair {
					fastPair = true
				}
			}
		}
		i += 1 + l
	}
	return
}

// hasAppleMsgType reports whether the Apple Continuity payload (the bytes after
// the 0x004C company ID) contains a message of the given type. Continuity packs
// [type][len][payload...] messages back-to-back. The type byte is inspected at
// every message boundary, including a lone trailing type byte with no length,
// so a Find My type in the final position can't slip past the detector.
func hasAppleMsgType(payload []byte, msgType byte) bool {
	for i := 0; i < len(payload); {
		if payload[i] == msgType {
			return true
		}
		if i+1 >= len(payload) {
			break // trailing type byte with no length; already checked above
		}
		i += 2 + int(payload[i+1])
	}
	return false
}

// Result is the outcome of analyzing a scan window.
type Result struct {
	Window       time.Duration
	Total        int
	DistinctMACs int
	IDSpread     int
	IDHistogram  map[uint16]int
	Violations   []string // hard guardrail failures
	Notes        []string // soft warnings (don't fail the run)
	RateChecked  bool
	ObservedRate float64 // distinct MACs per second
	RateFloor    float64 // required minimum when RateChecked
	Pass         bool
}

// Analyze evaluates observations. expectedRotateMs is splinterd's --rotate-ms (0
// = benchmark; skip the rate floor). The rate floor is 50% of the theoretical
// 1000/rotate devices/sec, since passive scanning never captures every advert.
func Analyze(obs []Observation, window time.Duration, expectedRotateMs int) Result {
	r := Result{Window: window, Total: len(obs), IDHistogram: map[uint16]int{}}

	macs := map[[6]byte]struct{}{}
	var connectable, fastpair, excluded, findmy int
	for _, o := range obs {
		macs[o.MAC] = struct{}{}
		if o.HasMfg {
			r.IDHistogram[o.CompanyID]++
			// Microsoft (Swift Pair) still pops a pairing prompt on bystanders and
			// is never emitted. Apple presence beacons are allowed (they don't pop
			// dialogs) — but an Apple Find My frame triggers anti-stalking alerts.
			if o.CompanyID == decoy.CompanyMicrosoft {
				excluded++
			}
		}
		if o.Connectable {
			connectable++
		}
		if o.FastPair {
			fastpair++
		}
		if o.AppleFindMy {
			findmy++
		}
	}
	r.DistinctMACs = len(macs)
	r.IDSpread = len(r.IDHistogram)

	if excluded > 0 {
		r.Violations = append(r.Violations, fmt.Sprintf("%d advert(s) carried an excluded company ID (Microsoft Swift Pair)", excluded))
	}
	if findmy > 0 {
		r.Violations = append(r.Violations, fmt.Sprintf("%d advert(s) carried an Apple Find My (0x12) message (triggers anti-stalking alerts)", findmy))
	}
	if fastpair > 0 {
		r.Violations = append(r.Violations, fmt.Sprintf("%d advert(s) carried a Google Fast Pair service-data block", fastpair))
	}
	if connectable > 0 {
		r.Violations = append(r.Violations, fmt.Sprintf("%d advert(s) were connectable (decoys must be non-connectable)", connectable))
	}
	if r.IDSpread < 2 {
		r.Notes = append(r.Notes, "vendor-ID spread < 2 (expected a variety of company IDs)")
	}

	rateOK := true
	if expectedRotateMs > 0 && window > 0 {
		r.RateChecked = true
		r.ObservedRate = float64(r.DistinctMACs) / window.Seconds()
		r.RateFloor = 0.5 * (1000.0 / float64(expectedRotateMs))
		rateOK = r.ObservedRate >= r.RateFloor
		if !rateOK {
			r.Violations = append(r.Violations, fmt.Sprintf(
				"distinct-MAC rate %.1f/s below floor %.1f/s (50%% of theoretical %.1f/s)",
				r.ObservedRate, r.RateFloor, 1000.0/float64(expectedRotateMs)))
		}
	}

	r.Pass = len(r.Violations) == 0
	return r
}

// String renders a concise PASS/FAIL report.
func (r Result) String() string {
	var b strings.Builder
	verdict := "PASS"
	if !r.Pass {
		verdict = "FAIL"
	}
	fmt.Fprintf(&b, "parity %s — %d adverts, %d distinct MACs over %s, %d vendor IDs\n",
		verdict, r.Total, r.DistinctMACs, r.Window.Round(time.Millisecond), r.IDSpread)
	if r.RateChecked {
		fmt.Fprintf(&b, "  rate: %.1f distinct MACs/s (floor %.1f/s)\n", r.ObservedRate, r.RateFloor)
	}
	for _, id := range sortedIDs(r.IDHistogram) {
		fmt.Fprintf(&b, "  vendor %#04x: %d\n", id, r.IDHistogram[id])
	}
	for _, n := range r.Notes {
		fmt.Fprintf(&b, "  note: %s\n", n)
	}
	for _, v := range r.Violations {
		fmt.Fprintf(&b, "  VIOLATION: %s\n", v)
	}
	return b.String()
}

func sortedIDs(h map[uint16]int) []uint16 {
	ids := make([]uint16, 0, len(h))
	for id := range h {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}
