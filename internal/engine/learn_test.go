package engine

import (
	"strings"
	"testing"

	"github.com/zachsis/splinter-uconnectcm4/internal/decoy"
	"github.com/zachsis/splinter-uconnectcm4/internal/hci"
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

	weights, summary := learnWeights(reports)
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
	l.setResult([]int{1, 2, 3}, "Samsung 5")
	if got := l.weightsSnapshot(); len(got) != 3 || got[1] != 2 {
		t.Fatalf("weights snapshot wrong: %v", got)
	}
	if l.Summary() != "Samsung 5" {
		t.Fatalf("summary = %q", l.Summary())
	}
}
