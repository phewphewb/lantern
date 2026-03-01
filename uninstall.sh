#!/usr/bin/env bash
# uninstall.sh — remove lantern binaries (and optionally system configs)
# Usage: sudo ./uninstall.sh [--purge]
#
#   --purge  also removes nginx/dnsmasq configs, TLS certs, cron job,
#            backups, and log file written by 'lantern setup'
set -euo pipefail

PURGE="${1:-}"
INSTALL_BIN="/usr/local/lib/lantern/bin"
INSTALL_WRAPPER="/usr/local/bin/lantern"
CMDS=(init discover validate setup sync certs ls)

if [[ "$EUID" -ne 0 ]]; then
  echo "Error: uninstall.sh must be run as root (sudo ./uninstall.sh)" >&2
  exit 1
fi

# ── 1. Remove binaries ────────────────────────────────────────────────────────
echo "==> Removing binaries from $INSTALL_BIN..."
for cmd in "${CMDS[@]}"; do
  target="$INSTALL_BIN/lantern-$cmd"
  if [[ -f "$target" ]]; then
    rm -f "$target"
    echo "    removed $target"
  fi
done
if [[ -d "$INSTALL_BIN" ]] && [[ -z "$(ls -A "$INSTALL_BIN" 2>/dev/null)" ]]; then
  rm -rf "/usr/local/lib/lantern"
  echo "    removed /usr/local/lib/lantern/"
fi

# ── 2. Remove wrapper ─────────────────────────────────────────────────────────
if [[ -f "$INSTALL_WRAPPER" ]]; then
  rm -f "$INSTALL_WRAPPER"
  echo "==> Removed wrapper $INSTALL_WRAPPER"
fi

if [[ "$PURGE" != "--purge" ]]; then
  echo ""
  echo "Done. Lantern binaries removed."
  echo "System configs (nginx, dnsmasq, certs, cron) were left in place."
  echo "To also remove those, run: sudo ./uninstall.sh --purge"
  exit 0
fi

# ── 3. Remove nginx configs and TLS certs (--purge) ──────────────────────────
NGINX_DIR="/etc/nginx/sites-enabled"
CERT_DIR="/etc/ssl/local"
CERTS_TO_REMOVE=()

echo "==> Removing nginx configs..."
for conf in "$NGINX_DIR"/*.conf; do
  [[ -f "$conf" ]] || continue
  if grep -q "ssl_certificate $CERT_DIR/" "$conf" 2>/dev/null; then
    # Collect cert paths before removing the config
    while IFS= read -r line; do
      crt="$(echo "$line" | awk '{print $2}' | tr -d ';')"
      [[ -n "$crt" ]] && CERTS_TO_REMOVE+=("$crt")
    done < <(grep "ssl_certificate " "$conf" 2>/dev/null || true)
    rm -f "$conf"
    echo "    removed $conf"
  fi
done

echo "==> Removing TLS certificates..."
for crt in "${CERTS_TO_REMOVE[@]+"${CERTS_TO_REMOVE[@]}"}"; do
  key="${crt%.crt}.key"
  [[ -f "$crt" ]] && rm -f "$crt" && echo "    removed $crt"
  [[ -f "$key" ]] && rm -f "$key" && echo "    removed $key"
done

# ── 4. Remove dnsmasq config (--purge) ───────────────────────────────────────
DNSMASQ_CONF="/etc/dnsmasq.d/local-services.conf"
if [[ -f "$DNSMASQ_CONF" ]]; then
  rm -f "$DNSMASQ_CONF"
  echo "==> Removed $DNSMASQ_CONF"
fi

# ── 5. Restart services (--purge) ────────────────────────────────────────────
echo "==> Restarting nginx and dnsmasq..."
if command -v systemctl &>/dev/null; then
  systemctl restart nginx   2>/dev/null || true
  systemctl restart dnsmasq 2>/dev/null || true
else
  service nginx restart   2>/dev/null || true
  service dnsmasq restart 2>/dev/null || true
fi

# ── 6. Remove cron entry (--purge) ───────────────────────────────────────────
echo "==> Removing cron job..."
current="$(crontab -l 2>/dev/null || true)"
if echo "$current" | grep -q "managed-by:lantern"; then
  echo "$current" | grep -v "managed-by:lantern" | crontab -
  echo "    cron entry removed"
else
  echo "    no cron entry found"
fi

# ── 7. Remove backups and log (--purge) ───────────────────────────────────────
BACKUP_ROOT="/var/backups/lantern"
if [[ -d "$BACKUP_ROOT" ]]; then
  rm -rf "$BACKUP_ROOT"
  echo "==> Removed backups at $BACKUP_ROOT"
fi

LOG_FILE="/var/log/lantern.log"
if [[ -f "$LOG_FILE" ]]; then
  rm -f "$LOG_FILE"
  echo "==> Removed log file $LOG_FILE"
fi

echo ""
echo "Done. Lantern and all system configs have been removed."
