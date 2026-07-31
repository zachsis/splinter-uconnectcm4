#!/usr/bin/env sh
# Install splinterd + its systemd unit on the uConsole. Run ON THE DEVICE as root:
#   sudo sh install.sh [path-to-arm64-binary]
# Default binary name is the one produced by `make build-uconsole` and scp'd by
# `make install-uconsole` (splinterd-arm64).
set -eu

BIN="${1:-splinterd-arm64}"
UNIT="${UNIT:-splinterd.service}"

if [ "$(id -u)" -ne 0 ]; then
	echo "must run as root (use sudo)" >&2
	exit 1
fi
if [ ! -f "$BIN" ]; then
	echo "binary not found: $BIN" >&2
	exit 1
fi
if [ ! -f "$UNIT" ]; then
	echo "unit not found: $UNIT (scp packaging/splinterd.service alongside this script)" >&2
	exit 1
fi

# Dedicated unprivileged system user.
if ! id splinter >/dev/null 2>&1; then
	useradd --system --no-create-home --shell /usr/sbin/nologin splinter
	echo "created system user 'splinter'"
fi

install -m 0755 "$BIN" /usr/local/bin/splinterd
install -m 0644 "$UNIT" /etc/systemd/system/splinterd.service
systemctl daemon-reload

echo "installed /usr/local/bin/splinterd and splinterd.service"
echo "start with:  sudo systemctl enable --now splinterd"
echo "logs:        journalctl -u splinterd -f"
echo
echo "NOTE: while running, splinterd takes over the Bluetooth adapter. It restores"
echo "it on stop; if BT ever misbehaves after a crash: sudo systemctl restart bluetooth"
