# Verifying parity

`splinter-verify` confirms that a scanner actually sees the decoy crowd
splinterd is meant to produce — the objective version of "watch it in nRF
Connect."

## Why a second device

While splinterd runs, it holds the uConsole's adapter exclusively, so the
uConsole can't scan itself. Run `splinter-verify` from a **second** BLE-capable
Linux machine (another Pi, a laptop). It also takes exclusive control of *its*
adapter for the scan window and hands it back afterward. It needs `CAP_NET_RAW`
+ `CAP_NET_ADMIN` (run with `sudo`, or `setcap 'cap_net_raw,cap_net_admin+eip'`).

## Run

```bash
# On the uConsole:
sudo systemctl start splinterd          # or: sudo splinterd --rotate-ms 250

# On a second Linux machine, build and scan:
GOOS=linux GOARCH=arm64 go build ./cmd/splinter-verify   # (or your host arch)
sudo ./splinter-verify --window 10s --expected-rotate-ms 250
```

Pass `--expected-rotate-ms` the same value splinterd runs with. In `--benchmark`
mode use `--expected-rotate-ms 0` to skip the rate floor and check only the
vendor-ID spread and the guardrails.

## What it checks

- **Distinct-MAC rate** ≥ 50% of the theoretical `1000/rotate-ms` (passive
  scanning never captures every advert, so a 50% floor keeps the check honest).
- **Vendor-ID spread** — a variety of company IDs, not all identical.
- **Guardrails (hard failures):** no advert carries an Apple (`0x004C`) or
  Microsoft (`0x0006`) company ID, no Google Fast Pair (`0xFE2C`) service data,
  and every decoy is non-connectable.

It prints a `parity PASS/FAIL` summary with the per-vendor histogram and exits
non-zero on failure.

## Control run

Run it with splinterd **stopped** to confirm it's measuring splinterd and not
ambient traffic — you should see ~0 decoys and a rate-floor failure.
