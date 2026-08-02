// Package config holds splinterd's runtime tunables. The struct is the single
// source of truth consumed by the decoy builder, the rotation engine, and the
// CLI, and its defaults mirror the ESP32 firmware's compile-time macros.
package config

import "time"

// Config holds all runtime tunables.
type Config struct {
	RotateMs     int           // paced-mode delay between decoys, ms (firmware SPLINTER_ROTATE_MS)
	NameProb     int           // % chance a decoy advertises a name (firmware SPLINTER_NAME_PROB)
	MfgProb      int           // % chance a decoy carries vendor mfg data (firmware SPLINTER_MFG_PROB)
	AdvMs        int           // on-air advertising interval MIN, ms; max = AdvMs+30 (firmware SPLINTER_ADV_MS)
	Benchmark    bool          // bounded self-calibrating benchmark (was: max-rate flood)
	Duration     time.Duration // benchmark run time; 0 = mode default (10s)
	Dense        bool          // self-calibrating maximum-visibility mode
	Aggressive   bool          // in --dense, allow the lowest advertising interval (higher power)
	AdvertsPerID int           // advertising events per identity before rotating (dense dwell multiplier)
	Dashboard    bool          // live mtr-style terminal dashboard instead of line logs
	Theme        string        // dashboard color theme name
	LearnWindow  time.Duration // learn-mode passive scan window
	AppleMode    string        // Apple decoy mode: off|naive|nearform (initial; dashboard 'a' cycles live)
	AppleShare   int           // % of decoys that impersonate Apple when AppleMode != off
	Trackers     bool          // emit service-data trackers (Tile/Fast Pair); dashboard 's' toggles live
	TrackerShare int           // % of decoys that are service-data trackers when Trackers is on
	HCIIndex     int           // hciX device index to drive
	Verbose      bool          // debug logging
}

// Default returns the firmware-matching defaults.
func Default() Config {
	return Config{
		RotateMs:     250,
		NameProb:     40,
		MfgProb:      90,
		AdvMs:        100,
		Benchmark:    false,
		AdvertsPerID: 2,
		Theme:        "matrix",
		LearnWindow:  15 * time.Second,
		AppleMode:    "naive",
		AppleShare:   15,
		Trackers:     false,
		TrackerShare: 20,
		HCIIndex:     0,
		Verbose:      false,
	}
}
