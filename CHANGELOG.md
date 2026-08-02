# Changelog

## Unreleased

### Added
- **Learn mode** (dashboard `l` key): passively scans nearby BLE (`--learn-window`,
  default 15s) and weights decoy selection toward the observed vendor mix — the
  fakes blend into the devices actually around you. Never replays real
  addresses/payloads. (#17)
- **htop-style crowd table**: the vendor "crowd" is now a multi-row table (name ·
  bar · count), sorted, height-adaptive. (#23)
- **Taller rate graph**: the rate history renders as a ~4-row block graph (~32
  vertical levels), height-adaptive; fails stays a compact sparkline. (#22)

### Added
- **Dashboard hotkeys** (`--dashboard`): `+`/`-` adjust the live rate (clamped to
  the visibility floor), `t` cycles the color theme, `q`/Ctrl-C quits. (#16)
- **Color themes** for the dashboard: `--theme matrix|amber|neon|mono` + live `t`
  cycling; honors `NO_COLOR` / non-color terminals (falls back to mono). (#18)
- Expanded the decoy vendor table to 26 entries across 6 device categories. (#15)

### Added
- **`--dashboard`**: a live, mtr-style in-place terminal UI (counters, rate/fails
  sparklines, current fake identity, vendor-crowd histogram) instead of scrolling
  log lines. Auto-falls-back to line logging when stdout isn't a TTY (piped /
  systemd). Hand-rolled ANSI — no new dependencies. (#13)
- **`--dense`**: self-calibrating maximum-visibility mode. Probes the controller
  (~10 s) for the fastest advertising interval it sustains, then advertises at
  `rotate-ms = --adverts-per-id × adv-ms` so each identity actually transmits
  before rotating. `--aggressive` allows the lowest interval; `--adverts-per-id`
  (default 2) tunes dwell. (#11)
- **Bounded self-calibrating `--benchmark`**: now probes advertising intervals for
  `--duration` (default 10 s) and prints the recommended visibility-optimal
  `(--adv-ms, --rotate-ms)`, then exits. (#10)
- Guardrail: warns when `--rotate-ms < --adv-ms` (decoys rotate before they
  transmit and aren't visible to scanners).

### Changed
- **`--benchmark` semantics**: was an unbounded max-HCI-rate flood; is now a
  bounded calibration + recommendation. The old flood maximized command rate but
  broadcast almost nothing observable (each identity was replaced before it
  transmitted).

### Notes
- On the uConsole's CYW43455, non-connectable advertising is floored at 100 ms;
  `--dense` calibrates to `adv-ms=100, rotate-ms=200` (~5 visible decoys/sec).

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
