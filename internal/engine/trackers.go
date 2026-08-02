package engine

import "sync/atomic"

// TrackerControl holds the live-adjustable opt-in for service-data tracker
// decoys (Tile / Fast Pair). The dashboard 's' hotkey toggles it; the engine
// loop reads it each iteration. Safe for concurrent use.
type TrackerControl struct{ on atomic.Bool }

// NewTrackerControl creates a control initialized to on.
func NewTrackerControl(on bool) *TrackerControl {
	c := &TrackerControl{}
	c.on.Store(on)
	return c
}

// Enabled reports whether tracker decoys are currently being emitted.
func (c *TrackerControl) Enabled() bool { return c.on.Load() }

// Toggle flips the enabled state and returns the new value.
func (c *TrackerControl) Toggle() bool {
	for {
		cur := c.on.Load()
		if c.on.CompareAndSwap(cur, !cur) {
			return !cur
		}
	}
}
