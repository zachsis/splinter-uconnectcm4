// Command splinterd is a BLE privacy / anti-tracking decoy for Linux — a native
// port of the splinter ESP32 firmware concept, targeting the ClockworkPi
// uConsole (CM4). It fabricates a churning crowd of plausible, non-connectable
// fake BLE devices so real devices don't stand out to a scanner in a space you
// control.
//
// The bootstrap milestone provides a compilable entrypoint with --version; the
// full flag surface and run loop are wired in later milestones.
package main

import (
	"fmt"
	"os"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	for _, a := range os.Args[1:] {
		switch a {
		case "--version", "-version":
			fmt.Println("splinterd", version)
			return
		case "--help", "-h", "-help":
			usage(os.Stdout)
			return
		}
	}
	usage(os.Stderr)
}

func usage(w *os.File) {
	fmt.Fprintln(w, "splinterd — BLE privacy decoy (uConsole/Linux)")
	fmt.Fprintln(w, "usage: splinterd [--version] [--help]")
	fmt.Fprintln(w, "(the run loop and tuning flags are added in later milestones)")
}
