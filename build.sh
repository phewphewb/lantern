#!/usr/bin/env bash
set -euo pipefail

SKIP_TESTS="${1:-}"
ROOT="$(cd "$(dirname "$0")" && pwd)"
BIN="$ROOT/bin"
CMDS=(init discover validate setup sync certs ls)

if [[ "$SKIP_TESTS" != "--skip-tests" ]]; then
  echo "Running tests..."
  go test ./...
fi

echo "Building binaries to $BIN/"
mkdir -p "$BIN"

for cmd in "${CMDS[@]}"; do
  echo "  lantern-$cmd"
  go build -o "$BIN/lantern-$cmd" "$ROOT/cmd/$cmd"
done

echo "Done."
