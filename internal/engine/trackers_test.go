package engine

import "testing"

func TestTrackerControlToggle(t *testing.T) {
	c := NewTrackerControl(false)
	if c.Enabled() {
		t.Fatalf("initial Enabled() = true, want false")
	}
	if got := c.Toggle(); !got || !c.Enabled() {
		t.Fatalf("first Toggle() = %v, Enabled() = %v, want true/true", got, c.Enabled())
	}
	if got := c.Toggle(); got || c.Enabled() {
		t.Fatalf("second Toggle() = %v, Enabled() = %v, want false/false", got, c.Enabled())
	}
}

func TestNewTrackerControlInitialOn(t *testing.T) {
	if c := NewTrackerControl(true); !c.Enabled() {
		t.Fatalf("initial-on Enabled() = false, want true")
	}
}
