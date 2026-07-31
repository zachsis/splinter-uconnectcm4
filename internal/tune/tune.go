// Package tune derives visibility-optimal advertising settings from a
// calibration probe. A BLE decoy is only observable if its identity stays
// on-air long enough to transmit at least one advertising PDU, i.e. the dwell
// (rotate interval) must be >= the advertising interval. This package computes
// the fastest still-visible settings the controller actually sustains.
package tune

import "fmt"

// Probe is the result of exercising one advertising interval: how many decoy
// cycles completed and how many the controller rejected.
type Probe struct {
	AdvMs  int
	Cycles int
	Fails  int
}

// FailRate returns the fraction of cycles the controller rejected (0..1).
func (p Probe) FailRate() float64 {
	if p.Cycles == 0 {
		return 1
	}
	return float64(p.Fails) / float64(p.Cycles)
}

// Recommendation is the derived visibility-optimal configuration.
type Recommendation struct {
	AdvMs         int     // advertising interval to use
	RotateMs      int     // dwell per identity = AdvertsPerID * AdvMs
	VisiblePerSec float64 // estimated distinct decoys/sec a scanner can observe
}

// Candidate advertising intervals (ms). Calm keeps a battery/RF-friendly floor;
// Aggressive reaches the 20ms BLE minimum.
var (
	AggressiveCandidates = []int{20, 30, 50, 100}
	CalmCandidates       = []int{50, 75, 100}
)

// Recommend chooses the lowest advertising interval the controller sustained
// with ZERO failures (so decoys reliably transmit), then sets the dwell to
// advertsPerID advertising events per identity. If nothing was clean it falls
// back to the interval with the lowest failure rate. advertsPerID is clamped to
// >= 1. Returns ok=false only when probes is empty.
func Recommend(probes []Probe, advertsPerID int) (Recommendation, bool) {
	if len(probes) == 0 {
		return Recommendation{}, false
	}
	if advertsPerID < 1 {
		advertsPerID = 1
	}

	best := -1 // index of chosen probe
	for i, p := range probes {
		if p.Fails == 0 {
			if best == -1 || p.AdvMs < probes[best].AdvMs {
				best = i
			}
		}
	}
	if best == -1 {
		// No clean interval — pick the lowest failure rate, tie-break on lower AdvMs.
		best = 0
		for i, p := range probes {
			if p.FailRate() < probes[best].FailRate() ||
				(p.FailRate() == probes[best].FailRate() && p.AdvMs < probes[best].AdvMs) {
				best = i
			}
		}
	}

	adv := probes[best].AdvMs
	rotate := adv * advertsPerID
	return Recommendation{
		AdvMs:         adv,
		RotateMs:      rotate,
		VisiblePerSec: 1000.0 / float64(rotate),
	}, true
}

// String renders the recommendation as a copy-pasteable flag suggestion.
func (r Recommendation) String() string {
	return fmt.Sprintf("--adv-ms %d --rotate-ms %d (~%.0f visible decoys/sec, est.)",
		r.AdvMs, r.RotateMs, r.VisiblePerSec)
}
