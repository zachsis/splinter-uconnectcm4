// Package engine runs splinterd's decoy rotation loop: it continuously retires
// the current fake device and mints a new one, producing the churning crowd a
// scanner observes. It depends only on the small Controller interface, so the
// loop is unit-tested against a fake without any Bluetooth hardware.
package engine

import (
	"context"
	crand "crypto/rand"
	"encoding/binary"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/zachsis/splinter-uconnectcm4/internal/config"
	"github.com/zachsis/splinter-uconnectcm4/internal/decoy"
	"github.com/zachsis/splinter-uconnectcm4/internal/hci"
)

// Controller is the subset of the HCI transport the engine needs. *hci.Conn
// satisfies it.
type Controller interface {
	SetAdvEnable(on bool) error
	SetRandomAddr(a [6]byte) error
	SetAdvParams(minMs, maxMs uint16, advType byte) error
	SetAdvData(ad []byte) error
}

// Run drives the rotation loop until ctx is cancelled. Advertising parameters
// are set once up front; each cycle then disables advertising, mints a fresh
// random MAC + decoy payload, and re-enables. On return, advertising is disabled.
func Run(ctx context.Context, ctrl Controller, cfg config.Config, log *slog.Logger) error {
	rng := newRNG()

	min := uint16(cfg.AdvMs)
	max := uint16(cfg.AdvMs + 30)
	if err := ctrl.SetAdvParams(min, max, hci.AdvNonconnInd); err != nil {
		return fmt.Errorf("set adv params: %w", err)
	}
	defer func() { _ = ctrl.SetAdvEnable(false) }()

	if cfg.Benchmark {
		log.Info("splinter running", "mode", "benchmark/flood")
	} else {
		log.Info("splinter running", "mode", "paced", "rotate_ms", cfg.RotateMs)
	}

	var ok, fail uint64
	tick := time.NewTicker(time.Second)
	defer tick.Stop()

	for {
		if ctx.Err() != nil {
			return nil
		}
		if err := cycle(ctrl, cfg, rng); err != nil {
			fail++
			if cfg.Verbose {
				log.Debug("decoy cycle failed", "err", err)
			}
		} else {
			ok++
		}

		select {
		case <-tick.C:
			log.Info("rate", "devices_per_sec", ok, "fail", fail)
			ok, fail = 0, 0
		default:
		}

		if !cfg.Benchmark {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(time.Duration(cfg.RotateMs) * time.Millisecond):
			}
		}
	}
}

// cycle mints and starts one decoy. On any HCI error after the leading disable
// it returns immediately without running the remaining steps, bounding a
// wedged-controller stall to a single read timeout.
//
// The leading SetAdvEnable(false) is best-effort: its sole purpose is to permit
// the random-address change, and disabling advertising that is already disabled
// is rejected by some controllers (e.g. the CYW43455 returns "Command
// Disallowed"). A genuine failure to disable while advertising is active surfaces
// on the next command (SetRandomAddr is illegal while advertising).
func cycle(ctrl Controller, cfg config.Config, rng *rand.Rand) error {
	_ = ctrl.SetAdvEnable(false)
	addr := decoy.RandomStaticAddr(rng)
	if err := ctrl.SetRandomAddr(addr); err != nil {
		return err
	}
	ad := decoy.BuildAdvData(cfg, rng)
	if err := ctrl.SetAdvData(ad); err != nil {
		return err
	}
	return ctrl.SetAdvEnable(true)
}

// newRNG seeds math/rand/v2 from crypto/rand.
func newRNG() *rand.Rand {
	var b [16]byte
	_, _ = crand.Read(b[:])
	return rand.New(rand.NewPCG(
		binary.LittleEndian.Uint64(b[0:8]),
		binary.LittleEndian.Uint64(b[8:16]),
	))
}
