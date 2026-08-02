// Package dashboard renders a live, in-place-refreshing terminal view of
// splinterd's decoy activity (mtr-style). The Model is a thread-safe stats sink
// that satisfies engine.Reporter; the renderer and the terminal driver read
// immutable Snapshots so the hot path never blocks on I/O.
package dashboard

import (
	"sort"
	"sync"
	"time"

	"github.com/zachsis/splinter-uconnectcm4/internal/verify"
)

// FocusTarget selects which table the scroll keys act on.
type FocusTarget int

const (
	FocusCrowd FocusTarget = iota
	FocusLearned
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
	trackersOn  bool
	paused      bool
	helpOpen    bool
	debugOn     bool
	debugPath   string
	learned     []verify.Observation
	focus       FocusTarget
	crowdScroll int
	learnScroll int
	crowdMax    int // max scroll offset for the crowd table (updated each frame)
	learnMax    int // max scroll offset for the learned table
	viewport    int // rows shown per table this frame (for page scrolling)
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
	Devices() []verify.Observation
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

// TrackerController is the subset of engine.TrackerControl the dashboard needs
// to show and toggle service-data tracker decoys. *engine.TrackerControl
// satisfies it.
type TrackerController interface {
	Enabled() bool
	Toggle() bool
}

// SetTrackers updates the displayed tracker-decoy state (mirrored from the
// TrackerController so the Snapshot carries it).
func (m *Model) SetTrackers(on bool) {
	m.mu.Lock()
	m.trackersOn = on
	m.mu.Unlock()
}

// Controls bundles the live controls the terminal driver reads and toggles, so
// the Run signature doesn't grow unbounded. Any field may be nil.
type Controls struct {
	Rate      RateAdjuster
	Learn     LearnController
	Apple     AppleController
	Trackers  TrackerController
	Broadcast BroadcastController
	Debug     DebugController
	OnQuit    func()
}

// BroadcastController is the subset of engine.BroadcastControl the dashboard
// needs to show and toggle the pause state. *engine.BroadcastControl satisfies it.
type BroadcastController interface {
	Paused() bool
	Toggle() bool
}

// DebugController is the subset of engine.DebugControl the dashboard needs to
// show and toggle debug file logging. *engine.DebugControl satisfies it.
type DebugController interface {
	Enabled() bool
	Toggle() bool
	Path() string
}

// SetPaused mirrors the broadcast pause state.
func (m *Model) SetPaused(p bool) { m.mu.Lock(); m.paused = p; m.mu.Unlock() }

// SetDebug mirrors the debug-logging state and current file path.
func (m *Model) SetDebug(on bool, path string) {
	m.mu.Lock()
	m.debugOn, m.debugPath = on, path
	m.mu.Unlock()
}

// SetLearned stores the devices seen in the last learn scan.
func (m *Model) SetLearned(d []verify.Observation) {
	m.mu.Lock()
	m.learned = d
	m.mu.Unlock()
}

// ToggleHelp shows/hides the help overlay (the `?` key).
func (m *Model) ToggleHelp() { m.mu.Lock(); m.helpOpen = !m.helpOpen; m.mu.Unlock() }

// HelpOpen reports whether the help overlay is showing.
func (m *Model) HelpOpen() bool { m.mu.Lock(); defer m.mu.Unlock(); return m.helpOpen }

// CycleFocus moves scroll focus to the next table (the Tab key).
func (m *Model) CycleFocus() {
	m.mu.Lock()
	m.focus = (m.focus + 1) % 2
	m.mu.Unlock()
}

// clampScrollLocked bounds both offsets to their content. Caller holds mu.
func (m *Model) clampScrollLocked() {
	if m.crowdScroll > m.crowdMax {
		m.crowdScroll = m.crowdMax
	}
	if m.crowdScroll < 0 {
		m.crowdScroll = 0
	}
	if m.learnScroll > m.learnMax {
		m.learnScroll = m.learnMax
	}
	if m.learnScroll < 0 {
		m.learnScroll = 0
	}
}

// SetScrollBounds recomputes each table's max scroll offset from its content and
// the given per-table viewport row count, then re-clamps the offsets. The render
// loop calls it each frame with dashboard.TableRows(height).
func (m *Model) SetScrollBounds(viewportRows int) {
	m.mu.Lock()
	m.viewport = viewportRows
	if m.crowdMax = len(m.vendorHist) - viewportRows; m.crowdMax < 0 {
		m.crowdMax = 0
	}
	if m.learnMax = len(m.learned) - viewportRows; m.learnMax < 0 {
		m.learnMax = 0
	}
	m.clampScrollLocked()
	m.mu.Unlock()
}

// ScrollPage moves the focused table by ~one page (dir -1 = up, +1 = down).
func (m *Model) ScrollPage(dir int) {
	m.mu.Lock()
	step := m.viewport - 1
	if step < 1 {
		step = 1
	}
	if m.focus == FocusLearned {
		m.learnScroll += dir * step
	} else {
		m.crowdScroll += dir * step
	}
	m.clampScrollLocked()
	m.mu.Unlock()
}

// Scroll moves the focused table by delta rows (negative = up), clamped.
func (m *Model) Scroll(delta int) {
	m.mu.Lock()
	if m.focus == FocusLearned {
		m.learnScroll += delta
	} else {
		m.crowdScroll += delta
	}
	m.clampScrollLocked()
	m.mu.Unlock()
}

// ScrollToEnd jumps the focused table to top (start=true) or bottom.
func (m *Model) ScrollToEnd(top bool) {
	m.mu.Lock()
	if m.focus == FocusLearned {
		if top {
			m.learnScroll = 0
		} else {
			m.learnScroll = m.learnMax
		}
	} else {
		if top {
			m.crowdScroll = 0
		} else {
			m.crowdScroll = m.crowdMax
		}
	}
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
	TrackersOn  bool
	Paused      bool
	HelpOpen    bool
	DebugOn     bool
	DebugPath   string
	Learned     []verify.Observation
	Focus       FocusTarget
	CrowdScroll int
	LearnScroll int
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
		TrackersOn:  m.trackersOn,
		Paused:      m.paused,
		HelpOpen:    m.helpOpen,
		DebugOn:     m.debugOn,
		DebugPath:   m.debugPath,
		Learned:     append([]verify.Observation(nil), m.learned...),
		Focus:       m.focus,
		CrowdScroll: m.crowdScroll,
		LearnScroll: m.learnScroll,
	}
}
