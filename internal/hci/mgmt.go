package hci

import "encoding/binary"

// Bluetooth management (mgmt) API framing — a separate protocol from HCI
// commands, spoken on the HCI_CHANNEL_CONTROL socket. Used only to power the
// controller down (to free it for the user channel) and back up on exit.
const (
	mgmtOpReadInfo    = 0x0004
	mgmtOpSetPowered  = 0x0005
	mgmtEvCmdComplete = 0x0001
	mgmtEvCmdStatus   = 0x0002

	hciDevNone = 0xFFFF // bind index for the control socket
)

// mgmtCommand frames a management command: [opcode LE][index LE][len LE][params].
func mgmtCommand(op uint16, index uint16, params []byte) []byte {
	buf := make([]byte, 6+len(params))
	binary.LittleEndian.PutUint16(buf[0:2], op)
	binary.LittleEndian.PutUint16(buf[2:4], index)
	binary.LittleEndian.PutUint16(buf[4:6], uint16(len(params)))
	copy(buf[6:], params)
	return buf
}

// parseMgmtCmdComplete inspects a management event frame and, if it is a Command
// Complete OR a Command Status for wantOp, returns its status. matched is false
// for other events. (A command that fails immediately — e.g. Set Powered while
// the controller is Busy — comes back as Command Status, not Command Complete;
// both carry [cmd_op:u16 @6][status:u8 @8], so we accept either.)
func parseMgmtCmdComplete(pkt []byte, wantOp uint16) (status byte, matched bool, ok bool) {
	if len(pkt) < 9 {
		return 0, false, len(pkt) >= 6 // short-but-valid header => keep reading
	}
	ev := binary.LittleEndian.Uint16(pkt[0:2])
	if ev != mgmtEvCmdComplete && ev != mgmtEvCmdStatus {
		return 0, false, true
	}
	if binary.LittleEndian.Uint16(pkt[6:8]) != wantOp {
		return 0, false, true
	}
	return pkt[8], true, true
}

// MgmtStatusBusy is the mgmt status returned when the controller can't be
// powered off right now (e.g. bluetoothd has an active connection/operation).
const MgmtStatusBusy = 0x0a

// setPoweredParam is the 1-byte payload for MGMT_OP_SET_POWERED.
func setPoweredParam(on bool) []byte {
	if on {
		return []byte{0x01}
	}
	return []byte{0x00}
}
