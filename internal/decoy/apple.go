package decoy

import "math/rand/v2"

// AppleKind selects how (or whether) a decoy impersonates an Apple device.
type AppleKind int32

const (
	AppleOff      AppleKind = iota // never emit Apple decoys
	AppleNaive                     // company 0x004C + random body (defeats company-ID bucketing)
	AppleNearform                  // well-formed Continuity Nearby Info (looks like a real iPhone)
)

// String returns a short label for status display.
func (k AppleKind) String() string {
	switch k {
	case AppleNaive:
		return "naive"
	case AppleNearform:
		return "nearby-info"
	default:
		return "off"
	}
}

// Continuity message types carried inside Apple manufacturer data.
const (
	// appleNearbyInfoType is the presence beacon every iPhone broadcasts
	// constantly. It does NOT pop any dialog on bystanders.
	appleNearbyInfoType = 0x10
	// AppleFindMyType is the "offline finding" (Find My / AirTag) message type.
	// hohd must NEVER emit it: a moving, persistent unknown Find My beacon
	// triggers "unknown tracker near you" anti-stalking alerts. Exported so the
	// verify harness can assert we never emit it.
	AppleFindMyType = 0x12
)

// buildApple serializes an Apple decoy: Flags + Apple manufacturer data. The
// manufacturer data always carries the Apple company ID (0x004C); its body is
// either random (naive) or a well-formed Nearby Info message (nearform). Apple
// decoys are nameless — real Continuity presence beacons carry no local name.
func buildApple(rng *rand.Rand, kind AppleKind) Decoy {
	buf := appendAD(make([]byte, 0, AdvMaxLen), adFlags, []byte{flagsValue})

	var body []byte
	if kind == AppleNearform {
		body = appleNearbyInfo(rng)
	} else { // AppleNaive
		body = appleNaiveBody(rng)
	}

	mfg := make([]byte, 2+len(body))
	mfg[0] = byte(CompanyApple)      // 0x4C, little-endian company ID
	mfg[1] = byte(CompanyApple >> 8) // 0x00
	copy(mfg[2:], body)
	buf = appendAD(buf, adMfgData, mfg)
	return Decoy{AD: buf, CompanyID: CompanyApple}
}

// appleNaiveBody builds a low-effort Apple payload: a single benign Nearby Info
// (0x10) message wrapping random bytes. It must NOT be raw random bytes — a
// Continuity parser reads the first byte as a message TYPE, so random data could
// form a Find My (0x12, anti-stalking alerts) or AirPods-pairing (0x07, popup)
// message on bystanders' phones. Fixing the type to 0x10 (presence only) with a
// length that consumes the whole payload guarantees no forbidden message type is
// ever parsed, while still carrying the Apple company ID that defeats company-ID
// bucketing.
func appleNaiveBody(rng *rand.Rand) []byte {
	n := 1 + rng.IntN(4) // 1..4 random payload bytes
	body := make([]byte, 2+n)
	body[0] = appleNearbyInfoType // 0x10 — never pops UI
	body[1] = byte(n)
	for i := 2; i < len(body); i++ {
		body[i] = byte(rng.UintN(256))
	}
	return body
}

// appleNearbyInfo builds a plausible-but-random Continuity Nearby Info message:
// [type 0x10][len 0x05][status][flags][3 random bytes]. Status/flags and the
// trailing bytes are randomized so decoys don't share a constant fingerprint.
// It is never a Find My (0x12) message.
func appleNearbyInfo(rng *rand.Rand) []byte {
	return []byte{
		appleNearbyInfoType,
		0x05,                  // 5 payload bytes follow
		byte(rng.UintN(0x20)), // status: activity/state nibble
		byte(rng.UintN(256)),  // info flags
		byte(rng.UintN(256)),  // auth/data
		byte(rng.UintN(256)),  // auth/data
		byte(rng.UintN(256)),  // auth/data
	}
}
