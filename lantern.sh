#!/usr/bin/env bash
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
CMD="${1:-}"
shift || true

case "$CMD" in
  init)     exec "$DIR/bin/lantern-init"     "$@" ;;
  discover) exec "$DIR/bin/lantern-discover" "$@" ;;
  validate) exec "$DIR/bin/lantern-validate" "$@" ;;
  setup)    exec "$DIR/bin/lantern-setup"    "$@" ;;
  sync)     exec "$DIR/bin/lantern-sync"     "$@" ;;
  certs)    exec "$DIR/bin/lantern-certs"    "$@" ;;
  ls)       exec "$DIR/bin/lantern-ls"       "$@" ;;
  *)
    echo "Usage: lantern {init|discover|validate|setup|sync|certs|ls}" >&2
    exit 1
    ;;
esac
