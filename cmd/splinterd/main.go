// Command splinterd is a BLE privacy / anti-tracking decoy for Linux — a native
// port of the splinter ESP32 firmware concept, targeting the ClockworkPi
// uConsole (CM4). It fabricates a churning crowd of plausible, non-connectable
// fake BLE devices so real devices don't stand out to a scanner in a space you
// control.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/zachsis/splinter-uconnectcm4/internal/config"
	"github.com/zachsis/splinter-uconnectcm4/internal/engine"
	"github.com/zachsis/splinter-uconnectcm4/internal/hci"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	cfg, err := config.Parse("splinterd", os.Args[1:], os.Stderr)
	switch {
	case errors.Is(err, config.ErrVersion):
		fmt.Println("splinterd", version)
		return
	case errors.Is(err, flag.ErrHelp):
		return // usage already printed by the flag package
	case err != nil:
		fmt.Fprintln(os.Stderr, "splinterd:", err)
		os.Exit(2)
	}

	if err := run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "splinterd:", err)
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

	log.Info("splinterd starting", "version", version, "hci", cfg.HCIIndex)
	return engine.Run(ctx, conn, cfg, log)
}

func newLogger(verbose bool) *slog.Logger {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}
