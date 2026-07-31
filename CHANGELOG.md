# Changelog

## v0.1.0 — 2026-07-31

Initial release. A native Linux/Go port of the [splinter](https://github.com/JakeSwiz/splinter)
BLE-decoy concept, targeting the ClockworkPi uConsole (CM4).

- Non-connectable BLE decoy: each cycle mints a fresh random-static MAC and
  spec-valid vendor manufacturer data; never emits Apple / Microsoft / Google
  Fast Pair shapes (no bystander pairing popups).
- Drives the onboard controller via an exclusive `HCI_CHANNEL_USER` socket, with
  its own controller bring-up (required for advertising on the user channel), and
  hands the adapter cleanly back to `bluetoothd` on every exit path. Paced and
  benchmark modes.
- `systemd` unit running unprivileged via ambient `CAP_NET_RAW`/`CAP_NET_ADMIN`,
  plus an installer.
- `splinter-verify` parity harness (scan-side crowd + guardrail assertions).
- Cross-compiles to a static aarch64 binary. Validated on the uConsole
  (Cypress CYW43455): ~10 decoys/sec, zero cycle failures.

Note: the CYW43455 lacks LE Extended Advertising, so this reproduces the classic
single-fast-rotated-advertiser behavior rather than the ESP32-C5's concurrent
instances — equivalent observable crowd.
