package decoy

import (
	"math/rand/v2"
	"testing"

	"github.com/zachsis/helmofhades/internal/config"
)

// adBody walks an AD payload and returns the body of the first structure of the
// given AD type, or nil.
func adBody(ad []byte, adType byte) []byte {
	for i := 0; i < len(ad); {
		l := int(ad[i])
		if l == 0 || i+1+l > len(ad) {
			break
		}
		if ad[i+1] == adType {
			return ad[i+2 : i+1+l]
		}
		i += 1 + l
	}
	return nil
}

func TestBuildTile(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	for i := 0; i < 1000; i++ {
		d := buildTile(rng)
		if d.CompanyID != ServiceTile || d.Name != "Tile" {
			t.Fatalf("tile decoy id/name = %#04x/%q", d.CompanyID, d.Name)
		}
		if uuids := adBody(d.AD, adServiceUUID16); len(uuids) < 2 || uuids[0] != 0xED || uuids[1] != 0xFE {
			t.Fatalf("tile service UUID list wrong: % x", uuids)
		}
		if sd := adBody(d.AD, adServiceData16); len(sd) < 2 || sd[0] != 0xED || sd[1] != 0xFE {
			t.Fatalf("tile service data UUID wrong: % x", sd)
		}
		if len(d.AD) > AdvMaxLen {
			t.Fatalf("AD exceeds budget: %d", len(d.AD))
		}
	}
}

// TestBuildFastPairNonDiscoverable is the security-critical guard: our Fast Pair
// decoys must be non-discoverable (no 3-byte model ID), so they never pop a
// pairing sheet on bystanders. The service-data body must lead with the 0x00
// version/flags byte and never be exactly 3 bytes long.
func TestBuildFastPairNonDiscoverable(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4))
	for i := 0; i < 2000; i++ {
		d := buildFastPair(rng)
		if d.CompanyID != ServiceGoogleFastPair {
			t.Fatalf("fast pair id = %#04x", d.CompanyID)
		}
		sd := adBody(d.AD, adServiceData16)
		if len(sd) < 2 || sd[0] != 0x2C || sd[1] != 0xFE {
			t.Fatalf("fast pair service data UUID wrong: % x", sd)
		}
		body := sd[2:] // payload after the UUID
		if len(body) == 3 {
			t.Fatalf("fast pair body is 3 bytes — reads as a discoverable model ID: % x", body)
		}
		if len(body) < 2 || body[0] != 0x00 {
			t.Fatalf("fast pair body must lead with 0x00 version/flags: % x", body)
		}
		// The account-key-filter field header must use the "hide UI" type nibble
		// (0x2), never "show UI" (0x0), so it can't trigger a pairing notification.
		if body[1]&0x0F != 0x2 {
			t.Fatalf("account-key filter must use the hide-UI type nibble: % x", body)
		}
		if len(d.AD) > AdvMaxLen {
			t.Fatalf("AD exceeds budget: %d", len(d.AD))
		}
	}
}

func TestPickTrackerWeighted(t *testing.T) {
	rng := rand.New(rand.NewPCG(5, 6))
	// Heavily weight Tile ([Tile]=20, [FastPair]=1); it should dominate but Fast
	// Pair still appears.
	var tile, fp int
	for i := 0; i < 5000; i++ {
		if pickTracker(rng, []int{20, 1}) == TrackerTile {
			tile++
		} else {
			fp++
		}
	}
	if tile <= fp {
		t.Fatalf("weighted Tile should dominate: tile=%d fp=%d", tile, fp)
	}
	if fp == 0 {
		t.Fatalf("Fast Pair should still appear with a nonzero weight")
	}
}

func TestBuildWithOptsTrackerShare(t *testing.T) {
	cfg := config.Default()
	rng := rand.New(rand.NewPCG(7, 8))
	// Trackers off => never a tracker even at share 100.
	for i := 0; i < 500; i++ {
		if isTracker(BuildWithOpts(cfg, rng, Options{Trackers: false, TrackerShare: 100})) {
			t.Fatalf("trackers off should never emit a tracker")
		}
	}
	// Trackers on, share 100 => always a tracker.
	for i := 0; i < 500; i++ {
		if !isTracker(BuildWithOpts(cfg, rng, Options{Trackers: true, TrackerShare: 100})) {
			t.Fatalf("share 100 should always emit a tracker")
		}
	}
}

func isTracker(d Decoy) bool {
	return d.CompanyID == ServiceTile || d.CompanyID == ServiceGoogleFastPair
}
