//go:build linux

package dashboard

import (
	"context"
	"fmt"
	"io"
	"time"

	"golang.org/x/sys/unix"
)

// IsTerminal reports whether fd refers to a terminal.
func IsTerminal(fd uintptr) bool {
	_, err := unix.IoctlGetTermios(int(fd), unix.TCGETS)
	return err == nil
}

func termSize(fd uintptr) (w, h int) {
	ws, err := unix.IoctlGetWinsize(int(fd), unix.TIOCGWINSZ)
	if err != nil || ws.Col == 0 {
		return 80, 24
	}
	return int(ws.Col), int(ws.Row)
}

// Run drives the dashboard: it switches to the alternate screen, redraws the
// model ~4x/sec until ctx is cancelled, then restores the terminal. Safe in a
// goroutine; the terminal is always restored on return (including panic unwind).
func Run(ctx context.Context, m *Model, out io.Writer, fd uintptr) {
	fmt.Fprint(out, "\x1b[?1049h\x1b[?25l")       // alt screen + hide cursor
	defer fmt.Fprint(out, "\x1b[?25h\x1b[?1049l") // restore cursor + main screen

	draw := func() {
		w, h := termSize(fd)
		fmt.Fprint(out, "\x1b[H\x1b[2J"+RenderFrame(m.Snapshot(), w, h))
	}
	draw()

	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			draw()
		}
	}
}
