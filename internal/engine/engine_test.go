package engine

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math/rand/v2"
	"sync"
	"testing"
	"time"

	"github.com/zachsis/helmofhades/internal/config"
	"github.com/zachsis/helmofhades/internal/decoy"
)

// fakeCtrl records the sequence of controller calls and can inject an error on a
// chosen method.
type fakeCtrl struct {
	mu       sync.Mutex
	calls    []string
	params   int
	failOn   string
	failWith error
}

func (f *fakeCtrl) rec(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, name)
	if name == f.failOn {
		return f.failWith
	}
	return nil
}
func (f *fakeCtrl) SetAdvEnable(on bool) error {
	if on {
		return f.rec("enable")
	}
	return f.rec("disable")
}
func (f *fakeCtrl) SetRandomAddr(a [6]byte) error { return f.rec("addr") }
func (f *fakeCtrl) SetAdvData(ad []byte) error    { return f.rec("data") }
func (f *fakeCtrl) SetAdvParams(min, max uint16, t byte) error {
	f.mu.Lock()
	f.params++
	f.mu.Unlock()
	return f.rec("params")
}
func (f *fakeCtrl) snapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestCycleOrder(t *testing.T) {
	f := &fakeCtrl{}
	if _, _, err := cycle(f, config.Default(), rand.New(rand.NewPCG(1, 2)), decoy.Options{}); err != nil {
		t.Fatal(err)
	}
	want := []string{"disable", "addr", "data", "enable"}
	got := f.snapshot()
	if len(got) != len(want) {
		t.Fatalf("calls = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("call %d = %q, want %q (%v)", i, got[i], want[i], got)
		}
	}
}

func TestCycleAbortsOnError(t *testing.T) {
	f := &fakeCtrl{failOn: "addr", failWith: errors.New("boom")}
	if _, _, err := cycle(f, config.Default(), rand.New(rand.NewPCG(1, 2)), decoy.Options{}); err == nil {
		t.Fatal("expected error")
	}
	// After the failing SetRandomAddr, no data/enable should have run.
	for _, c := range f.snapshot() {
		if c == "data" || c == "enable" {
			t.Fatalf("cycle continued past error: %v", f.snapshot())
		}
	}
}

func TestCalibrate(t *testing.T) {
	f := &fakeCtrl{}
	probes := Calibrate(context.Background(), f, config.Default(),
		[]int{20, 50}, 10*time.Millisecond, quietLog())
	if len(probes) != 2 {
		t.Fatalf("want 2 probes, got %d", len(probes))
	}
	if probes[0].AdvMs != 20 || probes[1].AdvMs != 50 {
		t.Fatalf("adv-ms mismatch: %+v", probes)
	}
	for _, p := range probes {
		if p.Cycles == 0 {
			t.Fatalf("probe %d ran no cycles", p.AdvMs)
		}
		if p.Fails != 0 {
			t.Fatalf("clean fake should have 0 fails: %+v", p)
		}
	}
	// One SetAdvParams per candidate, plus the params call inside each cycle... none:
	// cycle() does not set params, so params calls == number of candidates.
	if f.params != 2 {
		t.Fatalf("SetAdvParams called %d times, want 2 (one per candidate)", f.params)
	}
}

func TestRunPacedStopsOnCtx(t *testing.T) {
	f := &fakeCtrl{}
	cfg := config.Default()
	cfg.RotateMs = 5 // fast paced mode (exercises the non-benchmark wait branch)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()

	if err := Run(ctx, f, cfg, quietLog(), LogReporter{quietLog()}, nil, nil, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	calls := f.snapshot()
	if len(calls) == 0 || calls[len(calls)-1] != "disable" {
		t.Fatalf("expected deferred disable last, got %v", calls)
	}
}

func TestRunSetsParamsOnceAndDisablesOnExit(t *testing.T) {
	f := &fakeCtrl{}
	cfg := config.Default()
	cfg.Benchmark = true // no per-cycle sleep
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()

	if err := Run(ctx, f, cfg, quietLog(), LogReporter{quietLog()}, nil, nil, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if f.params != 1 {
		t.Fatalf("SetAdvParams called %d times, want 1", f.params)
	}
	calls := f.snapshot()
	if len(calls) == 0 || calls[len(calls)-1] != "disable" {
		t.Fatalf("last call = %q, want deferred disable", calls[len(calls)-1])
	}
	// Params must be the very first call.
	if calls[0] != "params" {
		t.Fatalf("first call = %q, want params", calls[0])
	}
}
