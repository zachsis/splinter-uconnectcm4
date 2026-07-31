package config

import (
	"errors"
	"flag"
	"io"
	"testing"
)

func TestParseDefaults(t *testing.T) {
	cfg, err := Parse("splinterd", nil, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if cfg != Default() {
		t.Fatalf("no flags should equal Default(), got %+v", cfg)
	}
}

func TestParseOverrides(t *testing.T) {
	cfg, err := Parse("splinterd", []string{"--rotate-ms", "500", "--benchmark", "--hci", "1"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RotateMs != 500 || !cfg.Benchmark || cfg.HCIIndex != 1 {
		t.Fatalf("flags not applied: %+v", cfg)
	}
}

func TestParseVersion(t *testing.T) {
	if _, err := Parse("splinterd", []string{"--version"}, io.Discard); !errors.Is(err, ErrVersion) {
		t.Fatalf("expected ErrVersion, got %v", err)
	}
}

func TestParseHelp(t *testing.T) {
	if _, err := Parse("splinterd", []string{"-h"}, io.Discard); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected flag.ErrHelp, got %v", err)
	}
}

func TestParseInvalid(t *testing.T) {
	cases := [][]string{
		{"--adv-ms", "10"},
		{"--name-prob", "150"},
		{"--mfg-prob", "-1"},
		{"--rotate-ms", "-5"},
		{"--hci", "-1"},
	}
	for _, args := range cases {
		if _, err := Parse("splinterd", args, io.Discard); err == nil {
			t.Fatalf("expected validation error for %v", args)
		}
	}
}
