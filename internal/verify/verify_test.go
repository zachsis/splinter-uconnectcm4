package verify

import (
	"math/rand/v2"
	"strings"
	"testing"
	"time"

	"github.com/zachsis/splinter-uconnectcm4/internal/config"
	"github.com/zachsis/splinter-uconnectcm4/internal/decoy"
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
		id, hasMfg, name, fastPair, _ := ParseAdvData(ad)
		if fastPair {
			t.Fatalf("decoy payload should never look like Fast Pair: %x", ad)
		}
		if hasMfg && (id == decoy.CompanyApple || id == decoy.CompanyMicrosoft) {
			t.Fatalf("decoy emitted excluded id %#04x", id)
		}
		if name != "" && len(name) > 12 {
			t.Fatalf("parsed name too long: %q", name)
		}
	}
}

func mac(b byte) [6]byte { return [6]byte{b, b, b, b, b, 0xC0} }

func TestParseAppleFindMy(t *testing.T) {
	// A crafted Apple advert carrying a Find My (0x12) message must be detected.
	findMy := []byte{0x02, 0x01, 0x06, 0x06, 0xFF, 0x4C, 0x00, 0x12, 0x02, 0xAA, 0xBB}
	if _, _, _, _, isFindMy := ParseAdvData(findMy); !isFindMy {
		t.Fatalf("expected Find My detection for % x", findMy)
	}
	// A real Nearby Info decoy (nearform) must NOT be flagged as Find My.
	rng := rand.New(rand.NewPCG(9, 10))
	for i := 0; i < 500; i++ {
		ad := decoy.BuildWithOpts(config.Default(), rng, decoy.Options{Apple: decoy.AppleNearform, AppleShare: 100}).AD
		id, hasMfg, _, _, isFindMy := ParseAdvData(ad)
		if !hasMfg || id != decoy.CompanyApple {
			t.Fatalf("expected Apple mfg advert, got id=%#04x hasMfg=%v", id, hasMfg)
		}
		if isFindMy {
			t.Fatalf("Nearby Info decoy wrongly flagged as Find My: % x", ad)
		}
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
		{MAC: mac(3), FastPair: true},                                                 // Fast Pair service data
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
