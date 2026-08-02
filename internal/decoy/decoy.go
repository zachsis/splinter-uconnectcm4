// Package decoy builds fake BLE identities: a random-static MAC and a spec-valid,
// non-connectable advertising payload attributing the device to a plausible
// vendor. It reproduces the behavior of the splinter firmware's decoy generation
// with no HCI/socket dependencies, so it is deterministically testable.
package decoy

import (
	"math/bits"
	"math/rand/v2"

	"github.com/zachsis/splinter-uconnectcm4/internal/config"
)

// Advertising Data (AD) types, from the Bluetooth assigned-numbers registry.
const (
	adFlags         = 0x01 // Flags
	adServiceUUID16 = 0x03 // Complete list of 16-bit Service Class UUIDs
	adName          = 0x09 // Complete Local Name
	adServiceData16 = 0x16 // Service Data - 16-bit UUID
	adMfgData       = 0xFF // Manufacturer Specific Data
)

// flagsValue = LE General Discoverable | BR/EDR Not Supported.
const flagsValue = 0x06

// AdvMaxLen is the legacy advertising payload budget.
const AdvMaxLen = 31

// RandomStaticAddr returns a BLE random-static address: 6 bytes whose most-
// significant octet has its top two bits set. The 46 random bits are never all
// zero or all one (regenerated in that astronomically rare case).
func RandomStaticAddr(rng *rand.Rand) [6]byte {
	for {
		var a [6]byte
		for i := range a {
			a[i] = byte(rng.UintN(256))
		}
		a[5] |= 0xC0

		ones := bits.OnesCount8(a[5] & 0x3F)
		for i := 0; i < 5; i++ {
			ones += bits.OnesCount8(a[i])
		}
		if ones != 0 && ones != 46 {
			return a
		}
	}
}

// Decoy is a generated fake identity: the serialized advertising payload plus
// the vendor it impersonates (surfaced for status/dashboard reporting).
type Decoy struct {
	AD []byte // <=31-byte legacy advertising payload
	// CompanyID is the display/grouping ID surfaced to Reporter.Decoy and the
	// dashboard's vendor histogram. For manufacturer-data decoys it is the
	// vendor's Bluetooth SIG company ID; for service-data trackers (buildTile,
	// buildFastPair) it is instead the tracker's service UUID (ServiceTile /
	// ServiceGoogleFastPair) — a different numeric namespace, reused here only
	// because CompanyLabel knows how to render both.
	CompanyID uint16
	Name      string // the advertised name, or "" if none was emitted
}

// pickVendor returns a vendor index chosen by the given weights (parallel to
// Vendors) when they are valid, otherwise uniformly at random.
func pickVendor(rng *rand.Rand, weights []int) int {
	if len(weights) == len(Vendors) {
		total := 0
		for _, w := range weights {
			if w > 0 {
				total += w
			}
		}
		if total > 0 {
			r := rng.IntN(total)
			for i, w := range weights {
				if w <= 0 {
					continue
				}
				if r < w {
					return i
				}
				r -= w
			}
		}
	}
	return rng.IntN(len(Vendors))
}

// Build generates one decoy with a uniformly-chosen vendor.
func Build(cfg config.Config, rng *rand.Rand) Decoy {
	return buildFor(cfg, rng, Vendors[rng.IntN(len(Vendors))])
}

// BuildWeighted generates one decoy with the vendor chosen by `weights` (parallel
// to Vendors; nil/invalid => uniform). Used by learn mode to blend into the
// observed vendor mix.
func BuildWeighted(cfg config.Config, rng *rand.Rand, weights []int) Decoy {
	return BuildWithOpts(cfg, rng, Options{Weights: weights})
}

// Options selects which payload kinds a decoy may take. The zero value emits
// only manufacturer-data vendor decoys (the original behavior).
type Options struct {
	Weights        []int     // vendor weighting (learn mode); nil/invalid => uniform
	Apple          AppleKind // Apple impersonation mode
	AppleShare     int       // % chance an eligible decoy impersonates Apple (Apple != off)
	Trackers       bool      // emit service-data trackers (Tile / Fast Pair) into the mix
	TrackerShare   int       // % chance an eligible decoy is a service-data tracker
	TrackerWeights []int     // [Tile, FastPair] learn weighting; nil/invalid => uniform
}

// BuildWithOpts generates one decoy, choosing its payload kind per opts: an
// Apple decoy with probability AppleShare, else a service-data tracker with
// probability TrackerShare, else a weighted manufacturer-data vendor decoy.
func BuildWithOpts(cfg config.Config, rng *rand.Rand, opts Options) Decoy {
	if opts.Apple != AppleOff && opts.AppleShare > 0 && rng.IntN(100) < opts.AppleShare {
		return buildApple(rng, opts.Apple)
	}
	if opts.Trackers && opts.TrackerShare > 0 && rng.IntN(100) < opts.TrackerShare {
		return buildTracker(rng, opts.TrackerWeights)
	}
	return buildFor(cfg, rng, Vendors[pickVendor(rng, opts.Weights)])
}

// buildFor serializes one decoy for a chosen vendor: Flags, an optional vendor
// name, and optional vendor manufacturer data. The payload is never shaped like
// Apple/Microsoft/Google formats.
func buildFor(cfg config.Config, rng *rand.Rand, v Vendor) Decoy {
	buf := make([]byte, 0, AdvMaxLen)
	buf = appendAD(buf, adFlags, []byte{flagsValue}) // always present

	name := ""
	if v.Name != "" && rng.IntN(100) < cfg.NameProb {
		nb := []byte(v.Name)
		if fits(buf, nb) {
			buf = appendAD(buf, adName, nb)
			name = v.Name
		}
	}

	if rng.IntN(100) < cfg.MfgProb {
		bodyLen := 3
		if name == "" {
			bodyLen = 3 + rng.IntN(5) // 3..7
		}
		mfg := make([]byte, 2+bodyLen)
		mfg[0] = byte(v.CompanyID) // little-endian company ID
		mfg[1] = byte(v.CompanyID >> 8)
		for i := 2; i < len(mfg); i++ {
			mfg[i] = byte(rng.UintN(256))
		}
		if fits(buf, mfg) {
			buf = appendAD(buf, adMfgData, mfg)
		}
	}
	return Decoy{AD: buf, CompanyID: v.CompanyID, Name: name}
}

// BuildAdvData returns just the advertising payload for a single decoy.
func BuildAdvData(cfg config.Config, rng *rand.Rand) []byte {
	return Build(cfg, rng).AD
}

// fits reports whether an AD structure carrying data (1 length byte + 1 type
// byte + data) still fits within the 31-byte budget.
func fits(buf, data []byte) bool {
	return len(buf)+2+len(data) <= AdvMaxLen
}

// appendAD appends one AD structure: [length][type][data...].
func appendAD(buf []byte, adType byte, data []byte) []byte {
	buf = append(buf, byte(1+len(data)), adType)
	return append(buf, data...)
}
