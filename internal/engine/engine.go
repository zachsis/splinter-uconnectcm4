// Package engine runs hohd's decoy rotation loop: it continuously retires
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

	"github.com/zachsis/helmofhades/internal/config"
	"github.com/zachsis/helmofhades/internal/decoy"
	"github.com/zachsis/helmofhades/internal/hci"
	"github.com/zachsis/helmofhades/internal/tune"
)

// Controller is the subset of the HCI transport the engine needs. *hci.Conn
// satisfies it.
type Controller interface {
	SetAdvEnable(on bool) error
	SetRandomAddr(a [6]byte) error
	SetAdvParams(minMs, maxMs uint16, advType byte) error
	SetAdvData(ad []byte) error
}

// Reporter receives decoy activity so it can be rendered (line log, dashboard…)
// without coupling the engine to any output format.
type Reporter interface {
	Decoy(addr [6]byte, companyID uint16, name string) // per decoy minted
	Rate(devPerSec, fails int)                         // once per second
}

// LogReporter is the default Reporter: it logs the per-second rate via slog,
// matching hohd's original line output. Individual decoys are too frequent
// to log, so Decoy is a no-op.
type LogReporter struct{ Log *slog.Logger }

func (r LogReporter) Decoy(addr [6]byte, companyID uint16, name string) {}
func (r LogReporter) Rate(devPerSec, fails int) {
	r.Log.Info("rate", "devices_per_sec", devPerSec, "fail", fails)
}

// Run drives the rotation loop until ctx is cancelled. Advertising parameters
// are set once up front; each cycle then disables advertising, mints a fresh
// random MAC + decoy payload, and re-enables. On return, advertising is disabled.
func Run(ctx context.Context, ctrl Controller, cfg config.Config, log *slog.Logger, rep Reporter, rate *RateControl, scanner Scanner, learn *LearnControl, apple *AppleControl, trackers *TrackerControl, broadcast *BroadcastControl) error {
	rng := newRNG()

	if rate == nil {
		rate = NewRateControl(cfg.RotateMs, cfg.AdvMs, 2000)
	}
	learnWindow := cfg.LearnWindow
	if learnWindow <= 0 {
		learnWindow = 15 * time.Second
	}

	min := uint16(cfg.AdvMs)
	max := uint16(cfg.AdvMs + 30)
	if err := ctrl.SetAdvParams(min, max, hci.AdvNonconnInd); err != nil {
		return fmt.Errorf("set adv params: %w", err)
	}
	// Guardrail: an identity that rotates before it advertises is never seen.
	if cfg.RotateMs < cfg.AdvMs {
		log.Warn("rotate-ms < adv-ms: decoys rotate before transmitting and won't be visible to scanners",
			"rotate_ms", cfg.RotateMs, "adv_ms", cfg.AdvMs)
	}
	defer func() { _ = ctrl.SetAdvEnable(false) }()

	if cfg.Benchmark {
		log.Info("hohd running", "mode", "benchmark/flood")
	} else {
		log.Info("hohd running", "mode", "paced", "rotate_ms", cfg.RotateMs)
	}

	var ok, fail uint64
	tick := time.NewTicker(time.Second)
	defer tick.Stop()

	for {
		if ctx.Err() != nil {
			return nil
		}

		// Learn mode: on request, pause advertising, scan the ambient vendor mix,
		// and reweight decoy selection toward it.
		if learn != nil && scanner != nil && learn.takeRequest() {
			learn.setLearning(true)
			_ = ctrl.SetAdvEnable(false)
			reports, err := scanner.Scan(learnWindow)
			if err != nil {
				// Don't silently blip: surface the failure and keep whatever
				// weighting was already in effect rather than resetting it.
				learn.setSummary("scan failed: " + err.Error())
				log.Warn("learn: scan failed", "err", err)
			} else {
				w, tw, summary := learnWeights(reports)
				learn.setResult(w, tw, summary)
				learn.setDevices(buildLearnedDevices(reports))
				log.Info("learn: reweighted decoys", "observed", summary)
			}
			learn.setLearning(false)
		}

		// Pause switch (dashboard Space): stop minting and disable advertising,
		// but keep the controller — resuming continues without a re-takeover.
		if broadcast != nil && broadcast.Paused() {
			_ = ctrl.SetAdvEnable(false)
		} else {
			opts := decoy.Options{AppleShare: cfg.AppleShare, TrackerShare: cfg.TrackerShare}
			if apple != nil {
				opts.Apple = apple.Kind()
			}
			if trackers != nil {
				opts.Trackers = trackers.Enabled()
			}
			if learn != nil {
				opts.Weights = learn.weightsSnapshot()
				opts.TrackerWeights = learn.trackerWeightsSnapshot()
			}
			addr, d, err := cycle(ctrl, cfg, rng, opts)
			if err != nil {
				fail++
				if cfg.Verbose {
					log.Debug("decoy cycle failed", "err", err)
				}
			} else {
				ok++
				rep.Decoy(addr, d.CompanyID, d.Name)
				log.Debug("decoy", "mac", macString(addr), "id", d.CompanyID, "name", d.Name)
			}
		}

		select {
		case <-tick.C:
			rep.Rate(int(ok), int(fail))
			ok, fail = 0, 0
		default:
		}

		if !cfg.Benchmark {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(rate.Duration()):
			}
		}
	}
}

// Calibrate probes each candidate advertising interval for perCandidate,
// cycling decoys at max rate and counting controller rejections. It measures the
// fastest interval the controller sustains reliably, feeding tune.Recommend.
// Advertising is left disabled on return.
func Calibrate(ctx context.Context, ctrl Controller, cfg config.Config, candidates []int, perCandidate time.Duration, log *slog.Logger) []tune.Probe {
	rng := newRNG()
	probes := make([]tune.Probe, 0, len(candidates))
	for _, advMs := range candidates {
		if ctx.Err() != nil {
			break
		}
		// A rejected interval (e.g. below the controller's floor) is unusable.
		if err := ctrl.SetAdvParams(uint16(advMs), uint16(advMs+30), hci.AdvNonconnInd); err != nil {
			probes = append(probes, tune.Probe{AdvMs: advMs, Cycles: 0, Fails: 1})
			log.Info("probe", "adv_ms", advMs, "rejected", true, "err", err)
			continue
		}
		var ok, fail int
		deadline := time.Now().Add(perCandidate)
		for time.Now().Before(deadline) && ctx.Err() == nil {
			if _, _, err := cycle(ctrl, cfg, rng, decoy.Options{}); err != nil {
				fail++
			} else {
				ok++
			}
		}
		p := tune.Probe{AdvMs: advMs, Cycles: ok + fail, Fails: fail}
		probes = append(probes, p)
		log.Info("probe", "adv_ms", advMs, "cycles", p.Cycles, "fails", p.Fails,
			"fail_rate", fmt.Sprintf("%.1f%%", p.FailRate()*100))
	}
	_ = ctrl.SetAdvEnable(false)
	return probes
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
func cycle(ctrl Controller, cfg config.Config, rng *rand.Rand, opts decoy.Options) ([6]byte, decoy.Decoy, error) {
	_ = ctrl.SetAdvEnable(false)
	addr := decoy.RandomStaticAddr(rng)
	if err := ctrl.SetRandomAddr(addr); err != nil {
		return addr, decoy.Decoy{}, err
	}
	d := decoy.BuildWithOpts(cfg, rng, opts)
	if err := ctrl.SetAdvData(d.AD); err != nil {
		return addr, decoy.Decoy{}, err
	}
	if err := ctrl.SetAdvEnable(true); err != nil {
		return addr, decoy.Decoy{}, err
	}
	return addr, d, nil
}

// macString formats a BLE address for debug logging.
func macString(a [6]byte) string {
	return fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X", a[0], a[1], a[2], a[3], a[4], a[5])
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
