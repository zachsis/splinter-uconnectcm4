package decoy

import "math/rand/v2"

// Service UUIDs for the item trackers hohd can impersonate. These devices
// advertise via Service Data (0x16) / Service UUID (0x03) rather than
// manufacturer-specific data, so a passive collector buckets them by UUID.
const (
	ServiceTile uint16 = 0xFEED // Tile item tracker
	// ServiceGoogleFastPair (0xFE2C) is declared in vendors.go.
)

// TrackerKind identifies a service-data tracker type.
type TrackerKind int

const (
	TrackerTile TrackerKind = iota
	TrackerFastPair
	trackerKinds // sentinel: count of tracker kinds
)

// pickTracker chooses a tracker kind weighted by `weights` ([Tile, FastPair],
// parallel to the TrackerKind order) when valid, else uniformly. Mirrors
// pickVendor so learn mode can tilt the Tile/Fast-Pair split toward what's seen.
func pickTracker(rng *rand.Rand, weights []int) TrackerKind {
	if len(weights) == int(trackerKinds) {
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
					return TrackerKind(i)
				}
				r -= w
			}
		}
	}
	return TrackerKind(rng.IntN(int(trackerKinds)))
}

// buildTracker serializes one service-data tracker decoy (Tile or Fast Pair),
// chosen per weights.
func buildTracker(rng *rand.Rand, weights []int) Decoy {
	if pickTracker(rng, weights) == TrackerFastPair {
		return buildFastPair(rng)
	}
	return buildTile(rng)
}

// buildTile builds a Tile decoy: Flags, a complete 16-bit Service UUID list
// advertising 0xFEED, Service Data for 0xFEED with a random tile-id-like body,
// and the local name "Tile" — the shape a scanner buckets as a Tile.
func buildTile(rng *rand.Rand) Decoy {
	buf := appendAD(make([]byte, 0, AdvMaxLen), adFlags, []byte{flagsValue})
	buf = appendAD(buf, adServiceUUID16, uuidLE(ServiceTile))

	sd := make([]byte, 2+8) // UUID + 8 random id bytes
	copy(sd, uuidLE(ServiceTile))
	for i := 2; i < len(sd); i++ {
		sd[i] = byte(rng.UintN(256))
	}
	buf = appendAD(buf, adServiceData16, sd)

	nb := []byte("Tile")
	if fits(buf, nb) {
		buf = appendAD(buf, adName, nb)
	}
	return Decoy{AD: buf, CompanyID: ServiceTile, Name: "Tile"}
}

// buildFastPair builds a Google Fast Pair *non-discoverable* decoy: Service Data
// for 0xFE2C beginning with a version/flags byte (0x00) followed by a random
// account-key filter. It carries NO 3-byte model ID, so — unlike a discoverable
// Fast Pair frame — it never pops a "tap to pair" sheet on bystanders' phones.
// The body length is always >= 5 so it can never be mistaken for the 3-byte
// discoverable model-ID frame.
func buildFastPair(rng *rand.Rand) Decoy {
	buf := appendAD(make([]byte, 0, AdvMaxLen), adFlags, []byte{flagsValue})

	// Layout after the UUID: [version/flags][account-key-filter field header][filter].
	// The field header is (length<<4)|type; type 0x2 is "hide UI", so even a
	// bystander who owns Fast Pair devices is never shown a subsequent-pairing
	// notification (type 0x0 "show UI" would risk one). Only the filter is random.
	filterLen := 4 + rng.IntN(4)      // 4..7 account-key-filter bytes
	sd := make([]byte, 2+2+filterLen) // UUID(2) + version(1) + field header(1) + filter
	copy(sd, uuidLE(ServiceGoogleFastPair))
	sd[2] = 0x00                     // version/flags: non-discoverable
	sd[3] = byte(filterLen<<4) | 0x2 // account-key filter: length nibble + hide-UI type
	for i := 4; i < len(sd); i++ {
		sd[i] = byte(rng.UintN(256))
	}
	buf = appendAD(buf, adServiceData16, sd)
	return Decoy{AD: buf, CompanyID: ServiceGoogleFastPair}
}

// uuidLE returns a 16-bit UUID as little-endian bytes (on-air order).
func uuidLE(u uint16) []byte { return []byte{byte(u), byte(u >> 8)} }
