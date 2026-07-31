// Command splinterd is a BLE privacy / anti-tracking decoy for Linux — a native
// port of the splinter ESP32 firmware concept, targeting the ClockworkPi
// uConsole (CM4). It fabricates a churning crowd of plausible, non-connectable
// fake BLE devices so real devices don't stand out to a scanner in a space you
// control.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/zachsis/splinter-uconnectcm4/internal/config"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	cfg, err := config.Parse("splinterd", os.Args[1:], os.Stderr)
	switch {
	case errors.Is(err, config.ErrVersion):
		fmt.Println("splinterd", version)
		return
	case errors.Is(err, flag.ErrHelp):
		return // usage already printed by the flag package
	case err != nil:
		fmt.Fprintln(os.Stderr, "splinterd:", err)
		os.Exit(2)
	}

	// The run loop, HCI transport wiring, signal handling, and clean bluetoothd
	// hand-back are added in the daemon-lifecycle milestone.
	fmt.Printf("splinterd configured: %+v\n", cfg)
}
