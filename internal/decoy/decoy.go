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
	adFlags   = 0x01 // Flags
	adName    = 0x09 // Complete Local Name
	adMfgData = 0xFF // Manufacturer Specific Data
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

// BuildAdvData returns a <=31-byte legacy advertising payload for a single
// decoy: Flags, an optional vendor name, and optional vendor manufacturer data.
// One vendor is chosen per call; both the name and the company ID come from that
// same vendor. The payload is never shaped like Apple/Microsoft/Google formats.
func BuildAdvData(cfg config.Config, rng *rand.Rand) []byte {
	v := Vendors[rng.IntN(len(Vendors))]

	buf := make([]byte, 0, AdvMaxLen)
	buf = appendAD(buf, adFlags, []byte{flagsValue}) // always present

	usedName := false
	if v.Name != "" && rng.IntN(100) < cfg.NameProb {
		name := []byte(v.Name)
		if fits(buf, name) {
			buf = appendAD(buf, adName, name)
			usedName = true
		}
	}

	if rng.IntN(100) < cfg.MfgProb {
		bodyLen := 3
		if !usedName {
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
	return buf
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
