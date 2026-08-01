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
	mu          sync.Mutex
	start       time.Time
	total       uint64
	failsCum    uint64
	rateHist    []int
	failHist    []int
	lastAddr    [6]byte
	lastName    string
	lastID      uint16
	haveLast    bool
	vendorHist  map[uint16]int
	mode        string
	advMs       int
	rotateMs    int
	themeIdx    int
	colorOn     bool
	learnActive bool
	learnLine   string
	appleMode   string
}

// SetColor enables or disables ANSI color (disabled => mono regardless of theme).
func (m *Model) SetColor(on bool) {
	m.mu.Lock()
	m.colorOn = on
	m.mu.Unlock()
}

// UseTheme selects a theme by name; returns false for an unknown name.
func (m *Model) UseTheme(name string) bool {
	for i, n := range themeOrder {
		if n == name {
			m.mu.Lock()
			m.themeIdx = i
			m.mu.Unlock()
			return true
		}
	}
	return false
}

// CycleTheme advances to the next theme (the `t` hotkey).
func (m *Model) CycleTheme() {
	m.mu.Lock()
	m.themeIdx = (m.themeIdx + 1) % len(themeOrder)
	m.mu.Unlock()
}

// effectiveTheme returns the active theme (mono when color is off). Caller holds mu.
func (m *Model) effectiveTheme() Theme {
	if !m.colorOn {
		return monoTheme
	}
	return themes[themeOrder[m.themeIdx]]
}

// RateAdjuster is the subset of engine.RateControl the dashboard needs to show
// and change the live rotation interval. *engine.RateControl satisfies it.
type RateAdjuster interface {
	Millis() int
	Adjust(deltaMs int) int
}

// LearnController is the subset of engine.LearnControl the dashboard needs to
// trigger and display learn mode. *engine.LearnControl satisfies it.
type LearnController interface {
	Request()
	Learning() bool
	Summary() string
}

// AppleController is the subset of engine.AppleControl the dashboard needs to
// show and cycle the Apple-decoy mode. *engine.AppleControl satisfies it.
type AppleController interface {
	Mode() string  // current mode label (off|naive|nearby-info)
	Cycle() string // advance to the next mode, returning its label
}

// SetAppleMode updates the displayed Apple-decoy mode (mirrored from the
// AppleController so the Snapshot carries it).
func (m *Model) SetAppleMode(mode string) {
	m.mu.Lock()
	m.appleMode = mode
	m.mu.Unlock()
}

// SetLearn updates the displayed learn-mode status (the render loop mirrors the
// LearnController here so the Snapshot carries it).
func (m *Model) SetLearn(active bool, summary string) {
	m.mu.Lock()
	m.learnActive = active
	m.learnLine = summary
	m.mu.Unlock()
}

// SetRate updates the displayed rotation interval (called when a hotkey changes it).
func (m *Model) SetRate(rotateMs int) {
	m.mu.Lock()
	m.rotateMs = rotateMs
	m.mu.Unlock()
}

// New returns a Model for the given run mode and advertising settings.
func New(mode string, advMs, rotateMs int) *Model {
	return &Model{
		start:      time.Now(),
		vendorHist: map[uint16]int{},
		mode:       mode,
		advMs:      advMs,
		rotateMs:   rotateMs,
		colorOn:    true, // default; main disables via SetColor when unsupported
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
	Uptime      time.Duration
	Total       uint64
	FailsCum    uint64
	RateHist    []int
	FailHist    []int
	LastAddr    [6]byte
	LastName    string
	LastID      uint16
	HaveLast    bool
	Vendors     []VendorCount
	Mode        string
	AdvMs       int
	RotateMs    int
	Theme       Theme
	LearnActive bool
	LearnLine   string
	AppleMode   string
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
		Uptime:      time.Since(m.start),
		Total:       m.total,
		FailsCum:    m.failsCum,
		RateHist:    append([]int(nil), m.rateHist...),
		FailHist:    append([]int(nil), m.failHist...),
		LastAddr:    m.lastAddr,
		LastName:    m.lastName,
		LastID:      m.lastID,
		HaveLast:    m.haveLast,
		Vendors:     vs,
		Mode:        m.mode,
		AdvMs:       m.advMs,
		RotateMs:    m.rotateMs,
		Theme:       m.effectiveTheme(),
		LearnActive: m.learnActive,
		LearnLine:   m.learnLine,
		AppleMode:   m.appleMode,
	}
}
