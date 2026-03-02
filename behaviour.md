# Router Configurator — Behaviour Specification

## Overview

A CLI tool that automates local network HTTPS configuration. It discovers
services on the local network, generates TLS certificates, configures a
reverse proxy, and sets up local DNS — enabling access to local services
via domain names over HTTPS instead of raw IP addresses.

---

## Commands

### `init`

Generates a `network.yaml` file pre-populated with all fields, sensible
defaults, and inline comments. The starting point before running
`discover` or editing manually.

**Flags:**
- `--config string` — output path (default: `network.yaml`)
- `--force` — overwrite if file already exists
- `--log-file string` — override the default log file path
  (default: `/var/log/router-configurator.log`)

**Expected output:**
```
Writing network.yaml... ✓

Next steps:
  1. Run discover to fill in service IPs automatically:
       ./router-configurator discover
  2. Or edit network.yaml manually, then validate:
       ./router-configurator validate --ping
```

**Generated file:**
```yaml
version: 1

# Local domain suffix — services become <name>.<suffix>
domain_suffix: home

# IP of this machine (where nginx and dnsmasq will run)
proxy_ip: ""

# Warn in 'certs status' if a certificate expires within N days
cert_warn_days: 30

# Automatic IP monitoring (used by 'sync', scheduled via cron)
monitor:
  check_interval: 5m      # informational — used by 'sync' cron instructions
  log_file: ""            # optional log file for sync output

services:
  # Add services here, or run: discover
  # - name: frigate
  #   ip: ""
  #   port: 5000
  #   websocket: true
  #
  # - name: truenas
  #   ip: ""
  #   port: 80
  #
  # - name: mainsail
  #   ip: ""
  #   port: 80
  #   moonraker_port: 7125
```

**Error cases:**
- File already exists → exit with message, suggest `--force` to overwrite

---

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
- `--log-file string` — override the default log file path
  (default: `/var/log/router-configurator.log`). Independent of terminal
  output — both are always active simultaneously.
- `--no-cron` — skip cron job installation even if `monitor.check_interval`
  is set in `network.yaml`

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
11. Reconciles the `sync` cron job in root's crontab:
    - `monitor.check_interval` **set** → add or update the entry
    - `monitor.check_interval` **absent** → remove the entry if present
    Skipped entirely when `--no-cron` is passed.
12. Prints post-setup instructions for the router and client devices

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
Installing cron job...    ✓  (*/5 * * * * ... sync --quiet)

Setup complete.

─── Next steps ────────────────────────────────────────────
1. Point your router DNS to this machine:
     Log in to your router's admin panel → DNS settings → Primary DNS
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

[DRY RUN] Would install cron job (root crontab):
  */5 * * * *  /usr/local/bin/router-configurator sync --quiet

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
- `--log-file string` — override the default log file path for `renew`
  (default: `/var/log/router-configurator.log`). `status` is read-only
  and never writes to the log file.

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

### `sync`

Non-interactive check-and-reconfigure cycle. Probes each service for its
current IP, compares against `network.yaml`, and runs `setup` automatically
if anything changed. Designed to be called by cron or manually.
Must be run with `sudo`.

**Flags:**
- `--config string` — path to config file (default: `network.yaml`)
- `--quiet` — suppress terminal/stdout output. Log file is unaffected.
- `--dry-run` — show what would change without applying anything
- `--log-file string` — override the log file path. Takes precedence over
  `monitor.log_file` from `network.yaml`
  (default: `/var/log/router-configurator.log`).

**Output — two independent controls:**
- `--quiet` controls terminal/stdout output only
- Log file output is always active; path resolved in this order:
  `--log-file` flag → `monitor.log_file` config → `/var/log/router-configurator.log`
- Both controls are unaffected by each other

**Log rotation:**
When the log file reaches `monitor.log_max_size`, it is renamed to
`<log_file>.1` and a new file is started. Only one rotated file is kept.

**Output format — terminal (no timestamps):**
```
IP change detected:
  mainsail  192.168.2.30 → 192.168.2.31

Updating network.yaml... ✓
Running setup...         ✓
Reconfigured successfully.
```

**Output format — log file (timestamps on every line):**
```
2024-01-15 14:32:05  IP change detected: mainsail 192.168.2.30 → 192.168.2.31
2024-01-15 14:32:05  Updating network.yaml... ok
2024-01-15 14:32:06  Running setup... ok
2024-01-15 14:32:08  Reconfigured successfully
```

**No change — terminal:**
```
All service IPs unchanged. Nothing to do.
```

**No change — log file:**
```
2024-01-15 14:32:05  All service IPs unchanged. Nothing to do.
```

**Flow:**
1. Reads `network.yaml`
2. Probes each listed service for its current IP (non-interactively)
3. Compares to existing config
4. If nothing changed → exit 0
5. If any IP changed → update `network.yaml`, run `setup`, log changes

**Error cases:**
- A service cannot be reached → log warning and skip, do not treat as
  IP change
- `setup` fails after IP update → auto-restore from backup, log error,
  exit non-zero

---

## Configuration File (`network.yaml`)

The file is the single source of truth for the entire setup.
`init` generates it. `discover` fills in IPs. `setup`, `sync`, and
`validate` read it.

```yaml
version: 1
domain_suffix: home
proxy_ip: 192.168.2.10      # IP of this machine (Frigate server)
cert_warn_days: 30          # warn in 'certs status' if cert expires within N days

monitor:
  check_interval: 5m        # setup installs a cron job at this interval
  log_file: /var/log/router-configurator.log   # sync writes here; omit for stdout
  log_max_size: 10MB        # rotate log file when it reaches this size

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
