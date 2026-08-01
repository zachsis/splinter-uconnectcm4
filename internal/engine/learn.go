package engine

import (
	"fmt"
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
	mu        sync.Mutex
	requested bool
	learning  bool
	weights   []int
	summary   string
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

func (l *LearnControl) setResult(weights []int, summary string) {
	l.mu.Lock()
	l.weights = weights
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

// learnWeights builds decoy weights (parallel to decoy.Vendors) from observed
// advertisements: a baseline of 1 per vendor plus the observed count for its
// company ID, so vendors actually present dominate while all stay possible.
// Returns a short summary of the top in-table vendors observed.
func learnWeights(reports []hci.AdvReport) ([]int, string) {
	obs := map[uint16]int{}
	for _, r := range reports {
		if id, hasMfg, _, _ := verify.ParseAdvData(r.Data); hasMfg {
			obs[id]++
		}
	}
	weights := make([]int, len(decoy.Vendors))
	inTable := map[uint16]bool{}
	for i, v := range decoy.Vendors {
		weights[i] = 1 + obs[v.CompanyID]
		inTable[v.CompanyID] = true
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
	summary := "no matching vendors nearby"
	if len(parts) > 0 {
		summary = strings.Join(parts, " · ")
	}
	return weights, summary
}
