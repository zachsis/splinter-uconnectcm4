// Package dashboard renders a live, in-place-refreshing terminal view of
// splinterd's decoy activity (mtr-style). The Model is a thread-safe stats sink
// that satisfies engine.Reporter; the renderer and the terminal driver read
// immutable Snapshots so the hot path never blocks on I/O.
package dashboard

import (
	"sort"
	"sync"
	"time"
)

// histLen is how many per-second samples the sparklines retain.
const histLen = 60

// Model accumulates decoy activity. It implements engine.Reporter.
type Model struct {
	mu         sync.Mutex
	start      time.Time
	total      uint64
	failsCum   uint64
	rateHist   []int
	failHist   []int
	lastAddr   [6]byte
	lastName   string
	lastID     uint16
	haveLast   bool
	vendorHist map[uint16]int
	mode       string
	advMs      int
	rotateMs   int
}

// New returns a Model for the given run mode and advertising settings.
func New(mode string, advMs, rotateMs int) *Model {
	return &Model{
		start:      time.Now(),
		vendorHist: map[uint16]int{},
		mode:       mode,
		advMs:      advMs,
		rotateMs:   rotateMs,
	}
}

// Decoy records one minted decoy (engine.Reporter).
func (m *Model) Decoy(addr [6]byte, companyID uint16, name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.total++
	m.lastAddr, m.lastID, m.lastName, m.haveLast = addr, companyID, name, true
	m.vendorHist[companyID]++
}

// Rate records one per-second rate sample (engine.Reporter).
func (m *Model) Rate(devPerSec, fails int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failsCum += uint64(fails)
	m.rateHist = pushRing(m.rateHist, devPerSec)
	m.failHist = pushRing(m.failHist, fails)
}

func pushRing(s []int, v int) []int {
	s = append(s, v)
	if len(s) > histLen {
		s = s[len(s)-histLen:]
	}
	return s
}

// VendorCount is one bar of the crowd histogram.
type VendorCount struct {
	ID    uint16
	Count int
}

// Snapshot is an immutable view of the model for rendering.
type Snapshot struct {
	Uptime   time.Duration
	Total    uint64
	FailsCum uint64
	RateHist []int
	FailHist []int
	LastAddr [6]byte
	LastName string
	LastID   uint16
	HaveLast bool
	Vendors  []VendorCount
	Mode     string
	AdvMs    int
	RotateMs int
}

// Snapshot copies the current state under lock.
func (m *Model) Snapshot() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	vs := make([]VendorCount, 0, len(m.vendorHist))
	for id, c := range m.vendorHist {
		vs = append(vs, VendorCount{ID: id, Count: c})
	}
	sort.Slice(vs, func(i, j int) bool {
		if vs[i].Count != vs[j].Count {
			return vs[i].Count > vs[j].Count
		}
		return vs[i].ID < vs[j].ID
	})
	return Snapshot{
		Uptime:   time.Since(m.start),
		Total:    m.total,
		FailsCum: m.failsCum,
		RateHist: append([]int(nil), m.rateHist...),
		FailHist: append([]int(nil), m.failHist...),
		LastAddr: m.lastAddr,
		LastName: m.lastName,
		LastID:   m.lastID,
		HaveLast: m.haveLast,
		Vendors:  vs,
		Mode:     m.mode,
		AdvMs:    m.advMs,
		RotateMs: m.rotateMs,
	}
}
