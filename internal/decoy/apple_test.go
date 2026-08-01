package decoy

import (
	"math/rand/v2"
	"testing"

	"github.com/zachsis/splinter-uconnectcm4/internal/config"
)

// mfgBody walks an AD payload and returns the manufacturer-specific data body
// (the bytes after the 0xFF type), or nil if none is present.
func mfgBody(ad []byte) []byte {
	for i := 0; i < len(ad); {
		l := int(ad[i])
		if l == 0 || i+1+l > len(ad) {
			break
		}
		if ad[i+1] == adMfgData {
			return ad[i+2 : i+1+l]
		}
		i += 1 + l
	}
	return nil
}

func TestAppleKindString(t *testing.T) {
	cases := map[AppleKind]string{AppleOff: "off", AppleNaive: "naive", AppleNearform: "nearby-info"}
	for k, want := range cases {
		if got := k.String(); got != want {
			t.Errorf("AppleKind(%d).String() = %q, want %q", k, got, want)
		}
	}
}

func TestBuildAppleNaive(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	for i := 0; i < 1000; i++ {
		d := buildApple(rng, AppleNaive)
		if d.CompanyID != CompanyApple {
			t.Fatalf("company = %#04x, want Apple", d.CompanyID)
		}
		if d.Name != "" {
			t.Fatalf("Apple decoys must be nameless, got %q", d.Name)
		}
		body := mfgBody(d.AD)
		if len(body) < 2 || body[0] != 0x4C || body[1] != 0x00 {
			t.Fatalf("mfg body missing little-endian Apple id: % x", body)
		}
		if len(d.AD) > AdvMaxLen {
			t.Fatalf("AD exceeds budget: %d", len(d.AD))
		}
	}
}

func TestBuildAppleNearformIsNearbyInfoNeverFindMy(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4))
	for i := 0; i < 1000; i++ {
		d := buildApple(rng, AppleNearform)
		body := mfgBody(d.AD)
		if len(body) < 3 || body[0] != 0x4C || body[1] != 0x00 {
			t.Fatalf("mfg body malformed: % x", body)
		}
		cont := body[2:] // Continuity messages after the company ID
		if cont[0] != appleNearbyInfoType {
			t.Fatalf("first Continuity msg = %#02x, want Nearby Info %#02x", cont[0], appleNearbyInfoType)
		}
		// Must never carry a Find My (0x12) message.
		for j := 0; j+1 < len(cont); {
			if cont[j] == AppleFindMyType {
				t.Fatalf("emitted forbidden Find My message: % x", cont)
			}
			j += 2 + int(cont[j+1])
		}
		if len(d.AD) > AdvMaxLen {
			t.Fatalf("AD exceeds budget: %d", len(d.AD))
		}
	}
}

// appleContinuityTypes returns the Continuity message types a parser would walk
// out of an Apple manufacturer body (the bytes after the 0x004C company ID),
// mirroring how a bystander's phone reads [type][len][payload...] runs.
func appleContinuityTypes(mfg []byte) []byte {
	if len(mfg) < 2 {
		return nil
	}
	cont := mfg[2:]
	var types []byte
	for i := 0; i < len(cont); {
		types = append(types, cont[i])
		if i+1 >= len(cont) {
			break
		}
		i += 2 + int(cont[i+1])
	}
	return types
}

// TestNaiveAppleNeverEmitsUITriggeringType is the regression guard for the
// security review's HIGH finding: naive Apple bodies used to be raw random
// bytes, so ~1/256 parsed as a Find My (0x12) message and could trigger
// anti-stalking alerts (0x07 would pop AirPods pairing). A large RNG sweep must
// never produce either forbidden message type.
func TestNaiveAppleNeverEmitsUITriggeringType(t *testing.T) {
	rng := rand.New(rand.NewPCG(0xBADC0DE, 0xF00D))
	const forbiddenFindMy, forbiddenAirPods = AppleFindMyType, 0x07
	for i := 0; i < 200000; i++ {
		d := buildApple(rng, AppleNaive)
		for _, typ := range appleContinuityTypes(mfgBody(d.AD)) {
			if typ == forbiddenFindMy || typ == forbiddenAirPods {
				t.Fatalf("naive Apple body emitted forbidden Continuity type %#02x: % x", typ, d.AD)
			}
		}
	}
}

func TestBuildWithOptsAppleShare(t *testing.T) {
	cfg := config.Default()
	rng := rand.New(rand.NewPCG(7, 8))

	// Share 100 with Apple on => every decoy is Apple.
	for i := 0; i < 500; i++ {
		if d := BuildWithOpts(cfg, rng, Options{Apple: AppleNaive, AppleShare: 100}); d.CompanyID != CompanyApple {
			t.Fatalf("share 100 should always emit Apple, got %#04x", d.CompanyID)
		}
	}
	// Apple off => never Apple, regardless of share.
	for i := 0; i < 500; i++ {
		if d := BuildWithOpts(cfg, rng, Options{Apple: AppleOff, AppleShare: 100}); d.CompanyID == CompanyApple {
			t.Fatalf("Apple off should never emit Apple")
		}
	}
	// Share 0 => never Apple even when enabled.
	for i := 0; i < 500; i++ {
		if d := BuildWithOpts(cfg, rng, Options{Apple: AppleNaive, AppleShare: 0}); d.CompanyID == CompanyApple {
			t.Fatalf("share 0 should never emit Apple")
		}
	}
}
