// Package config holds splinterd's runtime tunables. The struct is the single
// source of truth consumed by the decoy builder, the rotation engine, and the
// CLI, and its defaults mirror the ESP32 firmware's compile-time macros.
package config

// Config holds all runtime tunables.
type Config struct {
	RotateMs  int  // paced-mode delay between decoys, ms (firmware SPLINTER_ROTATE_MS)
	NameProb  int  // % chance a decoy advertises a name (firmware SPLINTER_NAME_PROB)
	MfgProb   int  // % chance a decoy carries vendor mfg data (firmware SPLINTER_MFG_PROB)
	AdvMs     int  // on-air advertising interval MIN, ms; max = AdvMs+30 (firmware SPLINTER_ADV_MS)
	Benchmark bool // max-rate flood vs paced (firmware SPLINTER_BENCHMARK)
	HCIIndex  int  // hciX device index to drive
	Verbose   bool // debug logging
}

// Default returns the firmware-matching defaults.
func Default() Config {
	return Config{
		RotateMs:  250,
		NameProb:  40,
		MfgProb:   90,
		AdvMs:     100,
		Benchmark: false,
		HCIIndex:  0,
		Verbose:   false,
	}
}
