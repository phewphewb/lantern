# Router Configurator — Behaviour Specification

## Overview

A CLI tool that automates local network HTTPS configuration. It discovers
services on the local network, generates TLS certificates, configures a
reverse proxy, and sets up local DNS — enabling access to local services
via domain names over HTTPS instead of raw IP addresses.

---

## Commands

### `discover`

Scans the local network to identify known services and writes their IPs
into `network.yaml`.

**Flow:**
1. Detects the machine's local subnet (e.g. `192.168.2.0/24`) from its
   active network interface
2. Concurrently probes all 254 possible hosts
3. For each active host, hits known service API endpoints to fingerprint
   the service
4. Presents results to the user for confirmation
5. Writes discovered IPs into `network.yaml`

**Expected output:**
```
Scanning 192.168.2.0/24...
Found 12 active hosts

Identified services:
  ✓ frigate    192.168.2.10  (port 5000)
  ✓ truenas    192.168.2.20  (port 80)
  ✓ mainsail   192.168.2.30  (port 80, moonraker: 7125)

Could not identify:
  ? 192.168.2.50
  ? 192.168.2.100

Write to network.yaml? [y/N]
```

**Error cases:**
- No active hosts found → suggest checking network interface
- A service is not found → warn and list unidentified IPs; user edits
  `network.yaml` by hand, then runs `validate` to confirm before setup
- `network.yaml` already exists with a **different IP** for a known service →
  warn and show the diff, require explicit confirmation before overwriting

**Merge behaviour (when `network.yaml` already exists):**
- Newly discovered IPs replace existing ones only after user confirms
- Fields not touched by discovery (e.g. `websocket`, `cert_warn_days`)
  are always preserved

---

### `setup`

Reads `network.yaml`, backs up existing config, then configures
certificates, reverse proxy, and DNS.

Must be run with `sudo`. `discover` and `validate` do not require root.

**Flags:**
- `--dry-run` — print every action that would be taken without executing
  anything. No files written, no services restarted, no backup created.
- `--config string` — path to config file (default: `network.yaml`)
- `--verbose` — show debug output

**Flow:**
1. Reads and validates `network.yaml`
2. Checks all service IPs are reachable
3. Checks required tools are installed (`nginx`, `dnsmasq`, `mkcert`)
4. Checks existing certs for expiry — warns if any expire within
   `cert_warn_days` (default 30)
5. **Creates a timestamped backup** of existing nginx and dnsmasq configs
6. Installs mkcert CA if not already installed
7. Generates TLS certificates for each service domain
8. Writes Nginx config files per service
9. Writes dnsmasq config
10. Restarts `nginx` and `dnsmasq`
11. Prints post-setup instructions for Bell router and client devices

**Expected output:**
```
Reading network.yaml...
  ✓ frigate    192.168.2.10:5000  → frigate.home
  ✓ truenas    192.168.2.20:80    → truenas.home
  ✓ mainsail   192.168.2.30:80    → mainsail.home

Checking tools...
  ✓ nginx      installed
  ✓ dnsmasq    installed
  ✓ mkcert     installed

Backing up existing config...
  → /var/backups/router-configurator/2024-01-15-143022/

Generating certificates...
  ✓ frigate.home
  ✓ truenas.home
  ✓ mainsail.home

Writing Nginx config...   ✓
Writing dnsmasq config... ✓
Restarting services...    ✓

Setup complete.

─── Next steps ────────────────────────────────────────────
1. Point your Bell router DNS to this machine:
     Router: http://192.168.2.1 → Advanced → DNS → Primary DNS
     Set to: 192.168.2.10

2. Install the CA certificate on each client device:
     CA file: /home/user/.local/share/mkcert/rootCA.pem

   Windows : double-click → install to "Trusted Root CAs"
   macOS   : Keychain Access → import → set to Always Trust
   iOS     : download file → Settings → trust profile
   Android : Settings → Security → Install certificate
───────────────────────────────────────────────────────────
```

**`--dry-run` expected output:**
```
[DRY RUN] Reading network.yaml...
  ✓ frigate    192.168.2.10:5000  → frigate.home
  ...

[DRY RUN] Would back up existing configs to:
  /var/backups/router-configurator/2024-01-15-143022/

[DRY RUN] Would run:
  mkcert -install
  mkcert -cert-file /etc/ssl/local/frigate.home.crt ... frigate.home
  ...

[DRY RUN] Would write:
  /etc/nginx/sites-enabled/frigate.home.conf
  /etc/nginx/sites-enabled/truenas.home.conf
  /etc/nginx/sites-enabled/mainsail.home.conf
  /etc/dnsmasq.d/local-services.conf

[DRY RUN] Would restart: nginx, dnsmasq

No changes made.
```

**Error cases:**
- Required tool not installed → print install command, exit before backup
- Service IP unreachable → warn, ask whether to continue
- Any step fails after backup was created → **auto-restore from backup**,
  print restored paths, print the error, exit with non-zero code
- Auto-restore also fails → print both errors and the backup path so the
  user can restore manually

---

### `ls`

Lists every file and directory the tool manages or touches, with their
current status on disk. Does not require root. Read-only.

Useful for knowing what exists, where certs are, and how many backups
have accumulated.

**Flags:**
- `--config string` — path to config file (default: `network.yaml`)

**Expected output:**
```
Managed paths:

Config:
  ✓ /home/user/network.yaml

Certificates:
  ✓ /etc/ssl/local/frigate.home.crt      expires 2026-01-15  (320 days)
  ✓ /etc/ssl/local/truenas.home.crt      expires 2026-01-15  (320 days)
  ! /etc/ssl/local/mainsail.home.crt     expires 2024-02-10  (22 days)
  ✓ /home/user/.local/share/mkcert/rootCA.pem   (CA cert)

Nginx config:
  ✓ /etc/nginx/sites-enabled/frigate.home.conf
  ✓ /etc/nginx/sites-enabled/truenas.home.conf
  - /etc/nginx/sites-enabled/mainsail.home.conf  (not yet created)

DNS config:
  ✓ /etc/dnsmasq.d/local-services.conf

Backups:
  /var/backups/router-configurator/
  ✓ 2024-01-15-143022/   3 files   4.2 KB
  ✓ 2024-01-10-091500/   3 files   4.1 KB
```

**Status indicators:**
- `✓` — file exists
- `!` — file exists but needs attention (e.g. cert expiring soon)
- `-` — file does not exist yet (setup has not been run, or partially run)

**Error cases:**
- `network.yaml` not found → print message, list only the paths the tool
  would manage based on defaults

---

### `certs`

Checks the status of all TLS certificates and optionally renews them.
Does not require root.

**Subcommands:**
- `certs status` — show expiry status for all service certs (default if no subcommand given)
- `certs renew` — regenerate certs for all services expiring within `cert_warn_days`

**Flags:**
- `--config string` — path to config file (default: `network.yaml`)
- `--all` — renew all certs regardless of expiry (use with `renew`)
- `--dry-run` — show what would be renewed without doing it

**Expected output (`certs status`):**
```
Certificate status:
  ✓ frigate.home    expires 2026-01-15  (320 days)
  ✓ truenas.home    expires 2026-01-15  (320 days)
  ! mainsail.home   expires 2024-02-10  (22 days)  ← expiring soon
```

**Expected output (`certs renew`):**
```
Renewing expiring certificates (threshold: 30 days)...
  - frigate.home    ok, skipped (320 days remaining)
  - truenas.home    ok, skipped (320 days remaining)
  ✓ mainsail.home   renewed → expires 2026-02-10

Done. Run setup to reload nginx with the new certificates.
```

**Error cases:**
- Cert file not found for a service → warn, suggest running `setup` first
- `mkcert` not installed → print install command, exit

---

### `validate`

Checks a `network.yaml` file for correctness without touching the system.
Does not require root.

**Flags:**
- `--config string` — path to config file (default: `network.yaml`)
- `--ping` — also verify each service IP is reachable on its port

**Expected output (passing):**
```
Validating network.yaml...
  ✓ version supported (1)
  ✓ domain_suffix present
  ✓ proxy_ip valid (192.168.2.10)
  ✓ 3 services defined, no duplicate names
  ✓ all ports in valid range

All checks passed.
```

**Expected output (with `--ping`):**
```
Validating network.yaml...
  ...
  ✓ 192.168.2.10:5000  reachable  (frigate)
  ✓ 192.168.2.20:80    reachable  (truenas)
  ✓ 192.168.2.30:80    reachable  (mainsail)

All checks passed.
```

**Expected output (failing):**
```
Validating network.yaml...
  ✓ version supported (1)
  ✗ mainsail: port 0 is not valid (must be 1–65535)
  ✗ truenas: ip "192.168.2.999" is not a valid IPv4 address

2 errors found. Fix network.yaml and run validate again.
```

**Checks performed:**
- YAML parses without error
- `version` field is present and equals a supported value
- `domain_suffix` and `proxy_ip` are present and non-empty
- `proxy_ip` is a valid IPv4 address
- At least one service is defined
- Each service has `name`, `ip`, and `port`
- Each `ip` is a valid IPv4 address
- Each `port` is in range 1–65535
- No two services share the same `name`

---

## Configuration File (`network.yaml`)

The file is the single source of truth for the entire setup.
`discover` writes it. `setup` and `validate` read it.

```yaml
version: 1
domain_suffix: home
proxy_ip: 192.168.2.10      # IP of this machine (Frigate server)
cert_warn_days: 30          # warn in setup if a cert expires within N days

services:
  - name: frigate
    ip: 192.168.2.10
    port: 5000
    websocket: true           # required for live camera streams

  - name: truenas
    ip: 192.168.2.20
    port: 80

  - name: mainsail
    ip: 192.168.2.30
    port: 80
    moonraker_port: 7125      # used during discovery fingerprinting
```

---

## Idempotency

Both `discover` and `setup` are safe to rerun:
- `discover` warns before overwriting any IP that differs from the
  existing value; all other fields are preserved
- `setup` creates a fresh backup on each run before regenerating configs;
  adding a new service to `network.yaml` and rerunning `setup` is the
  intended workflow for expanding the setup
