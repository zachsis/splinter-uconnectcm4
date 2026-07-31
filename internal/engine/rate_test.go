package engine

import "testing"

func TestRateControlClampAndAdjust(t *testing.T) {
	rc := NewRateControl(250, 100, 2000)
	if rc.Millis() != 250 {
		t.Fatalf("initial = %d, want 250", rc.Millis())
	}
	// Faster (negative delta) down to the floor.
	rc.Adjust(-100)                         // 150
	if got := rc.Adjust(-100); got != 100 { // clamps at min 100
		t.Fatalf("floor clamp = %d, want 100", got)
	}
	// Slower up to the ceiling.
	for i := 0; i < 100; i++ {
		rc.Adjust(50)
	}
	if got := rc.Millis(); got != 2000 {
		t.Fatalf("ceiling clamp = %d, want 2000", got)
	}
}

func TestNewRateControlClampsInitial(t *testing.T) {
	// Initial below floor is clamped up to the floor.
	if rc := NewRateControl(20, 100, 2000); rc.Millis() != 100 {
		t.Fatalf("initial-below-floor = %d, want 100", rc.Millis())
	}
	// Duration reflects millis.
	rc := NewRateControl(200, 100, 2000)
	if rc.Duration().Milliseconds() != 200 {
		t.Fatalf("duration = %v, want 200ms", rc.Duration())
	}
}
