package decoy

import (
	"math/rand/v2"
	"testing"

	"github.com/zachsis/splinter-uconnectcm4/internal/config"
)

func newRNG(a, b uint64) *rand.Rand { return rand.New(rand.NewPCG(a, b)) }

// parseAD walks a raw advertising payload into (type -> data) structures,
// returning an error if the [len][type][data] framing is malformed.
func parseAD(t *testing.T, buf []byte) map[byte][]byte {
	t.Helper()
	out := map[byte][]byte{}
	for i := 0; i < len(buf); {
		l := int(buf[i])
		if l == 0 || i+1+l > len(buf) {
			t.Fatalf("malformed AD at %d (len=%d, total=%d): %x", i, l, len(buf), buf)
		}
		out[buf[i+1]] = buf[i+2 : i+1+l]
		i += 1 + l
	}
	return out
}

func TestRandomStaticAddrForm(t *testing.T) {
	rng := newRNG(1, 2)
	for i := 0; i < 100000; i++ {
		a := RandomStaticAddr(rng)
		if a[5]&0xC0 != 0xC0 {
			t.Fatalf("top two bits of MSB not set: %x", a)
		}
		ones := 0
		for _, b := range a[:5] {
			ones += popcount(b)
		}
		ones += popcount(a[5] & 0x3F)
		if ones == 0 || ones == 46 {
			t.Fatalf("degenerate random part not rejected: %x", a)
		}
	}
}

func popcount(b byte) int {
	n := 0
	for ; b != 0; b &= b - 1 {
		n++
	}
	return n
}

func TestBuildAdvDataStructure(t *testing.T) {
	cfg := config.Default()
	rng := newRNG(42, 7)
	for i := 0; i < 100000; i++ {
		buf := BuildAdvData(cfg, rng)
		if len(buf) > AdvMaxLen {
			t.Fatalf("payload exceeds %d bytes: %d (%x)", AdvMaxLen, len(buf), buf)
		}
		ads := parseAD(t, buf)

		// Flags always present and equal to 0x06.
		f, ok := ads[adFlags]
		if !ok || len(f) != 1 || f[0] != flagsValue {
			t.Fatalf("flags missing/wrong: %x", buf)
		}
		// Name, if present, is <=12 chars and belongs to a real vendor.
		if name, ok := ads[adName]; ok {
			if len(name) > 12 {
				t.Fatalf("name too long: %q", name)
			}
			if !vendorHasName(string(name)) {
				t.Fatalf("name not from vendor table: %q", name)
			}
		}
		// Manufacturer data, if present, starts with a known company ID (LE) and
		// never an excluded one.
		if mfg, ok := ads[adMfgData]; ok {
			if len(mfg) < 2 {
				t.Fatalf("mfg too short: %x", mfg)
			}
			id := uint16(mfg[0]) | uint16(mfg[1])<<8
			if id == CompanyApple || id == CompanyMicrosoft || id == ServiceGoogleFastPair {
				t.Fatalf("excluded company id emitted: %#04x", id)
			}
			if !vendorHasID(id) {
				t.Fatalf("company id not from vendor table: %#04x", id)
			}
		}
	}
}

func TestNoExcludedVendors(t *testing.T) {
	if len(Vendors) < 8 {
		t.Fatalf("vendor table too small for a meaningful crowd: %d", len(Vendors))
	}
	for _, v := range Vendors {
		if v.CompanyID == CompanyApple || v.CompanyID == CompanyMicrosoft || v.CompanyID == ServiceGoogleFastPair {
			t.Fatalf("excluded company id in table: %#04x", v.CompanyID)
		}
		if len(v.Name) > 12 {
			t.Fatalf("vendor name too long: %q", v.Name)
		}
	}
}

func TestBuildAdvDataDeterministic(t *testing.T) {
	cfg := config.Default()
	a := drain(cfg, newRNG(99, 100), 50)
	b := drain(cfg, newRNG(99, 100), 50)
	for i := range a {
		if string(a[i]) != string(b[i]) {
			t.Fatalf("non-deterministic at %d: %x vs %x", i, a[i], b[i])
		}
	}
}

func drain(cfg config.Config, rng *rand.Rand, n int) [][]byte {
	out := make([][]byte, n)
	for i := range out {
		out[i] = BuildAdvData(cfg, rng)
	}
	return out
}

func vendorHasName(name string) bool {
	for _, v := range Vendors {
		if v.Name == name {
			return true
		}
	}
	return false
}

func vendorHasID(id uint16) bool {
	for _, v := range Vendors {
		if v.CompanyID == id {
			return true
		}
	}
	return false
}
