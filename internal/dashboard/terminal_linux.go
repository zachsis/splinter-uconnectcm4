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
// screen, reads keypresses, mirrors the live control state into the model, and
// redraws ~4x/sec until ctx is cancelled — then fully restores the terminal on
// return (including panic unwind).
func Run(ctx context.Context, m *Model, out io.Writer, fd uintptr, c Controls) {
	inFd := int(os.Stdin.Fd())
	if old, err := enableRaw(inFd); err == nil {
		defer restoreTerm(inFd, old)
		go readKeys(ctx, m, c)
	}

	fmt.Fprint(out, "\x1b[?1049h\x1b[?25l")       // alt screen + hide cursor
	defer fmt.Fprint(out, "\x1b[?25h\x1b[?1049l") // restore cursor + main screen

	draw := func() {
		if c.Learn != nil {
			m.SetLearn(c.Learn.Learning(), c.Learn.Summary())
			m.SetLearned(c.Learn.Devices())
		}
		if c.Apple != nil {
			m.SetAppleMode(c.Apple.Mode())
		}
		if c.Trackers != nil {
			m.SetTrackers(c.Trackers.Enabled())
		}
		if c.Broadcast != nil {
			m.SetPaused(c.Broadcast.Paused())
		}
		if c.Debug != nil {
			m.SetDebug(c.Debug.Enabled(), c.Debug.Path())
		}
		w, h := termSize(fd)
		m.SetScrollBounds(TableRows(h))
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

// readKeys reads keypresses (including multi-byte escape sequences) and applies
// them until ctx is cancelled. A short read deadline lets it observe cancellation
// and doubles as the ESC-delay: an incomplete escape sequence is carried to the
// next read, and a read timeout flushes it (so a genuine lone ESC isn't stuck).
func readKeys(ctx context.Context, m *Model, c Controls) {
	buf := make([]byte, 32)
	var carry []byte
	for ctx.Err() == nil {
		_ = os.Stdin.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		n, err := os.Stdin.Read(buf)
		if err != nil {
			if os.IsTimeout(err) {
				if len(carry) > 0 { // no continuation arrived; treat ESC as standalone
					forceInput(carry, m, c)
					carry = nil
				}
				continue
			}
			return
		}
		data := make([]byte, len(carry)+n) // fresh buffer: never aliases carry
		copy(data, carry)
		copy(data[len(carry):], buf[:n])
		consumed, quit := handleInput(data, m, c)
		if quit {
			return
		}
		carry = append(carry[:0], data[consumed:]...)
	}
}

// forceInput flushes a carried incomplete escape sequence when no continuation
// arrived, so the ESC never wedges. A lone/partial CSI has no bound action, so
// this is effectively a no-op beyond clearing the buffer.
func forceInput(data []byte, m *Model, c Controls) {
	for i := 1; i < len(data); i++ { // skip the leading ESC
		if handleSingle(data[i], m, c) {
			return
		}
	}
}
