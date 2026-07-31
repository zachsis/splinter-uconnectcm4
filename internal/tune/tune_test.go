package tune

import "testing"

func TestRecommendPicksLowestCleanInterval(t *testing.T) {
	probes := []Probe{
		{AdvMs: 20, Cycles: 100, Fails: 5},  // not clean
		{AdvMs: 50, Cycles: 100, Fails: 0},  // clean, lowest clean
		{AdvMs: 100, Cycles: 100, Fails: 0}, // clean but higher
	}
	r, ok := Recommend(probes, 2)
	if !ok {
		t.Fatal("expected ok")
	}
	if r.AdvMs != 50 || r.RotateMs != 100 {
		t.Fatalf("got adv=%d rotate=%d, want 50/100", r.AdvMs, r.RotateMs)
	}
	if r.VisiblePerSec != 10 {
		t.Fatalf("visible = %.1f, want 10", r.VisiblePerSec)
	}
}

func TestRecommendAggressiveReachesFloor(t *testing.T) {
	probes := []Probe{
		{AdvMs: 20, Cycles: 100, Fails: 0},
		{AdvMs: 50, Cycles: 100, Fails: 0},
	}
	r, _ := Recommend(probes, 2)
	if r.AdvMs != 20 || r.RotateMs != 40 || r.VisiblePerSec != 25 {
		t.Fatalf("got %+v, want adv=20 rotate=40 visible=25", r)
	}
}

func TestRecommendFallsBackToLowestFailRate(t *testing.T) {
	probes := []Probe{
		{AdvMs: 20, Cycles: 100, Fails: 40},
		{AdvMs: 50, Cycles: 100, Fails: 10}, // best (lowest fail rate)
		{AdvMs: 100, Cycles: 100, Fails: 30},
	}
	r, _ := Recommend(probes, 2)
	if r.AdvMs != 50 {
		t.Fatalf("fallback chose adv=%d, want 50", r.AdvMs)
	}
}

func TestRecommendClampsAdvertsPerID(t *testing.T) {
	probes := []Probe{{AdvMs: 20, Cycles: 10, Fails: 0}}
	r, _ := Recommend(probes, 0) // clamps to 1
	if r.RotateMs != 20 {
		t.Fatalf("rotate = %d, want 20 (advertsPerID clamped to 1)", r.RotateMs)
	}
}

func TestRecommendEmpty(t *testing.T) {
	if _, ok := Recommend(nil, 2); ok {
		t.Fatal("expected ok=false for empty probes")
	}
}
