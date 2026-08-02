package verify

import (
	"math/rand/v2"
	"strings"
	"testing"
	"time"

	"github.com/zachsis/helmofhades/internal/config"
	"github.com/zachsis/helmofhades/internal/decoy"
)

func TestResultString(t *testing.T) {
	fail := Analyze([]Observation{{MAC: mac(1), HasMfg: true, CompanyID: decoy.CompanyMicrosoft}}, time.Second, 0).String()
	if !strings.Contains(fail, "FAIL") || !strings.Contains(fail, "VIOLATION") {
		t.Fatalf("expected FAIL + VIOLATION, got:\n%s", fail)
	}
	obs := []Observation{
		{MAC: mac(1), HasMfg: true, CompanyID: 0x0075},
		{MAC: mac(2), HasMfg: true, CompanyID: 0x00E0},
	}
	pass := Analyze(obs, 10*time.Second, 0).String()
	if !strings.Contains(pass, "PASS") || !strings.Contains(pass, "vendor 0x0075") {
		t.Fatalf("expected PASS + histogram, got:\n%s", pass)
	}
}

// TestParseRoundTrip feeds decoy-built payloads back through the parser and
// checks the fields agree.
func TestParseRoundTrip(t *testing.T) {
	cfg := config.Default()
	rng := rand.New(rand.NewPCG(5, 6))
	for i := 0; i < 20000; i++ {
		ad := decoy.BuildAdvData(cfg, rng)
		info := ParseAdvData(ad)
		if info.FastPair {
			t.Fatalf("decoy payload should never look like Fast Pair: %x", ad)
		}
		if info.HasMfg && (info.CompanyID == decoy.CompanyApple || info.CompanyID == decoy.CompanyMicrosoft) {
			t.Fatalf("decoy emitted excluded id %#04x", info.CompanyID)
		}
		if info.Name != "" && len(info.Name) > 12 {
			t.Fatalf("parsed name too long: %q", info.Name)
		}
	}
}

func mac(b byte) [6]byte { return [6]byte{b, b, b, b, b, 0xC0} }

func TestParseAppleFindMy(t *testing.T) {
	// A crafted Apple advert carrying a Find My (0x12) message must be detected.
	findMy := []byte{0x02, 0x01, 0x06, 0x06, 0xFF, 0x4C, 0x00, 0x12, 0x02, 0xAA, 0xBB}
	if !ParseAdvData(findMy).AppleFindMy {
		t.Fatalf("expected Find My detection for % x", findMy)
	}
	// A real Nearby Info decoy (nearform) must NOT be flagged as Find My.
	rng := rand.New(rand.NewPCG(9, 10))
	for i := 0; i < 500; i++ {
		ad := decoy.BuildWithOpts(config.Default(), rng, decoy.Options{Apple: decoy.AppleNearform, AppleShare: 100}).AD
		info := ParseAdvData(ad)
		if !info.HasMfg || info.CompanyID != decoy.CompanyApple {
			t.Fatalf("expected Apple mfg advert, got id=%#04x hasMfg=%v", info.CompanyID, info.HasMfg)
		}
		if info.AppleFindMy {
			t.Fatalf("Nearby Info decoy wrongly flagged as Find My: % x", ad)
		}
	}
}

// TestTrackersRoundTrip feeds decoy-built tracker payloads through the parser:
// Tile classifies as Tile, and our Fast Pair decoys are always non-discoverable.
func TestTrackersRoundTrip(t *testing.T) {
	cfg := config.Default()
	rng := rand.New(rand.NewPCG(11, 12))
	var sawTile, sawFP bool
	for i := 0; i < 4000; i++ {
		d := decoy.BuildWithOpts(cfg, rng, decoy.Options{Trackers: true, TrackerShare: 100})
		info := ParseAdvData(d.AD)
		switch d.CompanyID {
		case decoy.ServiceTile:
			sawTile = true
			if !info.Tile {
				t.Fatalf("Tile decoy not detected as Tile: % x", d.AD)
			}
		case decoy.ServiceGoogleFastPair:
			sawFP = true
			if !info.FastPair {
				t.Fatalf("Fast Pair decoy not detected: % x", d.AD)
			}
			if info.FastPairDiscoverable {
				t.Fatalf("our Fast Pair decoy must never be discoverable: % x", d.AD)
			}
		default:
			t.Fatalf("unexpected tracker id %#04x", d.CompanyID)
		}
	}
	if !sawTile || !sawFP {
		t.Fatalf("expected both tracker kinds over the run (tile=%v fp=%v)", sawTile, sawFP)
	}
}

func TestAnalyzeCleanPass(t *testing.T) {
	var obs []Observation
	for i := 0; i < 40; i++ {
		obs = append(obs, Observation{MAC: mac(byte(i)), HasMfg: true, CompanyID: uint16(0x0075 + i%4)})
	}
	r := Analyze(obs, 10*time.Second, 250) // theoretical 4/s, floor 2/s; observed 4/s
	if !r.Pass {
		t.Fatalf("expected pass, got: %s", r)
	}
	if r.DistinctMACs != 40 || r.IDSpread != 4 {
		t.Fatalf("unexpected counts: %+v", r)
	}
}

func TestAnalyzeGuardrailViolations(t *testing.T) {
	obs := []Observation{
		{MAC: mac(1), HasMfg: true, CompanyID: decoy.CompanyMicrosoft},                // Swift Pair popup
		{MAC: mac(2), Connectable: true},                                              // connectable
		{MAC: mac(3), FastPair: true, FastPairDiscoverable: true},                     // discoverable Fast Pair popup
		{MAC: mac(4), HasMfg: true, CompanyID: decoy.CompanyApple, AppleFindMy: true}, // anti-stalking alert
	}
	r := Analyze(obs, time.Second, 0)
	if r.Pass {
		t.Fatalf("expected fail, got pass: %s", r)
	}
	if len(r.Violations) != 4 {
		t.Fatalf("expected 4 violations, got %d: %v", len(r.Violations), r.Violations)
	}
}

// TestAnalyzeAllowedTrackers confirms Tile and non-discoverable Fast Pair are
// allowed (no bystander UI), while a discoverable Fast Pair frame still fails.
func TestAnalyzeAllowedTrackers(t *testing.T) {
	ok := []Observation{
		{MAC: mac(1), Tile: true},
		{MAC: mac(2), FastPair: true}, // non-discoverable (FastPairDiscoverable false)
	}
	if r := Analyze(ok, time.Second, 0); !r.Pass {
		t.Fatalf("Tile + non-discoverable Fast Pair should pass, got: %s", r)
	}
	bad := []Observation{{MAC: mac(1), FastPair: true, FastPairDiscoverable: true}}
	if r := Analyze(bad, time.Second, 0); r.Pass {
		t.Fatalf("discoverable Fast Pair should fail")
	}
}

// TestAnalyzeAppleAllowed confirms the relaxed guardrail: a plain Apple presence
// beacon (naive or Nearby Info, not Find My) is no longer a violation.
func TestAnalyzeAppleAllowed(t *testing.T) {
	var obs []Observation
	for i := 0; i < 40; i++ {
		id := uint16(0x0075 + i%3)
		if i%3 == 2 {
			id = decoy.CompanyApple // mix Apple presence beacons into the crowd
		}
		obs = append(obs, Observation{MAC: mac(byte(i)), HasMfg: true, CompanyID: id})
	}
	r := Analyze(obs, 10*time.Second, 250)
	if !r.Pass {
		t.Fatalf("Apple presence beacons should be allowed, got: %s", r)
	}
}

func TestAnalyzeRateFloor(t *testing.T) {
	// 5 distinct MACs over 10s = 0.5/s; theoretical at rotate=250 is 4/s, floor 2/s -> fail.
	var obs []Observation
	for i := 0; i < 5; i++ {
		obs = append(obs, Observation{MAC: mac(byte(i)), HasMfg: true, CompanyID: 0x0075})
	}
	if r := Analyze(obs, 10*time.Second, 250); r.Pass {
		t.Fatalf("expected rate-floor failure, got pass: %s", r)
	}
	// Same data, benchmark mode (rotate 0) skips the rate floor -> passes guardrails.
	if r := Analyze(obs, 10*time.Second, 0); !r.Pass {
		t.Fatalf("benchmark mode should skip rate floor: %s", r)
	}
}
