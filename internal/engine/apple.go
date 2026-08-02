package engine

import (
	"sync/atomic"

	"github.com/zachsis/splinter-uconnectcm4/internal/decoy"
)

// AppleControl holds the live-adjustable Apple-decoy mode. The dashboard 'a'
// hotkey cycles it; the engine loop reads it each iteration. Safe for
// concurrent use.
type AppleControl struct{ kind atomic.Int32 }

// NewAppleControl creates a control initialized to kind.
func NewAppleControl(kind decoy.AppleKind) *AppleControl {
	c := &AppleControl{}
	c.kind.Store(int32(kind))
	return c
}

// Kind returns the current Apple mode (read by the engine loop).
func (c *AppleControl) Kind() decoy.AppleKind { return decoy.AppleKind(c.kind.Load()) }

// Mode returns the current mode's label (for dashboard display).
func (c *AppleControl) Mode() string { return c.Kind().String() }

// Cycle advances off -> naive -> nearby-info -> off and returns the new label.
func (c *AppleControl) Cycle() string {
	for {
		cur := c.kind.Load()
		next := (cur + 1) % 3
		if c.kind.CompareAndSwap(cur, next) {
			return decoy.AppleKind(next).String()
		}
	}
}

// ParseAppleMode maps a config string to an AppleKind. ok is false for an
// unrecognized value (the caller validates and reports).
func ParseAppleMode(s string) (kind decoy.AppleKind, ok bool) {
	switch s {
	case "off":
		return decoy.AppleOff, true
	case "naive":
		return decoy.AppleNaive, true
	case "nearform", "nearby-info":
		return decoy.AppleNearform, true
	}
	return decoy.AppleOff, false
}
