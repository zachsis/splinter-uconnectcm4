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
	fail := Analyze([]Observation{{MAC: mac(1), HasMfg: true, CompanyID: decoy.CompanyApple}}, time.Second, 0).String()
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
		id, hasMfg, name, fastPair := ParseAdvData(ad)
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
		{MAC: mac(1), HasMfg: true, CompanyID: decoy.CompanyApple},
		{MAC: mac(2), Connectable: true},
		{MAC: mac(3), FastPair: true},
	}
	r := Analyze(obs, time.Second, 0)
	if r.Pass {
		t.Fatalf("expected fail, got pass: %s", r)
	}
	if len(r.Violations) != 3 {
		t.Fatalf("expected 3 violations, got %d: %v", len(r.Violations), r.Violations)
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
