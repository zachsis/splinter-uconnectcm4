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

	"github.com/zachsis/splinter-uconnectcm4/internal/config"
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
	if err := cycle(f, config.Default(), rand.New(rand.NewPCG(1, 2))); err != nil {
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
	if err := cycle(f, config.Default(), rand.New(rand.NewPCG(1, 2))); err == nil {
		t.Fatal("expected error")
	}
	// After the failing SetRandomAddr, no data/enable should have run.
	for _, c := range f.snapshot() {
		if c == "data" || c == "enable" {
			t.Fatalf("cycle continued past error: %v", f.snapshot())
		}
	}
}

func TestRunPacedStopsOnCtx(t *testing.T) {
	f := &fakeCtrl{}
	cfg := config.Default()
	cfg.RotateMs = 5 // fast paced mode (exercises the non-benchmark wait branch)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()

	if err := Run(ctx, f, cfg, quietLog()); err != nil {
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

	if err := Run(ctx, f, cfg, quietLog()); err != nil {
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
