package hci

import "testing"

func TestAdvIntervalUnits(t *testing.T) {
	cases := map[uint16]uint16{20: 32, 50: 80, 100: 160, 130: 208, 250: 400}
	for ms, want := range cases {
		if got := advIntervalUnits(ms); got != want {
			t.Errorf("advIntervalUnits(%d) = %d, want %d", ms, got, want)
		}
	}
}

func TestCmdPacketFraming(t *testing.T) {
	// HCI_Reset: OGF 0x03, OCF 0x0003 -> opcode 0x0C03, no params.
	pkt := cmdPacket(ogfHostCtl, ocfReset, nil)
	want := []byte{pktCommand, 0x03, 0x0C, 0x00}
	if string(pkt) != string(want) {
		t.Fatalf("reset packet = % x, want % x", pkt, want)
	}
	// LE Set Advertise Enable with 1 param.
	pkt = cmdPacket(ogfLE, ocfSetAdvEnable, []byte{0x01})
	want = []byte{pktCommand, 0x0A, 0x20, 0x01, 0x01}
	if string(pkt) != string(want) {
		t.Fatalf("adv-enable packet = % x, want % x", pkt, want)
	}
}

func TestAdvParamsBlock(t *testing.T) {
	p := advParamsBlock(100, 130, AdvNonconnInd)
	if len(p) != 15 {
		t.Fatalf("params len = %d, want 15", len(p))
	}
	// min=160 units (0x00A0), max=208 units (0x00D0), little-endian.
	if p[0] != 0xA0 || p[1] != 0x00 || p[2] != 0xD0 || p[3] != 0x00 {
		t.Errorf("interval bytes = % x", p[0:4])
	}
	if p[4] != AdvNonconnInd {
		t.Errorf("adv type = %#02x, want %#02x", p[4], AdvNonconnInd)
	}
	if p[5] != 0x01 {
		t.Errorf("own addr type = %#02x, want 0x01 (random)", p[5])
	}
	if p[13] != 0x07 || p[14] != 0x00 {
		t.Errorf("channel map/filter = %#02x/%#02x, want 0x07/0x00", p[13], p[14])
	}
}

func TestAdvDataBlock(t *testing.T) {
	ad := []byte{0x02, 0x01, 0x06}
	p := advDataBlock(ad)
	if len(p) != 32 {
		t.Fatalf("adv data block len = %d, want 32", len(p))
	}
	if p[0] != 3 {
		t.Errorf("significant length = %d, want 3", p[0])
	}
	if p[1] != 0x02 || p[2] != 0x01 || p[3] != 0x06 {
		t.Errorf("adv bytes = % x", p[1:4])
	}
	for i := 4; i < 32; i++ {
		if p[i] != 0 {
			t.Fatalf("expected zero padding at %d", i)
		}
	}
}

func TestParseCommandComplete(t *testing.T) {
	want := opcode(ogfLE, ocfSetAdvEnable) // 0x200A
	// A Command Complete for a DIFFERENT opcode -> not matched, keep reading.
	other := []byte{pktEvent, evtCommandComplete, 0x04, 0x01, 0x03, 0x0C, 0x00}
	if _, matched, ok := parseCommandComplete(other, want); matched || !ok {
		t.Fatalf("unrelated event should not match (matched=%v ok=%v)", matched, ok)
	}
	// Matching, status success.
	good := []byte{pktEvent, evtCommandComplete, 0x04, 0x01, 0x0A, 0x20, 0x00}
	status, matched, _ := parseCommandComplete(good, want)
	if !matched || status != 0 {
		t.Fatalf("expected match status 0, got matched=%v status=%#02x", matched, status)
	}
	// Matching, failure status.
	bad := []byte{pktEvent, evtCommandComplete, 0x04, 0x01, 0x0A, 0x20, 0x12}
	if status, matched, _ := parseCommandComplete(bad, want); !matched || status != 0x12 {
		t.Fatalf("expected match status 0x12, got matched=%v status=%#02x", matched, status)
	}
}

func TestParseMgmtCmdComplete(t *testing.T) {
	// mgmt CMD_COMPLETE (0x0001), index 0, len 3, cmd_op=SET_POWERED, status 0, settings byte
	frame := []byte{0x01, 0x00, 0x00, 0x00, 0x03, 0x00, 0x05, 0x00, 0x00}
	status, matched, _ := parseMgmtCmdComplete(frame, mgmtOpSetPowered)
	if !matched || status != 0 {
		t.Fatalf("expected mgmt match status 0, got matched=%v status=%#02x", matched, status)
	}
	// Wrong opcode -> not matched.
	frame[6] = 0x04 // READ_INFO
	if _, matched, _ := parseMgmtCmdComplete(frame, mgmtOpSetPowered); matched {
		t.Fatalf("mgmt should not match different opcode")
	}
}
