// Command hohd is a BLE privacy / anti-tracking decoy for Linux — a native
// port of an ESP32 BLE-decoy firmware concept, targeting the ClockworkPi
// uConsole (CM4). It fabricates a churning crowd of plausible, non-connectable
// fake BLE devices so real devices don't stand out to a scanner in a space you
// control.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/zachsis/helmofhades/internal/config"
	"github.com/zachsis/helmofhades/internal/dashboard"
	"github.com/zachsis/helmofhades/internal/decoy"
	"github.com/zachsis/helmofhades/internal/engine"
	"github.com/zachsis/helmofhades/internal/hci"
	"github.com/zachsis/helmofhades/internal/tune"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	cfg, err := config.Parse("hohd", os.Args[1:], os.Stderr)
	switch {
	case errors.Is(err, config.ErrVersion):
		fmt.Println("hohd", version)
		return
	case errors.Is(err, flag.ErrHelp):
		return // usage already printed by the flag package
	case err != nil:
		fmt.Fprintln(os.Stderr, "hohd:", err)
		os.Exit(2)
	}

	if !dashboard.ValidTheme(cfg.Theme) {
		fmt.Fprintf(os.Stderr, "hohd: unknown --theme %q (valid: %s)\n",
			cfg.Theme, strings.Join(dashboard.ThemeNames(), ", "))
		os.Exit(2)
	}

	if err := run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "hohd:", err)
		os.Exit(1)
	}
}

// run takes exclusive control of the controller, drives the decoy loop until a
// signal arrives, and guarantees the controller is handed back to bluetoothd on
// every exit path (normal, SIGINT/SIGTERM, or panic — the deferred Close runs
// during stack unwinding).
func run(cfg config.Config) error {
	log := newLogger(cfg.Verbose)

	conn, err := hci.New(cfg.HCIIndex)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := conn.Close(); cerr != nil {
			log.Warn("controller restore reported errors "+
				"(if Bluetooth misbehaves, run: sudo systemctl restart bluetooth)", "err", cerr)
		} else {
			log.Info("controller restored to bluetoothd")
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if cfg.Benchmark {
		log.Info("hohd starting", "version", version, "hci", cfg.HCIIndex)
		return runBenchmark(ctx, conn, cfg, log)
	}

	if cfg.Dense {
		derived, err := calibrateDense(ctx, conn, cfg, log)
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return nil
		}
		cfg = derived
	}

	return runLoop(ctx, conn, cfg, log)
}

// calibrateDense probes (~10s) for the fastest advertising interval the
// controller sustains and returns cfg with the derived visibility-optimal
// adv-ms / rotate-ms (dwell = adverts-per-id x adv-ms).
func calibrateDense(ctx context.Context, conn engine.Controller, cfg config.Config, log *slog.Logger) (config.Config, error) {
	candidates := tune.CalmCandidates
	if cfg.Aggressive {
		candidates = tune.AggressiveCandidates
	}
	per := (10 * time.Second) / time.Duration(len(candidates))
	log.Info("dense: calibrating", "candidates_ms", candidates, "aggressive", cfg.Aggressive)

	probes := engine.Calibrate(ctx, conn, cfg, candidates, per, log)
	if ctx.Err() != nil {
		return cfg, nil
	}
	rec, ok := tune.Recommend(probes, cfg.AdvertsPerID)
	if !ok {
		return cfg, fmt.Errorf("dense calibration produced no usable settings")
	}
	cfg.AdvMs = rec.AdvMs
	cfg.RotateMs = rec.RotateMs
	log.Info("dense: calibrated", "adv_ms", rec.AdvMs, "rotate_ms", rec.RotateMs,
		"visible_per_sec_est", fmt.Sprintf("%.0f", rec.VisiblePerSec))
	return cfg, nil
}

// runLoop advertises using either the live dashboard (TTY) or line logging.
func runLoop(ctx context.Context, conn engine.Controller, cfg config.Config, log *slog.Logger) error {
	mode := "paced"
	if cfg.Dense {
		mode = "dense"
	}

	if cfg.Dashboard {
		if dashboard.IsTerminal(os.Stdout.Fd()) {
			// A cancelable child context so an in-dashboard 'q' quits everything.
			runCtx, runCancel := context.WithCancel(ctx)
			defer runCancel()

			m := dashboard.New(mode, cfg.AdvMs, cfg.RotateMs)
			m.SetColor(dashboard.ColorEnabled())
			m.UseTheme(cfg.Theme)
			rate := engine.NewRateControl(cfg.RotateMs, cfg.AdvMs, 2000)
			learn := engine.NewLearnControl()
			apple := engine.NewAppleControl(appleKind(cfg))
			m.SetAppleMode(apple.Mode())
			trackers := engine.NewTrackerControl(cfg.Trackers)
			m.SetTrackers(trackers.Enabled())
			broadcast := engine.NewBroadcastControl()
			// The engine logs through the debug logger, which discards until the
			// 'D' key arms it — so the dashboard frame is never corrupted, but
			// debug capture to a file works on demand.
			debug, dlog := engine.NewDebugControl(cfg.LogFile)
			if cfg.Debug {
				if err := debug.Enable(); err != nil {
					log.Warn("could not open debug log", "err", err)
				}
			}
			m.SetDebug(debug.Enabled(), debug.Path())
			scanner, _ := conn.(engine.Scanner) // *hci.Conn scans; nil disables learn
			done := make(chan struct{})
			go func() {
				dashboard.Run(runCtx, m, os.Stdout, os.Stdout.Fd(), dashboard.Controls{
					Rate: rate, Learn: learn, Apple: apple, Trackers: trackers,
					Broadcast: broadcast, Debug: debug, OnQuit: runCancel,
				})
				close(done)
			}()
			err := engine.Run(runCtx, conn, cfg, dlog, m, rate, scanner, learn, apple, trackers, broadcast)
			runCancel()
			<-done // wait for the terminal to be restored
			return err
		}
		log.Warn("--dashboard: stdout is not a terminal; falling back to line logging")
	}

	logger := log
	if cfg.Debug || cfg.LogFile != "" {
		if f, path, err := engine.OpenDebugFile(cfg.LogFile); err == nil {
			logger = slog.New(slog.NewTextHandler(io.MultiWriter(os.Stdout, f),
				&slog.HandlerOptions{Level: slog.LevelDebug}))
			logger.Info("debug log", "path", path)
		} else {
			log.Warn("could not open debug log", "err", err)
		}
	}
	logger.Info("hohd starting", "version", version, "hci", cfg.HCIIndex, "mode", mode)
	apple := engine.NewAppleControl(appleKind(cfg))
	trackers := engine.NewTrackerControl(cfg.Trackers)
	return engine.Run(ctx, conn, cfg, logger, engine.LogReporter{Log: logger}, nil, nil, nil, apple, trackers, nil)
}

// appleKind resolves the validated cfg.AppleMode string to an engine AppleKind.
// Validation already ran in config.Validate, so an unknown value falls back to
// off rather than erroring here.
func appleKind(cfg config.Config) decoy.AppleKind {
	kind, _ := engine.ParseAppleMode(cfg.AppleMode)
	return kind
}

// runBenchmark probes advertising intervals for a bounded window and prints the
// recommended visibility-optimal settings, then returns (it does not advertise
// indefinitely).
func runBenchmark(ctx context.Context, conn engine.Controller, cfg config.Config, log *slog.Logger) error {
	dur := cfg.Duration
	if dur <= 0 {
		dur = 10 * time.Second
	}
	candidates := tune.AggressiveCandidates
	per := dur / time.Duration(len(candidates))
	log.Info("calibrating", "duration", dur.String(), "candidates_ms", candidates)

	probes := engine.Calibrate(ctx, conn, cfg, candidates, per, log)
	rec, ok := tune.Recommend(probes, 2)
	if !ok {
		log.Warn("no calibration data collected")
		return nil
	}
	log.Info("recommended settings", "flags", rec.String(),
		"note", "estimate is model-based (TX-side); confirm real capture with hoh-verify on a 2nd device")
	return nil
}

func newLogger(verbose bool) *slog.Logger {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}
