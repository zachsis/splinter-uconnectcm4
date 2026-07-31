//go:build linux

package hci

import (
	"errors"
	"fmt"
	"time"

	"golang.org/x/sys/unix"
)

// readBudget bounds every controller round-trip (via SO_RCVTIMEO) so a wedged
// controller can never block a read — and therefore never block shutdown.
const readBudget = 2 * time.Second

// Conn is an exclusive HCI transport for one controller. It holds a user-channel
// socket for HCI commands and a control (mgmt) socket used only to power the
// controller down for the takeover and back up on Close.
type Conn struct {
	userFd int
	ctrlFd int
	index  int
}

// New takes exclusive control of hci<index>: it opens the mgmt socket, powers
// the controller down (freeing it from bluetoothd), binds the user channel, and
// resets the controller. The caller must hold CAP_NET_RAW and CAP_NET_ADMIN.
func New(index int) (*Conn, error) {
	ctrl, err := openSocket(unix.HCI_CHANNEL_CONTROL, hciDevNone)
	if err != nil {
		return nil, fmt.Errorf("mgmt socket: %w", err)
	}
	if err := setRcvTimeout(ctrl); err != nil {
		unix.Close(ctrl)
		return nil, err
	}
	c := &Conn{userFd: -1, ctrlFd: ctrl, index: index}

	// Power down and BLOCK on the mgmt Command Complete — binding the user
	// channel before the async power-down finishes races the kernel (EBUSY).
	if err := c.mgmtSetPowered(false); err != nil {
		c.Close()
		return nil, fmt.Errorf("powering hci%d down: %w", index, err)
	}

	user, err := openSocket(unix.HCI_CHANNEL_USER, uint16(index))
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("user channel hci%d: %w", index, err)
	}
	c.userFd = user
	if err := setRcvTimeout(user); err != nil {
		c.Close()
		return nil, err
	}
	if err := c.Reset(); err != nil {
		c.Close()
		return nil, fmt.Errorf("hci reset: %w", err)
	}
	// On the user channel the kernel does no controller initialization, so we
	// must do it ourselves — otherwise a dual-mode controller (e.g. CYW43455)
	// accepts advertising params/data/address but refuses LE Set Advertise
	// Enable with "Command Disallowed" (0x0c). These mirror the LE-relevant
	// parts of the kernel's own bring-up; best-effort (ignore unsupported bits).
	c.initController()
	return c, nil
}

// initController performs the minimal LE bring-up the kernel normally does on a
// managed adapter. Best-effort: individual commands may be unsupported.
func (c *Conn) initController() {
	// Write LE Host Supported (OGF 0x03, OCF 0x006D): LE on, simultaneous off.
	_, _ = c.sendCommand(0x03, 0x006D, []byte{0x01, 0x00})
	// Set Event Mask (OGF 0x03, OCF 0x0001): default mask incl. LE Meta event.
	_, _ = c.sendCommand(0x03, 0x0001, []byte{0xFF, 0xFF, 0xFB, 0xFF, 0x07, 0xF8, 0xBF, 0x3D})
	// LE Set Event Mask (OGF 0x08, OCF 0x0001): enable LE events.
	_, _ = c.sendCommand(0x08, 0x0001, []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF})
	// LE Read Buffer Size (OGF 0x08, OCF 0x0002): part of standard bring-up.
	_, _ = c.sendCommand(0x08, 0x0002, nil)
}

// Close disables advertising (best-effort, bounded), releases the user channel,
// and restores the controller to bluetoothd. Safe to call on a partial Conn and
// never blocked by a hung controller.
func (c *Conn) Close() error {
	var firstErr error
	if c.userFd >= 0 {
		_ = c.SetAdvEnable(false) // best-effort; bounded by SO_RCVTIMEO
		if err := unix.Close(c.userFd); err != nil && firstErr == nil {
			firstErr = err
		}
		c.userFd = -1
	}
	if c.ctrlFd >= 0 {
		if err := c.mgmtSetPowered(true); err != nil && firstErr == nil {
			firstErr = err
		}
		if err := unix.Close(c.ctrlFd); err != nil && firstErr == nil {
			firstErr = err
		}
		c.ctrlFd = -1
	}
	return firstErr
}

// Reset issues HCI_Reset.
func (c *Conn) Reset() error { _, err := c.sendCommand(ogfHostCtl, ocfReset, nil); return err }

// SetAdvEnable enables or disables legacy advertising.
func (c *Conn) SetAdvEnable(on bool) error {
	v := byte(0)
	if on {
		v = 1
	}
	_, err := c.sendCommand(ogfLE, ocfSetAdvEnable, []byte{v})
	return err
}

// SetRandomAddr sets the LE random address (only valid while advertising is off).
func (c *Conn) SetRandomAddr(a [6]byte) error {
	_, err := c.sendCommand(ogfLE, ocfSetRandomAddr, a[:])
	return err
}

// SetAdvParams sets legacy advertising parameters. minMs/maxMs are milliseconds;
// the 0.625 ms unit conversion is done here. Only valid while advertising is off.
func (c *Conn) SetAdvParams(minMs, maxMs uint16, advType byte) error {
	_, err := c.sendCommand(ogfLE, ocfSetAdvParams, advParamsBlock(minMs, maxMs, advType))
	return err
}

// SetAdvData sets the advertising payload (<=31 bytes).
func (c *Conn) SetAdvData(ad []byte) error {
	_, err := c.sendCommand(ogfLE, ocfSetAdvData, advDataBlock(ad))
	return err
}

// Scan performs a passive LE scan for the given window and returns the observed
// advertising reports. Reuses this initialized transport (the same bring-up that
// makes advertising work on the user channel), so callers get the fix for free.
func (c *Conn) Scan(window time.Duration) ([]AdvReport, error) {
	// LE Set Scan Parameters: passive, ~30ms interval/window, public addr, no filter.
	if _, err := c.sendCommand(ogfLE, ocfSetScanParams, []byte{0x00, 0x30, 0x00, 0x30, 0x00, 0x00, 0x00}); err != nil {
		return nil, fmt.Errorf("set scan params: %w", err)
	}
	if _, err := c.sendCommand(ogfLE, ocfSetScanEnable, []byte{0x01, 0x00}); err != nil {
		return nil, fmt.Errorf("enable scan: %w", err)
	}
	defer func() { _, _ = c.sendCommand(ogfLE, ocfSetScanEnable, []byte{0x00, 0x00}) }()

	var reports []AdvReport
	buf := make([]byte, 512)
	deadline := time.Now().Add(window)
	for time.Now().Before(deadline) {
		n, err := unix.Read(c.userFd, buf)
		if err != nil {
			if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) {
				continue // SO_RCVTIMEO tick; keep scanning until the deadline
			}
			return reports, fmt.Errorf("scan read: %w", err)
		}
		reports = append(reports, parseAdvReports(buf[:n])...)
	}
	return reports, nil
}

// sendCommand writes an HCI command and returns the status from the Command
// Complete event whose opcode matches (ignoring unrelated events). A read
// timeout (SO_RCVTIMEO) is surfaced as an error rather than blocking.
func (c *Conn) sendCommand(ogf, ocf uint16, params []byte) (byte, error) {
	if _, err := unix.Write(c.userFd, cmdPacket(ogf, ocf, params)); err != nil {
		return 0, fmt.Errorf("hci write: %w", err)
	}
	want := opcode(ogf, ocf)
	buf := make([]byte, 512)
	deadline := time.Now().Add(readBudget + time.Second)
	for time.Now().Before(deadline) {
		n, err := unix.Read(c.userFd, buf)
		if err != nil {
			if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) {
				return 0, fmt.Errorf("timeout awaiting reply to opcode %#04x", want)
			}
			return 0, fmt.Errorf("hci read: %w", err)
		}
		status, matched, _ := parseCommandComplete(buf[:n], want)
		if matched {
			if status != 0 {
				return status, fmt.Errorf("opcode %#04x failed, status %#02x", want, status)
			}
			return 0, nil
		}
	}
	return 0, fmt.Errorf("deadline awaiting reply to opcode %#04x", want)
}

// mgmtSetPowered powers the controller on/off via the mgmt socket and blocks on
// the matching Command Complete.
func (c *Conn) mgmtSetPowered(on bool) error {
	cmd := mgmtCommand(mgmtOpSetPowered, uint16(c.index), setPoweredParam(on))
	if _, err := unix.Write(c.ctrlFd, cmd); err != nil {
		return fmt.Errorf("mgmt write: %w", err)
	}
	buf := make([]byte, 512)
	deadline := time.Now().Add(readBudget + time.Second)
	for time.Now().Before(deadline) {
		n, err := unix.Read(c.ctrlFd, buf)
		if err != nil {
			if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) {
				return errors.New("timeout awaiting mgmt set-powered reply")
			}
			return fmt.Errorf("mgmt read: %w", err)
		}
		status, matched, _ := parseMgmtCmdComplete(buf[:n], mgmtOpSetPowered)
		if matched {
			if status != 0 {
				return fmt.Errorf("mgmt set-powered failed, status %#02x", status)
			}
			return nil
		}
	}
	return errors.New("deadline awaiting mgmt set-powered reply")
}

func openSocket(channel uint16, dev uint16) (int, error) {
	fd, err := unix.Socket(unix.AF_BLUETOOTH, unix.SOCK_RAW|unix.SOCK_CLOEXEC, unix.BTPROTO_HCI)
	if err != nil {
		if errors.Is(err, unix.EPERM) {
			return -1, fmt.Errorf("opening an HCI socket requires CAP_NET_RAW: %w", err)
		}
		return -1, err
	}
	if err := unix.Bind(fd, &unix.SockaddrHCI{Dev: dev, Channel: channel}); err != nil {
		unix.Close(fd)
		switch {
		case errors.Is(err, unix.EPERM):
			return -1, fmt.Errorf("binding the HCI channel requires CAP_NET_ADMIN: %w", err)
		case errors.Is(err, unix.ENODEV):
			return -1, fmt.Errorf("can't open hci%d: no such device: %w", dev, err)
		default:
			return -1, err
		}
	}
	return fd, nil
}

func setRcvTimeout(fd int) error {
	tv := unix.NsecToTimeval(int64(readBudget))
	return unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &tv)
}
