package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
)

// ErrVersion is returned by Parse when --version was requested.
var ErrVersion = errors.New("version requested")

// Parse builds a Config from command-line args, starting from Default() and
// applying flags, then validating. It returns ErrVersion if --version was given
// and flag.ErrHelp if -h/--help was given (usage already printed to out).
func Parse(name string, args []string, out io.Writer) (Config, error) {
	cfg := Default()
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(out)

	fs.IntVar(&cfg.RotateMs, "rotate-ms", cfg.RotateMs, "paced-mode delay between decoys (ms)")
	fs.IntVar(&cfg.NameProb, "name-prob", cfg.NameProb, "percent chance a decoy advertises a name (0-100)")
	fs.IntVar(&cfg.MfgProb, "mfg-prob", cfg.MfgProb, "percent chance a decoy carries vendor mfg data (0-100)")
	fs.IntVar(&cfg.AdvMs, "adv-ms", cfg.AdvMs, "on-air advertising interval minimum in ms (max = this + 30)")
	fs.BoolVar(&cfg.Benchmark, "benchmark", cfg.Benchmark, "bounded self-calibrating benchmark: probe intervals and recommend optimal settings")
	fs.DurationVar(&cfg.Duration, "duration", cfg.Duration, "benchmark duration (e.g. 20s; 0 = 10s default)")
	fs.BoolVar(&cfg.Dense, "dense", cfg.Dense, "self-calibrating maximum-visibility mode (probe then run at optimal settings)")
	fs.BoolVar(&cfg.Aggressive, "aggressive", cfg.Aggressive, "in --dense, allow the lowest advertising interval (more visible, higher power)")
	fs.IntVar(&cfg.AdvertsPerID, "adverts-per-id", cfg.AdvertsPerID, "advertising events per identity before rotating (dense dwell multiplier)")
	fs.BoolVar(&cfg.Dashboard, "dashboard", cfg.Dashboard, "live mtr-style terminal dashboard instead of line logs (needs a TTY)")
	fs.StringVar(&cfg.Theme, "theme", cfg.Theme, "dashboard color theme: matrix|amber|neon|mono")
	fs.DurationVar(&cfg.LearnWindow, "learn-window", cfg.LearnWindow, "learn-mode passive scan window (dashboard 'l' key)")
	fs.IntVar(&cfg.HCIIndex, "hci", cfg.HCIIndex, "HCI device index to drive (hciX)")
	fs.BoolVar(&cfg.Verbose, "verbose", cfg.Verbose, "enable debug logging")
	showVersion := fs.Bool("version", false, "print version and exit")

	if err := fs.Parse(args); err != nil {
		return cfg, err // includes flag.ErrHelp
	}
	if *showVersion {
		return cfg, ErrVersion
	}
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// Validate checks the tunables against BLE and sanity limits. Validation is in
// milliseconds; the ms->0.625ms conversion happens later in the HCI layer.
func (c Config) Validate() error {
	if c.AdvMs < 20 {
		return fmt.Errorf("--adv-ms must be >= 20 (BLE minimum), got %d", c.AdvMs)
	}
	if c.NameProb < 0 || c.NameProb > 100 {
		return fmt.Errorf("--name-prob must be 0..100, got %d", c.NameProb)
	}
	if c.MfgProb < 0 || c.MfgProb > 100 {
		return fmt.Errorf("--mfg-prob must be 0..100, got %d", c.MfgProb)
	}
	if c.RotateMs < 0 {
		return fmt.Errorf("--rotate-ms must be >= 0, got %d", c.RotateMs)
	}
	if c.HCIIndex < 0 {
		return fmt.Errorf("--hci must be >= 0, got %d", c.HCIIndex)
	}
	if c.Dense && c.Benchmark {
		return fmt.Errorf("--dense and --benchmark are mutually exclusive")
	}
	if c.Dashboard && c.Benchmark {
		return fmt.Errorf("--dashboard is not supported with --benchmark (which is a bounded probe)")
	}
	if c.AdvertsPerID < 1 {
		return fmt.Errorf("--adverts-per-id must be >= 1, got %d", c.AdvertsPerID)
	}
	return nil
}
