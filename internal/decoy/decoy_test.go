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

func TestBuildWeighted(t *testing.T) {
	cfg := config.Default()
	rng := newRNG(11, 22)
	// Heavily weight index 2; expect its company ID to dominate.
	weights := make([]int, len(Vendors))
	weights[2] = 100
	counts := map[uint16]int{}
	for i := 0; i < 1000; i++ {
		counts[BuildWeighted(cfg, rng, weights).CompanyID]++
	}
	if counts[Vendors[2].CompanyID] < 900 {
		t.Fatalf("weighted pick not biased toward index 2 (%#04x): %v", Vendors[2].CompanyID, counts)
	}
	// nil weights => uniform => a spread of company IDs.
	distinct := map[uint16]bool{}
	for i := 0; i < 2000; i++ {
		distinct[BuildWeighted(cfg, rng, nil).CompanyID] = true
	}
	if len(distinct) < 5 {
		t.Fatalf("uniform should spread across vendors, got %d distinct", len(distinct))
	}
}

func TestNoExcludedVendors(t *testing.T) {
	if len(Vendors) < 25 {
		t.Fatalf("vendor table too small for a diverse crowd: %d (want >= 25)", len(Vendors))
	}
	ids := map[uint16]struct{}{}
	named, nameless := 0, 0
	for _, v := range Vendors {
		if v.CompanyID == CompanyApple || v.CompanyID == CompanyMicrosoft || v.CompanyID == ServiceGoogleFastPair {
			t.Fatalf("excluded company id in table: %#04x", v.CompanyID)
		}
		if len(v.Name) > 12 {
			t.Fatalf("vendor name too long: %q", v.Name)
		}
		ids[v.CompanyID] = struct{}{}
		if v.Name == "" {
			nameless++
		} else {
			named++
		}
	}
	if len(ids) < 6 {
		t.Fatalf("too few distinct company IDs for diversity: %d (want >= 6)", len(ids))
	}
	if named == 0 || nameless == 0 {
		t.Fatalf("want a mix of named and nameless entries, got named=%d nameless=%d", named, nameless)
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
