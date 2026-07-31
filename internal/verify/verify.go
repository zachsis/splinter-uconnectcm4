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
}

// ParseAdvData walks a raw advertising payload and extracts the fields the
// harness cares about. Malformed structures are skipped rather than erroring —
// the scanner sees real-world traffic, not just splinterd's.
func ParseAdvData(data []byte) (companyID uint16, hasMfg bool, name string, fastPair bool) {
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
	var connectable, fastpair, excluded int
	for _, o := range obs {
		macs[o.MAC] = struct{}{}
		if o.HasMfg {
			r.IDHistogram[o.CompanyID]++
			if o.CompanyID == decoy.CompanyApple || o.CompanyID == decoy.CompanyMicrosoft {
				excluded++
			}
		}
		if o.Connectable {
			connectable++
		}
		if o.FastPair {
			fastpair++
		}
	}
	r.DistinctMACs = len(macs)
	r.IDSpread = len(r.IDHistogram)

	if excluded > 0 {
		r.Violations = append(r.Violations, fmt.Sprintf("%d advert(s) carried an excluded company ID (Apple/Microsoft)", excluded))
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
