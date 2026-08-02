package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenDebugFilePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "debug.log")
	f, gotPath, err := OpenDebugFile(path)
	if err != nil {
		t.Fatalf("OpenDebugFile: %v", err)
	}
	defer f.Close()
	if gotPath != path {
		t.Errorf("path = %q, want %q", gotPath, path)
	}
	info, err := f.Stat()
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("perm = %v, want 0600 (log captures nearby device MACs/names)", perm)
	}
}

func TestDebugControlEnableDisableToggle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "debug.log")
	c, log := NewDebugControl(path)
	if c.Enabled() || c.Path() != "" {
		t.Fatalf("fresh control should be disabled with no path")
	}

	// Writes before Enable are discarded, not written to disk.
	log.Debug("should be discarded")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("log file should not exist before Enable: err=%v", err)
	}

	if err := c.Enable(); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	if !c.Enabled() || c.Path() != path {
		t.Fatalf("after Enable: Enabled()=%v Path()=%q, want true/%q", c.Enabled(), c.Path(), path)
	}
	// A second Enable is a no-op (idempotent, doesn't reopen/truncate).
	if err := c.Enable(); err != nil {
		t.Fatalf("second Enable: %v", err)
	}

	log.Debug("hello")
	if got := c.Toggle(); got {
		t.Fatalf("Toggle from enabled should return false (disabled)")
	}
	if c.Enabled() || c.Path() != "" {
		t.Fatalf("after Disable: Enabled()=%v Path()=%q, want false/\"\"", c.Enabled(), c.Path())
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("log file should exist after being enabled: %v", err)
	}
	if info.Size() == 0 {
		t.Errorf("log file should contain the write made while enabled")
	}

	// Writing through the (now-discarding) logger after Disable must not panic
	// or error visibly, and must not grow the closed file.
	sizeAfterDisable := info.Size()
	log.Debug("after disable, should be discarded")
	if info, err := os.Stat(path); err == nil && info.Size() != sizeAfterDisable {
		t.Errorf("file grew after Disable: %d -> %d", sizeAfterDisable, info.Size())
	}

	// Toggle from disabled re-enables (opens a fresh file at the same path).
	if got := c.Toggle(); !got || !c.Enabled() {
		t.Fatalf("Toggle from disabled should re-enable")
	}
}
