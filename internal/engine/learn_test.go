package engine

import (
	"strings"
	"testing"

	"github.com/zachsis/helmofhades/internal/decoy"
	"github.com/zachsis/helmofhades/internal/hci"
)

func advWithCompany(id uint16) hci.AdvReport {
	// Flags AD + manufacturer data carrying the company ID (little-endian).
	data := []byte{0x02, 0x01, 0x06, 0x04, 0xFF, byte(id), byte(id >> 8), 0x00}
	return hci.AdvReport{Data: data}
}

func TestLearnWeights(t *testing.T) {
	// Observe lots of Samsung (0x0075), some Garmin (0x0087), and one unknown.
	var reports []hci.AdvReport
	for i := 0; i < 10; i++ {
		reports = append(reports, advWithCompany(0x0075))
	}
	for i := 0; i < 3; i++ {
		reports = append(reports, advWithCompany(0x0087))
	}
	reports = append(reports, advWithCompany(0x1234)) // not in our table

	weights, _, summary := learnWeights(reports)
	if len(weights) != len(decoy.Vendors) {
		t.Fatalf("weights len = %d, want %d", len(weights), len(decoy.Vendors))
	}
	// Every vendor keeps a baseline weight >= 1.
	for i, w := range weights {
		if w < 1 {
			t.Fatalf("vendor %d weight %d < baseline 1", i, w)
		}
	}
	// A Samsung entry should outweigh a Sony entry (Sony wasn't observed).
	var samsungW, sonyW int
	for i, v := range decoy.Vendors {
		if v.CompanyID == 0x0075 && samsungW == 0 {
			samsungW = weights[i]
		}
		if v.CompanyID == 0x012D && sonyW == 0 {
			sonyW = weights[i]
		}
	}
	if samsungW <= sonyW {
		t.Fatalf("observed Samsung weight %d should exceed unobserved Sony %d", samsungW, sonyW)
	}
	// Summary names the observed in-table vendors; the unknown ID is excluded.
	if !strings.Contains(summary, "Samsung") || !strings.Contains(summary, "Garmin") {
		t.Fatalf("summary missing observed vendors: %q", summary)
	}
	if strings.Contains(summary, "0x1234") {
		t.Fatalf("summary should not include out-of-table IDs: %q", summary)
	}
}

func TestLearnBoostDampenedAndCapped(t *testing.T) {
	if learnBoost(0) != 0 {
		t.Fatalf("no observation should give zero boost")
	}
	if b := learnBoost(1); b < 1 {
		t.Fatalf("a single observation should still register, got %d", b)
	}
	// sqrt-dampened: 100 observations must not be ~100x a single one.
	if b := learnBoost(100); b > learnMaxBoost {
		t.Fatalf("boost %d exceeds cap %d", b, learnMaxBoost)
	}
	// Monotonic up to the cap.
	if learnBoost(2) < learnBoost(1) {
		t.Fatalf("boost should be non-decreasing in observations")
	}
}

// TestLearnWeightsCapDominance guards the core anti-skew property: even when a
// single vendor utterly dominates the airwaves, it must not dominate the decoy
// crowd, or the decoys become a conspicuous cluster instead of camouflage.
func TestLearnWeightsCapDominance(t *testing.T) {
	var reports []hci.AdvReport
	for i := 0; i < 500; i++ { // only Samsung, and a LOT of it
		reports = append(reports, advWithCompany(0x0075))
	}
	weights, _, _ := learnWeights(reports)
	total, top := 0, 0
	for _, w := range weights {
		total += w
		if w > top {
			top = w
		}
	}
	share := float64(top) / float64(total)
	if share > 0.20 {
		t.Fatalf("single dominant vendor took %.0f%% of the crowd; want <=20%%", share*100)
	}
}

// nameAD builds a Complete Local Name AD structure.
func nameAD(name string) []byte {
	return append([]byte{byte(1 + len(name)), 0x09}, name...)
}

func TestBuildLearnedDevices(t *testing.T) {
	addr1 := [6]byte{1}
	addr2 := [6]byte{2}
	reports := []hci.AdvReport{
		// addr1, first sighting: weak RSSI, carries the company ID but no name.
		{Addr: addr1, RSSI: -70, Data: advWithCompany(0x0075).Data},
		// addr1, second sighting: stronger RSSI, carries a name but no mfg data.
		{Addr: addr1, RSSI: -50, Data: nameAD("Phone")},
		// addr2, single sighting.
		{Addr: addr2, RSSI: -40, Data: advWithCompany(0x1234).Data},
	}

	got := buildLearnedDevices(reports)
	if len(got) != 2 {
		t.Fatalf("want 2 distinct devices, got %d: %+v", len(got), got)
	}
	// Sorted by descending RSSI: addr2 (-40) before addr1 (-50).
	if got[0].MAC != addr2 || got[1].MAC != addr1 {
		t.Fatalf("want addr2 then addr1 (RSSI desc), got %+v", got)
	}
	d1 := got[1]
	if d1.RSSI != -50 {
		t.Errorf("addr1 RSSI = %d, want -50 (the stronger sighting)", d1.RSSI)
	}
	if d1.Name != "Phone" {
		t.Errorf("addr1 Name = %q, want %q (filled in from the earlier sighting)", d1.Name, "Phone")
	}
	if !d1.HasMfg || d1.CompanyID != 0x0075 {
		t.Errorf("addr1 mfg = (%v, %#04x), want (true, 0x0075) (filled in from the earlier sighting)", d1.HasMfg, d1.CompanyID)
	}
}

func TestLearnControlRequestAndStatus(t *testing.T) {
	l := NewLearnControl()
	if l.Learning() || l.Summary() != "" {
		t.Fatal("fresh control should be idle/empty")
	}
	l.Request()
	if !l.takeRequest() {
		t.Fatal("takeRequest should return true after Request")
	}
	if l.takeRequest() {
		t.Fatal("takeRequest should be false the second time")
	}
	l.setResult([]int{1, 2, 3}, []int{4, 7}, "Samsung 5")
	if got := l.weightsSnapshot(); len(got) != 3 || got[1] != 2 {
		t.Fatalf("weights snapshot wrong: %v", got)
	}
	if got := l.trackerWeightsSnapshot(); len(got) != 2 || got[1] != 7 {
		t.Fatalf("tracker weights snapshot wrong: %v", got)
	}
	if l.Summary() != "Samsung 5" {
		t.Fatalf("summary = %q", l.Summary())
	}
}
