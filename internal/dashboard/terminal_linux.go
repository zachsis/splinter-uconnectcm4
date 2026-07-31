//go:build linux

package dashboard

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// rateStepMs is how much each +/- keypress changes the rotation interval.
const rateStepMs = 25

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

// enableRaw puts fd into raw-ish mode (no line buffering, no echo) so single
// keypresses arrive immediately, returning the prior state for restoration.
func enableRaw(fd int) (*unix.Termios, error) {
	t, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return nil, err
	}
	old := *t
	t.Lflag &^= unix.ICANON | unix.ECHO
	t.Cc[unix.VMIN] = 1
	t.Cc[unix.VTIME] = 0
	if err := unix.IoctlSetTermios(fd, unix.TCSETS, t); err != nil {
		return nil, err
	}
	return &old, nil
}

func restoreTerm(fd int, old *unix.Termios) {
	if old != nil {
		_ = unix.IoctlSetTermios(fd, unix.TCSETS, old)
	}
}

// Run drives the dashboard: it puts the terminal into raw mode + the alternate
// screen, reads keypresses (+/- rate, q quit), redraws ~4x/sec until ctx is
// cancelled, then fully restores the terminal on return (including panic unwind).
func Run(ctx context.Context, m *Model, out io.Writer, fd uintptr, rate RateAdjuster, onQuit func()) {
	inFd := int(os.Stdin.Fd())
	if old, err := enableRaw(inFd); err == nil {
		defer restoreTerm(inFd, old)
		go readKeys(ctx, m, rate, onQuit)
	}

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

// readKeys reads single keypresses and applies them until ctx is cancelled. It
// uses a short read deadline so it can observe cancellation promptly.
func readKeys(ctx context.Context, m *Model, rate RateAdjuster, onQuit func()) {
	buf := make([]byte, 1)
	for ctx.Err() == nil {
		_ = os.Stdin.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		n, err := os.Stdin.Read(buf)
		if err != nil {
			if os.IsTimeout(err) {
				continue
			}
			return
		}
		if n == 0 {
			continue
		}
		switch buf[0] {
		case 'q', 'Q', 0x03: // 0x03 = Ctrl-C (if ISIG is off)
			onQuit()
			return
		case '+', '=':
			if rate != nil {
				m.SetRate(rate.Adjust(-rateStepMs)) // shorter interval = faster
			}
		case '-', '_':
			if rate != nil {
				m.SetRate(rate.Adjust(+rateStepMs)) // longer interval = slower
			}
		case 't', 'T':
			m.CycleTheme()
		}
	}
}
