//go:build linux

package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/zachsis/splinter-uconnectcm4/internal/verify"
	"golang.org/x/sys/unix"
)

// Minimal, self-contained HCI passive scanner. Like the daemon's transport it
// takes exclusive control of the adapter (mgmt power-down + user channel), then
// enables passive scanning and collects LE advertising reports for a window.

const (
	pktCommand = 0x01
	pktEvent   = 0x04

	ogfLE         = 0x08
	ogfHostCtl    = 0x03
	ocfReset      = 0x0003
	ocfScanParams = 0x000B
	ocfScanEnable = 0x000C

	evtCommandComplete = 0x0E
	evtLEMeta          = 0x3E
	subEvtAdvReport    = 0x02

	mgmtOpSetPowered = 0x0005
	hciDevNone       = 0xFFFF

	scanBudget = 2 * time.Second
)

func opcode(ogf, ocf uint16) uint16 { return ocf | (ogf << 10) }

// scan performs a passive LE scan on hci<index> for the given duration and
// returns the observed advertisements.
func scan(index int, window time.Duration) ([]verify.Observation, error) {
	ctrl, err := openSock(unix.HCI_CHANNEL_CONTROL, hciDevNone)
	if err != nil {
		return nil, fmt.Errorf("mgmt socket: %w", err)
	}
	defer unix.Close(ctrl)
	setTimeout(ctrl)
	if err := mgmtSetPowered(ctrl, index, false); err != nil {
		return nil, fmt.Errorf("power down hci%d: %w", index, err)
	}
	// Always restore the adapter to bluetoothd.
	defer mgmtSetPowered(ctrl, index, true)

	user, err := openSock(unix.HCI_CHANNEL_USER, uint16(index))
	if err != nil {
		return nil, fmt.Errorf("user channel hci%d: %w", index, err)
	}
	defer unix.Close(user)
	setTimeout(user)

	if _, err := sendCmd(user, ogfHostCtl, ocfReset, nil); err != nil {
		return nil, fmt.Errorf("reset: %w", err)
	}
	// LE Set Scan Parameters: passive, ~30ms interval/window, public addr, no filter.
	params := make([]byte, 7)
	params[0] = 0x00 // passive
	binary.LittleEndian.PutUint16(params[1:3], 0x0030)
	binary.LittleEndian.PutUint16(params[3:5], 0x0030)
	if _, err := sendCmd(user, ogfLE, ocfScanParams, params); err != nil {
		return nil, fmt.Errorf("set scan params: %w", err)
	}
	if _, err := sendCmd(user, ogfLE, ocfScanEnable, []byte{0x01, 0x00}); err != nil {
		return nil, fmt.Errorf("enable scan: %w", err)
	}
	defer sendCmd(user, ogfLE, ocfScanEnable, []byte{0x00, 0x00})

	var obs []verify.Observation
	buf := make([]byte, 512)
	deadline := time.Now().Add(window)
	for time.Now().Before(deadline) {
		n, err := unix.Read(user, buf)
		if err != nil {
			if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) {
				continue // SO_RCVTIMEO tick; keep scanning until the deadline
			}
			return obs, fmt.Errorf("read: %w", err)
		}
		obs = append(obs, parseAdvReports(buf[:n])...)
	}
	return obs, nil
}

// parseAdvReports extracts observations from an LE Advertising Report meta event.
func parseAdvReports(pkt []byte) []verify.Observation {
	if len(pkt) < 4 || pkt[0] != pktEvent || pkt[1] != evtLEMeta || pkt[3] != subEvtAdvReport {
		return nil
	}
	p := pkt[4:]
	if len(p) < 1 {
		return nil
	}
	num := int(p[0])
	p = p[1:]
	var out []verify.Observation
	for i := 0; i < num; i++ {
		if len(p) < 9 {
			break
		}
		evType := p[0]
		var addr [6]byte
		copy(addr[:], p[2:8]) // addr is little-endian; kept as-is for identity only
		dataLen := int(p[8])
		if len(p) < 9+dataLen+1 {
			break
		}
		data := p[9 : 9+dataLen]
		id, hasMfg, name, fastPair := verify.ParseAdvData(data)
		out = append(out, verify.Observation{
			MAC:         addr,
			Connectable: evType == 0x00 || evType == 0x01, // ADV_IND / ADV_DIRECT_IND
			CompanyID:   id,
			HasMfg:      hasMfg,
			Name:        name,
			FastPair:    fastPair,
		})
		p = p[9+dataLen+1:] // advance past this report (incl. RSSI byte)
	}
	return out
}

func openSock(channel uint16, dev uint16) (int, error) {
	fd, err := unix.Socket(unix.AF_BLUETOOTH, unix.SOCK_RAW|unix.SOCK_CLOEXEC, unix.BTPROTO_HCI)
	if err != nil {
		return -1, err
	}
	if err := unix.Bind(fd, &unix.SockaddrHCI{Dev: dev, Channel: channel}); err != nil {
		unix.Close(fd)
		return -1, err
	}
	return fd, nil
}

func setTimeout(fd int) {
	tv := unix.NsecToTimeval(int64(scanBudget))
	_ = unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &tv)
}

func sendCmd(fd int, ogf, ocf uint16, params []byte) (byte, error) {
	pkt := make([]byte, 4+len(params))
	pkt[0] = pktCommand
	binary.LittleEndian.PutUint16(pkt[1:3], opcode(ogf, ocf))
	pkt[3] = byte(len(params))
	copy(pkt[4:], params)
	if _, err := unix.Write(fd, pkt); err != nil {
		return 0, err
	}
	want := opcode(ogf, ocf)
	buf := make([]byte, 512)
	deadline := time.Now().Add(scanBudget + time.Second)
	for time.Now().Before(deadline) {
		n, err := unix.Read(fd, buf)
		if err != nil {
			if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) {
				return 0, errors.New("timeout awaiting command complete")
			}
			return 0, err
		}
		if n >= 7 && buf[0] == pktEvent && buf[1] == evtCommandComplete &&
			binary.LittleEndian.Uint16(buf[4:6]) == want {
			if buf[6] != 0 {
				return buf[6], fmt.Errorf("opcode %#04x status %#02x", want, buf[6])
			}
			return 0, nil
		}
	}
	return 0, errors.New("deadline awaiting command complete")
}

func mgmtSetPowered(fd int, index int, on bool) error {
	val := byte(0)
	if on {
		val = 1
	}
	cmd := make([]byte, 7)
	binary.LittleEndian.PutUint16(cmd[0:2], mgmtOpSetPowered)
	binary.LittleEndian.PutUint16(cmd[2:4], uint16(index))
	binary.LittleEndian.PutUint16(cmd[4:6], 1)
	cmd[6] = val
	if _, err := unix.Write(fd, cmd); err != nil {
		return err
	}
	buf := make([]byte, 512)
	deadline := time.Now().Add(scanBudget + time.Second)
	for time.Now().Before(deadline) {
		n, err := unix.Read(fd, buf)
		if err != nil {
			if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) {
				return errors.New("timeout awaiting mgmt reply")
			}
			return err
		}
		// mgmt CMD_COMPLETE (0x0001) with matching cmd opcode.
		if n >= 9 && binary.LittleEndian.Uint16(buf[0:2]) == 0x0001 &&
			binary.LittleEndian.Uint16(buf[6:8]) == mgmtOpSetPowered {
			if buf[8] != 0 {
				return fmt.Errorf("mgmt set-powered status %#02x", buf[8])
			}
			return nil
		}
	}
	return errors.New("deadline awaiting mgmt reply")
}
