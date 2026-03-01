#!/usr/bin/env bash
# install.sh — build and install lantern on the local machine
# Usage: sudo ./install.sh [--skip-tests]
set -euo pipefail

SKIP_TESTS="${1:-}"
ROOT="$(cd "$(dirname "$0")" && pwd)"
BIN="$ROOT/bin"
INSTALL_BIN="/usr/local/lib/lantern/bin"
INSTALL_WRAPPER="/usr/local/bin/lantern"

if [[ "$EUID" -ne 0 ]]; then
  echo "Error: install.sh must be run as root (sudo ./install.sh)" >&2
  exit 1
fi

# ── 1. Build ──────────────────────────────────────────────────────────────────
BUILD_ARGS=()
[[ "$SKIP_TESTS" == "--skip-tests" ]] && BUILD_ARGS+=(--skip-tests)
"$ROOT/build.sh" "${BUILD_ARGS[@]}"

# ── 2. Install ────────────────────────────────────────────────────────────────
CMDS=(init discover validate setup sync certs ls)
echo "==> Installing to $INSTALL_BIN..."
mkdir -p "$INSTALL_BIN"
for cmd in "${CMDS[@]}"; do
  install -m 755 "$BIN/lantern-$cmd" "$INSTALL_BIN/lantern-$cmd"
done

echo "==> Installing wrapper to $INSTALL_WRAPPER..."
install -m 755 "$ROOT/lantern.sh" "$INSTALL_WRAPPER"

# ── 4. Smoke-test ─────────────────────────────────────────────────────────────
echo "==> Verifying install..."
lantern 2>&1 | grep -q "Usage:" && echo "    lantern wrapper OK" || true

echo ""
echo "Done. Run: sudo lantern init"
