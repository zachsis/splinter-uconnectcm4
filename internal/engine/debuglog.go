package engine

import (
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// DefaultLogPath returns a timestamped log filename in the current directory,
// used when no explicit --log-file is given.
func DefaultLogPath() string {
	return fmt.Sprintf("hohd-%s.log", time.Now().Format("20060102-150405"))
}

// OpenDebugFile opens (create/append) the debug log at path, or a timestamped
// default when path is empty. The file is 0600 so recon it captures (nearby
// device MACs/names) isn't world-readable.
func OpenDebugFile(path string) (*os.File, string, error) {
	if path == "" {
		path = DefaultLogPath()
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, "", err
	}
	return f, path, nil
}

// debugWriter forwards writes to the currently-open log file, or discards when
// debug logging is off. The hot path is lock-free (an atomic pointer load).
type debugWriter struct{ file atomic.Pointer[os.File] }

func (w *debugWriter) Write(p []byte) (int, error) {
	if f := w.file.Load(); f != nil {
		return f.Write(p)
	}
	return len(p), nil // discard
}

// DebugControl toggles debug file logging at runtime (the dashboard 'D' key).
// The engine logs at debug level through a switchable writer this owns; while
// disabled the writer discards, so an idle debug switch never touches the disk
// and never corrupts the dashboard.
type DebugControl struct {
	w        *debugWriter
	pathTmpl string
	mu       sync.Mutex
	curPath  string
}

// NewDebugControl returns a control plus the *slog.Logger the engine should use
// in dashboard mode. The logger emits at debug level into the switchable writer;
// nothing is written until Enable/Toggle opens a file. pathTmpl is the
// --log-file value ("" => a timestamped file in the current directory).
func NewDebugControl(pathTmpl string) (*DebugControl, *slog.Logger) {
	w := &debugWriter{}
	c := &DebugControl{w: w, pathTmpl: pathTmpl}
	log := slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return c, log
}

// Enabled reports whether a log file is currently open.
func (c *DebugControl) Enabled() bool { return c.w.file.Load() != nil }

// Path returns the open log file's path, or "".
func (c *DebugControl) Path() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.curPath
}

// Enable opens the log file if not already open (best-effort).
func (c *DebugControl) Enable() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.w.file.Load() != nil {
		return nil
	}
	f, path, err := OpenDebugFile(c.pathTmpl)
	if err != nil {
		return err
	}
	c.curPath = path
	c.w.file.Store(f)
	return nil
}

// Disable closes the log file if open.
func (c *DebugControl) Disable() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if f := c.w.file.Swap(nil); f != nil {
		_ = f.Close()
	}
	c.curPath = ""
}

// Toggle flips debug logging and returns the new enabled state (false if the
// file couldn't be opened).
func (c *DebugControl) Toggle() bool {
	if c.Enabled() {
		c.Disable()
		return false
	}
	return c.Enable() == nil
}
