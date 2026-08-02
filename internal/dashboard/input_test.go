package dashboard

import (
	"testing"

	"github.com/zachsis/helmofhades/internal/verify"
)

// fake controllers for input dispatch tests.
type fakeBroadcast struct{ paused bool }

func (f *fakeBroadcast) Paused() bool { return f.paused }
func (f *fakeBroadcast) Toggle() bool { f.paused = !f.paused; return f.paused }

type fakeDebug struct{ on bool }

func (f *fakeDebug) Enabled() bool { return f.on }
func (f *fakeDebug) Toggle() bool  { f.on = !f.on; return f.on }
func (f *fakeDebug) Path() string  { return "x.log" }

type fakeTrackers struct{ on bool }

func (f *fakeTrackers) Enabled() bool { return f.on }
func (f *fakeTrackers) Toggle() bool  { f.on = !f.on; return f.on }

func newTestModel(crowd, learned int) *Model {
	m := New("paced", 100, 200)
	for i := 0; i < crowd; i++ {
		m.Decoy([6]byte{byte(i)}, uint16(0x1000+i), "")
	}
	devs := make([]verify.Observation, learned)
	for i := range devs {
		devs[i] = verify.Observation{MAC: [6]byte{byte(i)}, RSSI: int8(-i)}
	}
	m.SetLearned(devs)
	return m
}

func TestHandleSingleKeys(t *testing.T) {
	m := newTestModel(0, 0)
	bc := &fakeBroadcast{}
	dbg := &fakeDebug{}
	tr := &fakeTrackers{}
	quit := false
	c := Controls{Broadcast: bc, Debug: dbg, Trackers: tr, OnQuit: func() { quit = true }}

	if handleSingle(' ', m, c); !bc.paused || !m.Snapshot().Paused {
		t.Errorf("space should pause")
	}
	if handleSingle(' ', m, c); bc.paused {
		t.Errorf("space again should resume")
	}
	if handleSingle('D', m, c); !dbg.on || !m.Snapshot().DebugOn {
		t.Errorf("D should enable debug")
	}
	if handleSingle('s', m, c); !tr.on {
		t.Errorf("s should toggle trackers")
	}
	if handleSingle('?', m, c); !m.HelpOpen() {
		t.Errorf("? should open help")
	}
	if !handleSingle('q', m, c) || !quit {
		t.Errorf("q should quit and call OnQuit")
	}
}

func TestHandleCSIScroll(t *testing.T) {
	m := newTestModel(0, 20) // 20 learned devices
	m.CycleFocus()           // focus -> learned
	m.SetScrollBounds(5)     // viewport 5 => max offset 15

	// "ESC [ B" (down) x3
	if handleInput([]byte{0x1b, '[', 'B', 0x1b, '[', 'B', 0x1b, '[', 'B'}, m, Controls{}); m.Snapshot().LearnScroll != 3 {
		t.Fatalf("down x3 => offset 3, got %d", m.Snapshot().LearnScroll)
	}
	// PgDn "ESC [ 6 ~" advances a page (viewport-1 = 4).
	handleInput([]byte{0x1b, '[', '6', '~'}, m, Controls{})
	if got := m.Snapshot().LearnScroll; got != 7 {
		t.Fatalf("PgDn => 3+4=7, got %d", got)
	}
	// End "ESC [ F" jumps to max (15).
	handleInput([]byte{0x1b, '[', 'F'}, m, Controls{})
	if got := m.Snapshot().LearnScroll; got != 15 {
		t.Fatalf("End => max 15, got %d", got)
	}
	// Up past top clamps at 0.
	for i := 0; i < 30; i++ {
		handleInput([]byte{0x1b, '[', 'A'}, m, Controls{})
	}
	if got := m.Snapshot().LearnScroll; got != 0 {
		t.Fatalf("up past top => 0, got %d", got)
	}
}

func TestHandleInputLoneEscNoHang(t *testing.T) {
	m := newTestModel(0, 5)
	// A lone ESC (no following '[') must be treated as standalone and not
	// consume/misparse the following real key.
	if _, quit := handleInput([]byte{0x1b, 'q'}, m, Controls{OnQuit: func() {}}); !quit {
		t.Fatalf("lone ESC then q should still register the quit")
	}
}

// TestHandleInputSplitEscape is the regression guard for the peer-review flag:
// an escape sequence split across reads must NOT leak its bytes to single-key
// handlers (a split Up-arrow's trailing 'A'/'D' would otherwise fire apple/debug).
func TestHandleInputSplitEscape(t *testing.T) {
	m := newTestModel(0, 20)
	m.CycleFocus()
	m.SetScrollBounds(5)
	dbg := &fakeDebug{}
	c := Controls{Debug: dbg}

	// First read ends on a bare ESC — it must be carried, not acted on.
	consumed, _ := handleInput([]byte{'j', 0x1b}, m, c) // 'j' scrolls down 1, ESC carried
	if consumed != 1 {
		t.Fatalf("bare trailing ESC should be carried (consumed=1), got %d", consumed)
	}
	if m.Snapshot().LearnScroll != 1 {
		t.Fatalf("the 'j' before ESC should have scrolled once")
	}
	// Next read supplies the continuation "[A" (Up). Caller prepends the carry.
	consumed, _ = handleInput([]byte{0x1b, '[', 'A'}, m, c)
	if consumed != 3 {
		t.Fatalf("completed Up sequence should consume 3, got %d", consumed)
	}
	if dbg.on {
		t.Fatalf("a split arrow must never toggle debug logging")
	}
	if m.Snapshot().LearnScroll != 0 {
		t.Fatalf("Up should have scrolled back to 0, got %d", m.Snapshot().LearnScroll)
	}
}

func TestScrollBoundsClampOnShrink(t *testing.T) {
	m := newTestModel(3, 0) // 3 crowd rows
	m.SetScrollBounds(1)    // max offset 2
	m.Scroll(5)             // clamps to 2
	if m.Snapshot().CrowdScroll != 2 {
		t.Fatalf("expected clamp to 2, got %d", m.Snapshot().CrowdScroll)
	}
	// Content shrinks below the offset -> next bounds update re-clamps.
	m.Decoy([6]byte{9}, 0, "") // still same distinct-count-ish; force recompute
	m.SetScrollBounds(10)      // viewport bigger than content => max 0
	if m.Snapshot().CrowdScroll != 0 {
		t.Fatalf("expected re-clamp to 0, got %d", m.Snapshot().CrowdScroll)
	}
}
