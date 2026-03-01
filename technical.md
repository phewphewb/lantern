# Router Configurator — Technical Specification

## Language & Runtime

- Go 1.22+
- Single compiled binary with subcommands
- Target platform: Linux (x86-64), runs on the Frigate server

---

## Project Structure

```
router-configurator/
├── network.yaml
├── go.mod
├── go.sum
├── cmd/
│   ├── init/
│   │   └── main.go          # init subcommand entrypoint
│   ├── discover/
│   │   └── main.go          # discover subcommand entrypoint
│   ├── setup/
│   │   └── main.go          # setup subcommand entrypoint
│   ├── sync/
│   │   └── main.go          # sync subcommand entrypoint
│   ├── certs/
│   │   └── main.go          # certs status/renew subcommand entrypoint
│   ├── validate/
│   │   └── main.go          # validate subcommand entrypoint
│   └── ls/
│       └── main.go          # ls subcommand entrypoint
├── internal/
│   ├── config/
│   │   └── config.go        # YAML structs, read/write, validation
│   ├── scanner/
│   │   └── scanner.go       # subnet detection, host probing, fingerprinting
│   ├── fingerprints/
│   │   ├── frigate.go
│   │   ├── truenas.go
│   │   └── mainsail.go
│   ├── writers/
│   │   ├── nginx.go         # nginx config generation
│   │   └── dnsmasq.go       # dnsmasq config generation
│   ├── certs/
│   │   └── certs.go         # mkcert wrapper + x509 expiry checking
│   ├── exec/
│   │   └── executor.go      # Executor interface, RealExecutor, DryRunExecutor
│   ├── backup/
│   │   └── backup.go        # timestamped backup + restore
│   ├── paths/
│   │   └── paths.go         # single source of truth for all managed paths
│   ├── sync/
│   │   └── sync.go          # atomic check-and-reconfigure logic
│   ├── logrotate/
│   │   └── rotator.go       # RotatingFile io.Writer — size-based rotation
│   └── ui/
│       └── printer.go       # Printer interface, TerminalPrinter, FilePrinter, MultiPrinter
└── templates/
    ├── nginx-service.conf.tmpl
    └── dnsmasq.conf.tmpl
```

---

## Dependencies

| Package | Purpose |
|---|---|
| `gopkg.in/yaml.v3` | YAML parsing and writing |

All other functionality uses Go standard library only.

---

## CLI Interface

Built with the `flag` stdlib package — no external CLI framework.

```
router-configurator <command> [flags]

Commands:
  init              Generate a default network.yaml
  discover          Scan network and populate network.yaml
  setup             Configure nginx, dnsmasq, and certs from network.yaml
  sync              Check for IP changes and reconfigure if needed
  certs [status]    Show TLS certificate expiry status
  certs renew       Renew expiring (or all) certificates
  validate          Check network.yaml for correctness
  ls                List all files and directories the tool manages

Shared flags (all commands):
  --config string    Path to config file (default: network.yaml)
  --verbose          Enable debug output

Write command flags (init, setup, certs renew, sync):
  --log-file string  Override the default log file path.
                     Default: /var/log/router-configurator.log
                     All write commands always log to a file — this flag
                     changes where. For sync, takes precedence over
                     monitor.log_file from network.yaml.

setup / sync flags:
  --dry-run          Print actions without executing them

sync flags:
  --quiet            Suppress terminal/stdout output (log file unaffected)

certs renew flags:
  --all              Renew all certs regardless of expiry
  --dry-run          Print what would be renewed without doing it
```

`init`, `discover`, `validate`, `certs`, and `ls` run as the current user.
`setup` and `sync` must be run with `sudo`.

---

## Config Structs

```go
type Monitor struct {
    CheckInterval string `yaml:"check_interval"`
    LogFile       string `yaml:"log_file,omitempty"`
    LogMaxSize    string `yaml:"log_max_size,omitempty"`  // e.g. "10MB"
}

type Config struct {
    Version      int       `yaml:"version"`
    DomainSuffix string    `yaml:"domain_suffix"`
    ProxyIP      string    `yaml:"proxy_ip"`
    CertWarnDays int       `yaml:"cert_warn_days"`
    Monitor      Monitor   `yaml:"monitor"`
    Services     []Service `yaml:"services"`
}

type Service struct {
    Name          string `yaml:"name"`
    IP            string `yaml:"ip"`
    Port          int    `yaml:"port"`
    WebSocket     bool   `yaml:"websocket,omitempty"`
    MoonrakerPort int    `yaml:"moonraker_port,omitempty"`
}
```

---

## Discovery Algorithm

### 1. Subnet detection
Parse the machine's active non-loopback network interface using the `net`
package to determine the local subnet (e.g. `192.168.2.0/24`).

### 2. Host sweep
Spawn 254 goroutines (one per candidate IP), controlled by `sync.WaitGroup`.
Each goroutine attempts a TCP connection to a list of known ports:
`[80, 443, 5000, 7125, 8971]` with a **1 second timeout**.
Active hosts are sent on a buffered results channel.

### 3. Fingerprinting
For each active host, attempt HTTP GET requests to known service endpoints
with a **2 second timeout**. Match response status and body to identify
the service.

| Service  | Probe endpoint                       | Match condition                  |
|----------|--------------------------------------|----------------------------------|
| Frigate  | `GET http://ip:5000/api/version`     | JSON body contains `"version"`   |
| Frigate  | `GET http://ip:8971/api/version`     | JSON body contains `"version"`   |
| TrueNAS  | `GET http://ip/api/v2.0/system/version` | HTTP 200 with JSON body       |
| Mainsail | `GET http://ip:7125/printer/info`    | JSON body contains `"state"`     |

### 4. Concurrency model
```
main goroutine
  └── launches 254 scanner goroutines
        └── each sends (ip, port) or nothing to results channel
  └── reads from results channel until WaitGroup done
  └── launches fingerprint goroutines for active hosts
  └── collects identified services
```

---

## Config File Generation

Templates are embedded into the binary at compile time using `embed.FS`
(stdlib, Go 1.16+), so the binary has no external file dependencies.

**Nginx**: one file per service written to `/etc/nginx/sites-enabled/`
using `text/template`. Each file includes:
- `server` block on port 443 with SSL
- `proxy_pass` to service IP:port
- WebSocket upgrade headers if `websocket: true`
- Separate `server` block on port 80 for HTTP → HTTPS redirect

**dnsmasq**: single file written to `/etc/dnsmasq.d/local-services.conf`
using `text/template`. One `address=/service.home/ip` line per service.

---

## External Tool Integration

All external tools invoked via `os/exec`. Each call:
1. Checks the tool is available with `exec.LookPath`
2. Captures stdout and stderr
3. On non-zero exit: wraps stderr as a Go error

| Tool | Invocation |
|---|---|
| `mkcert` | `mkcert -install` then `mkcert -cert-file ... -key-file ... domain` |
| `systemctl` | `systemctl enable --now nginx` / `systemctl restart nginx` |
| `systemctl` | `systemctl enable --now dnsmasq` / `systemctl restart dnsmasq` |

---

## Error Handling

- Errors wrapped with context: `fmt.Errorf("scanning host %s: %w", ip, err)`
- Fatal errors (missing tool, unreadable config): print message, `os.Exit(1)`
- Non-fatal warnings (unreachable service IP): print, prompt user to
  continue or abort
- Network timeouts during discovery: silently skip (expected for inactive hosts)

---

## Logging

- `log/slog` (stdlib, Go 1.21+) for structured logging
- Default level: `INFO` — shows progress steps and results
- `--verbose` flag sets level to `DEBUG` — shows per-host probe results,
  raw HTTP responses, exec commands
