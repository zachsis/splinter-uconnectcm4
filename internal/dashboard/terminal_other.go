//go:build !linux

package dashboard

import (
	"context"
	"io"
)

// IsTerminal always reports false off Linux (splinterd only runs on Linux; this
// keeps the tree building and the pure renderer testable on the dev Mac).
func IsTerminal(fd uintptr) bool { return false }

// Run is a no-op off Linux.
func Run(ctx context.Context, m *Model, out io.Writer, fd uintptr, rate RateAdjuster, onQuit func(), learn LearnController, apple AppleController, trackers TrackerController) {
}
