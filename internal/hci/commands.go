// Package hci is a minimal HCI transport that drives a Bluetooth controller
// directly for legacy LE advertising. The command/event encoding in this file is
// pure (no syscalls) so it is unit-tested on any OS; the exclusive-socket
// transport lives in hci_linux.go.
package hci

import "encoding/binary"

// HCI packet type indicators (first octet on the user channel).
const (
	pktCommand = 0x01
	pktEvent   = 0x04
)

// Opcode groups/commands (OGF/OCF) we use.
const (
	ogfLE            = 0x08
	ogfHostCtl       = 0x03
	ocfReset         = 0x0003 // with ogfHostCtl
	ocfSetRandomAddr = 0x0005
	ocfSetAdvParams  = 0x0006
	ocfSetAdvData    = 0x0008
	ocfSetAdvEnable  = 0x000A
	ocfSetScanParams = 0x000B
	ocfSetScanEnable = 0x000C
)

// Event codes.
const (
	evtCommandComplete = 0x0E
	evtLEMeta          = 0x3E
	subEvtAdvReport    = 0x02
)

// AdvReport is one observed advertising report from a passive scan.
type AdvReport struct {
	Addr        [6]byte
	Connectable bool
	Data        []byte // raw advertising payload (AD structures)
}

// parseAdvReports extracts advertising reports from an LE Advertising Report
// meta event. Returns nil for any other packet or on truncation.
func parseAdvReports(pkt []byte) []AdvReport {
	if len(pkt) < 4 || pkt[0] != pktEvent || pkt[1] != evtLEMeta || pkt[3] != subEvtAdvReport {
		return nil
	}
	p := pkt[4:]
	if len(p) < 1 {
		return nil
	}
	num := int(p[0])
	p = p[1:]
	var out []AdvReport
	for i := 0; i < num; i++ {
		if len(p) < 9 {
			break
		}
		evType := p[0]
		dataLen := int(p[8])
		if len(p) < 9+dataLen+1 {
			break
		}
		var r AdvReport
		copy(r.Addr[:], p[2:8])
		r.Connectable = evType == 0x00 || evType == 0x01 // ADV_IND / ADV_DIRECT_IND
		r.Data = append([]byte(nil), p[9:9+dataLen]...)
		out = append(out, r)
		p = p[9+dataLen+1:] // advance past this report (incl. RSSI byte)
	}
	return out
}

// ADV_NONCONN_IND — non-connectable, non-scannable undirected advertising.
const AdvNonconnInd = 0x03

// opcode packs an OGF/OCF pair into the 16-bit HCI opcode.
func opcode(ogf, ocf uint16) uint16 { return ocf | (ogf << 10) }

// advIntervalUnits converts milliseconds to the controller's 0.625 ms units
// (ms * 1000 / 625 == ms * 8 / 5), matching the firmware's ADV_ITVL_UNITS.
func advIntervalUnits(ms uint16) uint16 { return uint16(uint32(ms) * 8 / 5) }

// cmdPacket frames a full HCI command packet: [0x01][opcode LE][plen][params].
func cmdPacket(ogf, ocf uint16, params []byte) []byte {
	pkt := make([]byte, 4+len(params))
	pkt[0] = pktCommand
	binary.LittleEndian.PutUint16(pkt[1:3], opcode(ogf, ocf))
	pkt[3] = byte(len(params))
	copy(pkt[4:], params)
	return pkt
}

// advParamsBlock builds the 15-byte LE Set Advertising Parameters payload. Peer
// address is zeroed; channel map is all three advertising channels (0x07);
// filter policy allows any (0x00); own address type is random (0x01).
func advParamsBlock(minMs, maxMs uint16, advType byte) []byte {
	p := make([]byte, 15)
	binary.LittleEndian.PutUint16(p[0:2], advIntervalUnits(minMs))
	binary.LittleEndian.PutUint16(p[2:4], advIntervalUnits(maxMs))
	p[4] = advType
	p[5] = 0x01 // own address type: random
	p[6] = 0x00 // peer address type
	// p[7:13] peer address left zero
	p[13] = 0x07 // channel map: 37/38/39
	p[14] = 0x00 // filter policy: allow any
	return p
}

// advDataBlock builds the 32-byte LE Set Advertising Data payload:
// [significant length][31-byte zero-padded AD].
func advDataBlock(ad []byte) []byte {
	p := make([]byte, 32)
	p[0] = byte(len(ad))
	copy(p[1:], ad)
	return p
}

// parseCommandComplete inspects a raw event packet and, if it is a Command
// Complete for wantOpcode, returns its status byte. matched is false for any
// other event (the caller keeps reading).
func parseCommandComplete(pkt []byte, wantOpcode uint16) (status byte, matched bool, ok bool) {
	// [0]=0x04 evt, [1]=evtcode, [2]=plen, [3]=num_cmd_pkts, [4:6]=opcode, [6]=status
	if len(pkt) < 7 || pkt[0] != pktEvent || pkt[1] != evtCommandComplete {
		return 0, false, true
	}
	op := binary.LittleEndian.Uint16(pkt[4:6])
	if op != wantOpcode {
		return 0, false, true
	}
	return pkt[6], true, true
}
