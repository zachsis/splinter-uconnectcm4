package engine

import (
	"sync/atomic"
	"time"
)

// RateControl holds a live-adjustable paced rotation interval (ms), clamped to
// [minMs, maxMs]. It is safe for concurrent reads (the engine loop) and
// adjustments (the dashboard key handler).
type RateControl struct {
	ms    atomic.Int64
	minMs int64
	maxMs int64
}

// NewRateControl creates a control initialised to ms, clamped to [minMs, maxMs].
// minMs is the visibility floor (advertising interval) — the dwell must not drop
// below it or decoys rotate before they transmit.
func NewRateControl(ms, minMs, maxMs int) *RateControl {
	if minMs < 1 {
		minMs = 1
	}
	if maxMs < minMs {
		maxMs = minMs
	}
	rc := &RateControl{minMs: int64(minMs), maxMs: int64(maxMs)}
	rc.ms.Store(clampInt64(int64(ms), rc.minMs, rc.maxMs))
	return rc
}

// Millis returns the current rotation interval in milliseconds.
func (rc *RateControl) Millis() int { return int(rc.ms.Load()) }

// Duration returns the current rotation interval as a time.Duration.
func (rc *RateControl) Duration() time.Duration {
	return time.Duration(rc.ms.Load()) * time.Millisecond
}

// Adjust changes the interval by deltaMs (clamped) and returns the new value.
// A negative delta means a faster rate (shorter interval).
func (rc *RateControl) Adjust(deltaMs int) int {
	for {
		cur := rc.ms.Load()
		next := clampInt64(cur+int64(deltaMs), rc.minMs, rc.maxMs)
		if rc.ms.CompareAndSwap(cur, next) {
			return int(next)
		}
	}
}

func clampInt64(v, lo, hi int64) int64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
