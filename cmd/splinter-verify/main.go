// Command splinter-verify passively scans for BLE advertisements and reports
// whether the observed crowd matches splinterd's expected decoy behavior:
// enough distinct devices, a spread of vendor IDs, and none of the
// popup-triggering formats splinterd must never emit. Run it from a SECOND
// BLE-capable machine while splinterd is advertising (the uConsole's own adapter
// is busy). Exits non-zero if the parity check fails.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/zachsis/splinter-uconnectcm4/internal/verify"
)

func main() {
	hci := flag.Int("hci", 0, "HCI device index to scan with")
	window := flag.Duration("window", 10*time.Second, "scan window")
	expected := flag.Int("expected-rotate-ms", 250, "splinterd's --rotate-ms (0 = benchmark; skip rate floor)")
	flag.Parse()

	obs, err := scan(*hci, *window)
	if err != nil {
		fmt.Fprintln(os.Stderr, "splinter-verify:", err)
		os.Exit(2)
	}
	result := verify.Analyze(obs, *window, *expected)
	fmt.Print(result.String())
	if !result.Pass {
		os.Exit(1)
	}
}
