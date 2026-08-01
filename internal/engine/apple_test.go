package engine

import (
	"testing"

	"github.com/zachsis/splinter-uconnectcm4/internal/decoy"
)

func TestAppleControlCycle(t *testing.T) {
	c := NewAppleControl(decoy.AppleNaive)
	if c.Kind() != decoy.AppleNaive || c.Mode() != "naive" {
		t.Fatalf("initial = %v/%q, want naive", c.Kind(), c.Mode())
	}
	// naive -> nearby-info -> off -> naive
	want := []string{"nearby-info", "off", "naive"}
	for i, w := range want {
		if got := c.Cycle(); got != w {
			t.Fatalf("cycle %d = %q, want %q", i, got, w)
		}
	}
	if c.Kind() != decoy.AppleNaive {
		t.Fatalf("after full cycle Kind = %v, want naive", c.Kind())
	}
}

func TestParseAppleMode(t *testing.T) {
	cases := map[string]decoy.AppleKind{
		"off":         decoy.AppleOff,
		"naive":       decoy.AppleNaive,
		"nearform":    decoy.AppleNearform,
		"nearby-info": decoy.AppleNearform,
	}
	for s, want := range cases {
		if got, ok := ParseAppleMode(s); !ok || got != want {
			t.Errorf("ParseAppleMode(%q) = %v,%v want %v", s, got, ok, want)
		}
	}
	if _, ok := ParseAppleMode("bogus"); ok {
		t.Errorf("ParseAppleMode(bogus) should report !ok")
	}
}
