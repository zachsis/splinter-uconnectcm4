package engine

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/zachsis/splinter-uconnectcm4/internal/decoy"
	"github.com/zachsis/splinter-uconnectcm4/internal/hci"
	"github.com/zachsis/splinter-uconnectcm4/internal/verify"
)

// Scanner performs a passive LE scan for the given window. *hci.Conn satisfies it.
type Scanner interface {
	Scan(window time.Duration) ([]hci.AdvReport, error)
}

// LearnControl coordinates "learn mode" between the dashboard (which requests a
// scan and reads status) and the engine loop (which performs the scan and
// reweights decoy selection toward the observed vendor mix). Safe for concurrent
// use.
type LearnControl struct {
	mu             sync.Mutex
	requested      bool
	learning       bool
	weights        []int
	trackerWeights []int // [Tile, FastPair], parallel to decoy.TrackerKind
	summary        string
}

// NewLearnControl returns an idle learn control.
func NewLearnControl() *LearnControl { return &LearnControl{} }

// Request asks the engine to run a learning scan on its next loop iteration.
func (l *LearnControl) Request() {
	l.mu.Lock()
	l.requested = true
	l.mu.Unlock()
}

func (l *LearnControl) takeRequest() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	r := l.requested
	l.requested = false
	return r
}

func (l *LearnControl) setLearning(v bool) {
	l.mu.Lock()
	l.learning = v
	l.mu.Unlock()
}

func (l *LearnControl) setResult(weights, trackerWeights []int, summary string) {
	l.mu.Lock()
	l.weights = weights
	l.trackerWeights = trackerWeights
	l.summary = summary
	l.mu.Unlock()
}

// setSummary updates only the status line, leaving the current weights in place
// (used when a scan fails so the previous learning isn't discarded).
func (l *LearnControl) setSummary(summary string) {
	l.mu.Lock()
	l.summary = summary
	l.mu.Unlock()
}

func (l *LearnControl) weightsSnapshot() []int {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.weights == nil {
		return nil
	}
	return append([]int(nil), l.weights...)
}

func (l *LearnControl) trackerWeightsSnapshot() []int {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.trackerWeights == nil {
		return nil
	}
	return append([]int(nil), l.trackerWeights...)
}

// Learning reports whether a learning scan is currently running.
func (l *LearnControl) Learning() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.learning
}

// Summary describes the last learned vendor mix (empty until the first scan).
func (l *LearnControl) Summary() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.summary
}

// Learn-mode weighting. The goal is to *tilt* decoy selection toward the
// observed vendor mix without collapsing the crowd onto whatever happens to be
// loudest nearby — a cluster of identical decoys is more conspicuous than a
// diverse one. So every vendor keeps a solid floor (learnBaseWeight) and each
// observed vendor earns a dampened, capped boost (learnBoost).
const (
	// learnBaseWeight is the floor weight every vendor keeps, so the decoy crowd
	// stays diverse even when only one vendor is observed nearby.
	learnBaseWeight = 2
	// learnMaxBoost caps how much any single observed vendor can be favored. With
	// the base above and a 26-vendor table this bounds any one vendor to ~17% of
	// decoys even if it is the only thing seen — a lopsided RF environment (e.g.
	// only Samsung nearby) can't collapse the crowd into one telltale vendor.
	learnMaxBoost = 8
)

// learnBoost maps an observed advertisement count to a bounded weight bonus.
// It is sqrt-dampened so a single fast-beaconing device (many adverts/sec)
// doesn't outweigh several steadier ones, and hard-capped at learnMaxBoost.
func learnBoost(obs int) int {
	if obs <= 0 {
		return 0
	}
	b := int(math.Round(2 * math.Sqrt(float64(obs))))
	if b > learnMaxBoost {
		b = learnMaxBoost
	}
	if b < 1 {
		b = 1
	}
	return b
}

// learnWeights builds decoy weights (parallel to decoy.Vendors) from observed
// advertisements: a floor per vendor plus a dampened, capped boost for the
// company IDs actually seen nearby, so present vendors are favored while the
// crowd stays diverse. It also builds tracker weights ([Tile, FastPair]) from
// observed service-data trackers, using the same floor+boost. Returns the vendor
// weights, the tracker weights, and a short summary of what was seen.
func learnWeights(reports []hci.AdvReport) (vendorWeights, trackerWeights []int, summary string) {
	obs := map[uint16]int{}
	var tileN, fastpairN int
	for _, r := range reports {
		info := verify.ParseAdvData(r.Data)
		if info.HasMfg {
			obs[info.CompanyID]++
		}
		if info.Tile {
			tileN++
		}
		if info.FastPair {
			fastpairN++
		}
	}
	weights := make([]int, len(decoy.Vendors))
	inTable := map[uint16]bool{}
	for i, v := range decoy.Vendors {
		weights[i] = learnBaseWeight + learnBoost(obs[v.CompanyID])
		inTable[v.CompanyID] = true
	}
	trackerWeights = []int{
		decoy.TrackerTile:     learnBaseWeight + learnBoost(tileN),
		decoy.TrackerFastPair: learnBaseWeight + learnBoost(fastpairN),
	}

	type vc struct {
		id uint16
		n  int
	}
	var top []vc
	for id, n := range obs {
		if inTable[id] {
			top = append(top, vc{id, n})
		}
	}
	sort.Slice(top, func(i, j int) bool { return top[i].n > top[j].n })

	var parts []string
	for i, v := range top {
		if i >= 4 {
			break
		}
		parts = append(parts, fmt.Sprintf("%s %d", decoy.CompanyLabel(v.id), v.n))
	}
	if tileN > 0 {
		parts = append(parts, fmt.Sprintf("Tile %d", tileN))
	}
	if fastpairN > 0 {
		parts = append(parts, fmt.Sprintf("Fast Pair %d", fastpairN))
	}

	// Distinguish "scan saw manufacturer adverts but none we can impersonate"
	// (e.g. an all-Apple room) from "scan saw nothing" — otherwise a working
	// scan in an Apple-heavy environment looks like a failure.
	totalMfg := 0
	for _, n := range obs {
		totalMfg += n
	}
	summary = "no manufacturer adverts seen"
	switch {
	case len(parts) > 0:
		summary = strings.Join(parts, " · ")
	case totalMfg > 0:
		summary = fmt.Sprintf("%d adverts, none impersonable — crowd stays diverse", totalMfg)
	}
	return weights, trackerWeights, summary
}
