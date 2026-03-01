# Lantern

A CLI tool that automates local HTTPS configuration for self-hosted homelab services.

It discovers services on your LAN, generates trusted TLS certificates via [mkcert](https://github.com/FiloSottile/mkcert), configures [nginx](https://nginx.org) as a reverse proxy, and sets up [dnsmasq](https://thekelleys.org.uk/dnsmasq/doc.html) for local DNS resolution — so you can reach `frigate.home` over HTTPS instead of `192.168.2.10:5000`.

> **Note:** If you are already running a capable router such as pfSense or OPNsense, you likely do not need this tool. Those platforms handle local DNS (Unbound host overrides), DHCP-pushed search domains, and certificate management natively. Lantern is designed for setups where the router is a consumer device with no DNS override capability.

---

## What it does

```
192.168.2.10:5000  →  https://frigate.home
192.168.2.20:80    →  https://truenas.home
192.168.2.30:80    →  https://mainsail.home
```

- **Discovers** known services on the local network automatically
- **Generates** locally-trusted TLS certificates (no browser warnings)
- **Configures** nginx with one config file per service
- **Configures** dnsmasq so local domain names resolve to your proxy
- **Monitors** for IP changes (DHCP drift) and reconfigures automatically via cron

---

## Supported services

Auto-detected during `discover`:

| Service | Default port | Detection endpoint |
|---|---|---|
| [Frigate](https://frigate.video) NVR | 5000 or 8971 | `/api/version` |
| [TrueNAS](https://www.truenas.com) | 80 | `/api/v2.0/system/version` |
| [Mainsail](https://docs.mainsail.xyz) | 80 | Moonraker API at port 7125 |

Other services can be added manually to `network.yaml` or by [implementing a new fingerprinter](#adding-a-new-fingerprinter).

---

## Requirements

- Linux x86-64
- Go 1.22+
- [`mkcert`](https://github.com/FiloSottile/mkcert) — for certificate generation
- `nginx` — reverse proxy
- `dnsmasq` — local DNS resolver
- `sudo` access — required for `setup` and `sync`

---

## Install

```bash
sudo ./install.sh
```

This builds all binaries, installs them to `/usr/local/lib/lantern/bin/`, and installs the `lantern` wrapper to `/usr/local/bin/`. Pass `--skip-tests` to skip the test run.

To build locally without installing (e.g. during development):

```bash
./build.sh              # builds to ./bin/
./lantern-local.sh init # run from the repo
```

---

## Quick start

```bash
# 1. Generate a default network.yaml
./lantern.sh init

# 2. Scan the network to fill in service IPs automatically
./lantern.sh discover

# 3. Validate the result (--ping also checks reachability)
./lantern.sh validate --ping

# 4. Preview what setup will do
sudo ./lantern.sh setup --dry-run

# 5. Apply
sudo ./lantern.sh setup
```

After setup, point your router's primary DNS to this machine's IP and install the mkcert CA certificate on each client device. The `setup` command prints exact instructions at the end.

---

## Commands

| Command | sudo | Description |
|---|---|---|
| `init` | no | Generate a default `network.yaml` |
| `discover` | no | Scan the network and populate `network.yaml` with service IPs |
| `validate` | no | Check `network.yaml` for correctness |
| `certs` | no | Show TLS certificate expiry status |
| `certs renew` | no | Renew expiring (or all) certificates |
| `ls` | no | List all files and directories managed by this tool |
| `setup` | **yes** | Write nginx/dnsmasq configs, generate certs, install cron job |
| `sync` | **yes** | Check for IP changes and reconfigure if anything changed |

### Shared flags

```
--config string    Path to config file (default: network.yaml)
--verbose          Enable debug output
--log-file string  Override the default log file path
                   (default: /var/log/router-configurator.log)
                   Applies to: init, setup, certs renew, sync
```

### setup / sync flags

```
--dry-run    Print actions without executing them
--no-cron    Skip crontab management for this run (setup only)
```

### sync flags

```
--quiet    Suppress terminal output (log file is unaffected)
```

### certs flags

```
--all       Renew all certs regardless of expiry (use with renew)
--dry-run   Show what would be renewed without doing it
```

---

## network.yaml reference

```yaml
version: 1

# Local domain suffix — services become <name>.<suffix>
domain_suffix: home

# IP of this machine (where nginx and dnsmasq run)
proxy_ip: 192.168.2.10

# Warn in 'certs' if a certificate expires within N days
cert_warn_days: 30

monitor:
  check_interval: 5m        # setup installs a cron job at this interval
  log_file: ""              # optional: path for sync log output
  log_max_size: 10MB        # rotate log file when it reaches this size

services:
  - name: frigate
    ip: 192.168.2.10
    port: 5000
    websocket: true          # required for live camera streams

  - name: truenas
    ip: 192.168.2.20
    port: 80

  - name: mainsail
    ip: 192.168.2.30
    port: 80
    moonraker_port: 7125     # used during discovery fingerprinting
```

`network.yaml` is the single source of truth. `init` generates it, `discover` fills in IPs, and `setup`, `sync`, and `validate` read it.

---

## Automatic IP monitoring

Services on DHCP can change IP. Set `monitor.check_interval` and `setup` installs a cron job that runs `sync` automatically:

```yaml
monitor:
  check_interval: 5m
  log_file: /var/log/lantern-sync.log
  log_max_size: 10MB
```

```bash
sudo ./lantern.sh setup
# → Installing cron job... ✓  (*/5 * * * * ... sync --quiet)
```

`setup` is idempotent: rerunning it updates the interval if you change `check_interval`, and removes the cron entry entirely if you delete the field. Pass `--no-cron` to skip crontab management for a single run.

To run sync manually:

```bash
sudo ./lantern.sh sync            # terminal output + log file
sudo ./lantern.sh sync --dry-run  # preview only, no changes
```

Log output is controlled by two independent mechanisms:
- `--quiet` suppresses terminal output only; the log file is unaffected
- `monitor.log_file` controls the log file path; terminal output is unaffected

---

## Adding a new service manually

Edit `network.yaml`, add the service entry, then rerun setup — it is idempotent:

```bash
./lantern.sh validate
sudo ./lantern.sh setup
```

---

## Adding a new fingerprinter

Create a file in `internal/fingerprints/` implementing the `Fingerprinter` interface:

```go
type Fingerprinter interface {
    Name() string
    Probe(ctx context.Context, ip string) (Result, bool)
}
```

Register it in `cmd/discover/main.go`:

```go
registry.Register(fingerprints.NewMyService(client))
```

No changes required to the scanner engine.

---

## Backup and recovery

Before writing any file, `setup` creates a timestamped backup:

```
/var/backups/router-configurator/YYYY-MM-DD-HHMMSS/
```

On failure, it automatically restores from that backup. If auto-restore also fails, it prints the backup path so you can restore manually:

```bash
ls /var/backups/router-configurator/

sudo cp /var/backups/router-configurator/<timestamp>/frigate.home.conf \
        /etc/nginx/sites-enabled/
sudo systemctl restart nginx
```

---

## Running tests

```bash
go test ./...
```

All packages are tested without root access, real network connections, or installed system tools. The `exec.Executor` and `Fingerprinter` interfaces are injected as mocks.

---

## Project structure

```
cmd/              One main.go per subcommand — flag parsing and dependency wiring only
internal/
  config/         YAML structs, read/write, validation
  scanner/        Subnet detection, host sweep, fingerprint orchestration
  fingerprints/   Service-specific fingerprinters (Frigate, TrueNAS, Mainsail)
  writers/        Nginx and dnsmasq config generation (text/template)
  certs/          x509 expiry checking and mkcert integration
  sync/           Atomic check-and-reconfigure cycle
  backup/         Timestamped backup and restore
  paths/          Single source of truth for all managed filesystem paths
  exec/           Executor interface: RealExecutor and DryRunExecutor
  ui/             Printer interface: Terminal, File, Multi, Null
  logrotate/      Size-based log rotation (io.Writer)
```

Dependencies flow inward: `cmd/` depends on `internal/`, never the reverse.
