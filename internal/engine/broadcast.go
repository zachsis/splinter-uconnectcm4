package engine

import "sync/atomic"

// BroadcastControl is the live pause switch for advertising. The dashboard's
// Space hotkey toggles it; the engine loop reads it each iteration and, while
// paused, disables advertising and mints nothing — without releasing the
// controller back to bluetoothd. Safe for concurrent use.
type BroadcastControl struct{ paused atomic.Bool }

// NewBroadcastControl returns a control that starts running (not paused).
func NewBroadcastControl() *BroadcastControl { return &BroadcastControl{} }

// Paused reports whether broadcasting is currently paused.
func (c *BroadcastControl) Paused() bool { return c.paused.Load() }

// Toggle flips the paused state and returns the new value.
func (c *BroadcastControl) Toggle() bool {
	for {
		cur := c.paused.Load()
		if c.paused.CompareAndSwap(cur, !cur) {
			return !cur
		}
	}
}
