#!/usr/bin/env bash
# lantern-local.sh — run lantern subcommands from the local bin/ directory
# Usage: ./lantern-local.sh <subcommand> [args...]
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
BIN="$ROOT/bin"

CMD="${1:-}"
shift || true

case "$CMD" in
  init)     exec "$BIN/lantern-init"     "$@" ;;
  discover) exec "$BIN/lantern-discover" "$@" ;;
  validate) exec "$BIN/lantern-validate" "$@" ;;
  setup)    exec "$BIN/lantern-setup"    "$@" ;;
  sync)     exec "$BIN/lantern-sync"     "$@" ;;
  certs)    exec "$BIN/lantern-certs"    "$@" ;;
  ls)       exec "$BIN/lantern-ls"       "$@" ;;
  *)
    echo "Usage: lantern-local.sh {init|discover|validate|setup|sync|certs|ls}" >&2
    exit 1
    ;;
esac
