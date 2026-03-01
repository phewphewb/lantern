# Router Configurator — Architecture

## Guiding Principles

1. **Core logic has no knowledge of specific services** — adding Frigate,
   TrueNAS, or a new service in the future never touches the scanner engine
2. **CLI layer is a thin shell** — commands parse flags, then delegate
   entirely to internal packages; no business logic lives in `cmd/`
3. **Dependencies flow inward** — outer layers depend on inner layers,
   never the reverse
4. **Interfaces at boundaries** — anywhere a component could be swapped
   or tested in isolation, define an interface

---

## Layers

```
┌─────────────────────────────────┐
│           cmd/                  │  Parse flags, wire dependencies,
│   discover/main.go              │  call internal packages, print results
│   setup/main.go                 │
└────────────────┬────────────────┘
                 │ depends on
┌────────────────▼────────────────┐
│         internal/               │  All business logic
│  scanner/   nginx/   dns/       │  No knowledge of CLI, flags, or output
│  certs/     config/             │
└────────────────┬────────────────┘
                 │ depends on
┌────────────────▼────────────────┐
│          interfaces/            │  Contracts between components
│  Fingerprinter  ConfigWriter    │  Defined in internal/, implemented
│  Executor       Printer         │  by concrete types or mocks
└─────────────────────────────────┘
```

---

## Strategy: Swappable Service Fingerprinting

The scanner has no knowledge of Frigate, TrueNAS, or Mainsail.
It only knows about the `Fingerprinter` interface.

```go
// internal/scanner/fingerprinter.go

type Result struct {
    Name string
    IP   string
    Port int
}

type Fingerprinter interface {
    // Name returns the service label (e.g. "frigate")
    Name() string

    // Probe attempts to identify the service at the given IP.
    // Returns (result, true) on match, (zero, false) on no match.
    Probe(ctx context.Context, ip string) (Result, bool)
}
```

Each service is its own implementation in `internal/fingerprints/`:

```
internal/fingerprints/
├── frigate.go       # probes :5000/api/version and :8971/api/version
├── truenas.go       # probes :80/api/v2.0/system/version
└── mainsail.go      # probes :7125/printer/info
```

A registry holds all known fingerprinters:

```go
// internal/scanner/registry.go

type Registry struct {
    fingerprinters []Fingerprinter
}

func (r *Registry) Register(f Fingerprinter) {
    r.fingerprinters = append(r.fingerprinters, f)
}

func (r *Registry) Identify(ctx context.Context, ip string) (Result, bool) {
    for _, f := range r.fingerprinters {
        if result, ok := f.Probe(ctx, ip); ok {
            return result, true
        }
    }
    return Result{}, false
}
```

Wired up in `cmd/discover/main.go`:

```go
registry := &scanner.Registry{}
registry.Register(fingerprints.NewFrigate())
registry.Register(fingerprints.NewTrueNAS())
registry.Register(fingerprints.NewMailsail())

results := scanner.Run(ctx, subnet, registry)
```

**Adding a new service** = create one file in `internal/fingerprints/`,
register it in `cmd/discover/main.go`. Zero changes to scanner logic.

---

## Strategy: Swappable Config Writers

The same pattern applies to config file generation. The setup engine
does not know whether it is writing Nginx or any other proxy format.

```go
// internal/setup/writer.go

type ConfigWriter interface {
    // Write generates and writes config files for all services.
    Write(cfg config.Config) error

    // Reload restarts or signals the service to reload its config.
    Reload() error
}
```

Implementations:

```
internal/writers/
├── nginx.go      # writes to /etc/nginx/sites-enabled/
└── dnsmasq.go    # writes to /etc/dnsmasq.d/
```

Wired in `cmd/setup/main.go`:

```go
writers := []setup.ConfigWriter{
    writers.NewNginx(executor),
    writers.NewDnsmasq(executor),
}
setup.Run(cfg, writers, certs, printer)
```

**Switching from Nginx to Caddy** = write `internal/writers/caddy.go`,
swap the registration. Core setup logic unchanged.

---

## Strategy: Mockable Command Execution

External tools (`mkcert`, `nginx`, `systemctl`) are called through an
interface rather than directly via `os/exec`. This makes the setup logic
fully testable without root access or installed tools.

```go
// internal/exec/executor.go

type Executor interface {
    Run(name string, args ...string) error
    Output(name string, args ...string) ([]byte, error)
}

// RealExecutor wraps os/exec — used in production
type RealExecutor struct{}

// DryRunExecutor prints commands without running them — useful for --dry-run flag
type DryRunExecutor struct{}
```

---

## Strategy: Mockable HTTP Client

The fingerprinter implementations receive an `*http.Client` as a
dependency rather than using `http.DefaultClient`. This allows tests
to inject an `httptest.Server` without hitting real network addresses.

```go
type FrigateFingerprinter struct {
    client *http.Client
}

func NewFrigate(client *http.Client) *FrigateFingerprinter {
    return &FrigateFingerprinter{client: client}
}
```

---

## Context and Cancellation

`context.Context` is threaded through all network operations.
This is critical for the goroutine pool in the scanner:

- User presses Ctrl+C → context cancelled → all 254 probe goroutines
  exit cleanly without leaking
- Per-probe timeout set via `context.WithTimeout` (1s for TCP, 2s for HTTP)
- The top-level context is created in `cmd/` and passed down

```go
ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
defer cancel()
scanner.Run(ctx, subnet, registry)
```

---

## Output Abstraction

Terminal output is separated from business logic via a `Printer` interface.
Internal packages never call `fmt.Println` directly.

```go
// internal/ui/printer.go

type Printer interface {
    Info(msg string)
    Success(label, detail string)
    Warn(label, detail string)
    Fatal(msg string)
    Prompt(question string) bool
}
```

This keeps business logic free of formatting concerns and enables:
- A `--json` output mode in the future (implement `JSONPrinter`)
- Silent mode for scripted use
- Clean unit tests (implement `NullPrinter`)

**All write commands** (`init`, `setup`, `certs renew`, `sync`) always write
timestamped output to a log file. The path is resolved in priority order:

1. `--log-file` CLI flag
2. `monitor.log_file` from `network.yaml` (sync only)
3. Default: `/var/log/router-configurator.log`

Each command wires a `FilePrinter` alongside the `TerminalPrinter` via
`MultiPrinter`. Terminal and log file output are always independent.

```go
// cmd/setup/main.go (same pattern in init, certs, sync)
logPath := resolveLogFile(flagLogFile, defaultLogPath)
rotating := logrotate.New(logPath, defaultLogMaxSize)

printer := ui.NewMultiPrinter(
    ui.NewFilePrinter(rotating),   // always active, timestamped
    ui.NewTerminalPrinter(),        // always active, no timestamps
)
```

| Terminal output | Log file |
|---|---|
| yes (no timestamps) | yes (timestamped, always) |

---

## Config Validation

Reading the YAML and validating its contents are two separate steps.
`config.Read()` only unmarshals. `config.Validate()` checks semantics:

```go
cfg, err := config.Read("network.yaml")   // parse only
if err := cfg.Validate(); err != nil {    // check IPs valid, ports in range,
    printer.Fatal(err.Error())            // required fields present, etc.
}
```

This means validation logic is independently testable without touching
the filesystem.

---

## Dry Run

`--dry-run` is implemented entirely through the `Executor` interface.
When the flag is set, `cmd/setup/main.go` injects a `DryRunExecutor`
instead of `RealExecutor`. No special branching inside business logic.

```go
var executor exec.Executor
if dryRun {
    executor = exec.NewDryRunExecutor(printer)  // prints commands
} else {
    executor = exec.NewRealExecutor()           // runs commands
}
```

File writes follow the same pattern — a `FileWriter` interface with a
`RealFileWriter` and a `DryRunFileWriter` that prints paths instead of
writing them.

---

## Privilege Handling

`discover`, `validate`, and `certs` never require root.

`setup` always requires root — it writes to `/etc/nginx/sites-enabled/`,
`/etc/dnsmasq.d/`, and calls `systemctl`. The binary does not attempt
internal privilege escalation. The user runs:

```
sudo ./router-configurator setup
```

`setup` checks at startup that it is running as root (`os.Getuid() == 0`)
and exits immediately with a clear message if not:

```
Error: setup must be run with sudo
```

---

## Backup and Rollback

Before writing any file, `setup` creates a timestamped backup directory:

```
/var/backups/router-configurator/YYYY-MM-DD-HHMMSS/
```

The backup captures all files that will be overwritten:
- `/etc/nginx/sites-enabled/*.conf` files managed by this tool
- `/etc/dnsmasq.d/local-services.conf`

**On failure:** auto-restore is attempted from the backup. Both the
original error and the restore result are printed. If restore also fails,
the backup path is printed so the user can recover manually.

**On success:** the backup is kept. No automatic cleanup — the user can
delete old backups freely.

Since `setup` is idempotent, recovery from any failure is simply:
fix the problem → run `setup` again.

---

## Certificate Management Command

Certificate status and renewal are handled by a dedicated `certs` command
rather than passively inside `setup`. This gives the user explicit control
and a clear entrypoint.

`certs` reads existing certificate files, parses them using Go's
`crypto/x509` package (stdlib — no external dependency), and compares
expiry dates against `cert_warn_days` from `network.yaml`.

```go
cert, _ := x509.ParseCertificate(block.Bytes)
daysLeft := int(time.Until(cert.NotAfter).Hours() / 24)
```

`certs renew` delegates to `mkcert` via the `Executor` interface —
the same executor used by `setup`, meaning `--dry-run` works on it too.

---

## Config Versioning

`network.yaml` carries a top-level `version` field:

```yaml
version: 1
```

On startup, `config.Read()` checks this field before unmarshalling the
rest of the file. If the version is missing or unsupported, it exits with
a clear message rather than silently misreading the config.

This allows future schema changes without silent breakage. The supported
version list is a constant in `internal/config/config.go`.

---

## Managed Paths as a Single Source of Truth

Every path the tool reads from or writes to is defined in one place:
`internal/paths/paths.go`. No other package hardcodes a path string.

```go
// internal/paths/paths.go

type Paths struct {
    NginxSitesDir  string   // /etc/nginx/sites-enabled/
    DnsmasqConf    string   // /etc/dnsmasq.d/local-services.conf
    CertDir        string   // /etc/ssl/local/
    BackupRoot     string   // /var/backups/router-configurator/
    MkcertCARoot   string   // ~/.local/share/mkcert/
}

func (p *Paths) NginxConfFor(service string) string { ... }
func (p *Paths) CertFileFor(service string) string  { ... }
func (p *Paths) KeyFileFor(service string) string   { ... }
```

This serves two purposes:
1. `ls` can enumerate all managed paths without duplicating knowledge
   from `setup`, `certs`, or `backup`
2. Paths become configurable in the future without hunting through
   the codebase for hardcoded strings

---

## Mode-Agnostic Monitoring

`sync` is the atomic unit of monitoring — one complete check-and-reconfigure
cycle. It has no knowledge of who called it or how it was scheduled.

```
User runs manually  →  ./router-configurator sync
Cron runs it        →  same binary, same code path
Test calls it       →  same sync.Run(), injected mocks
```

All reconfiguration logic lives in `internal/sync/sync.go`. The
`cmd/sync/main.go` entrypoint is a thin shell: parse flags, wire
dependencies, call `sync.Run()`, exit.

Cron is the scheduler — it requires no code. The user adds one line to
root's crontab and the OS handles the rest.

**Output is controlled by two independent mechanisms:**

1. **`--quiet` flag** — suppresses all terminal/stdout output. The log
   file is completely unaffected by this flag.

2. **`monitor.log_file` config** — when set, `sync` writes all output
   to the log file. The `--quiet` flag is completely unaffected by this.

```go
// cmd/sync/main.go
var printers []ui.Printer

if cfg.Monitor.LogFile != "" {
    rotating := logrotate.New(cfg.Monitor.LogFile, cfg.Monitor.LogMaxSize)
    printers = append(printers, ui.NewFilePrinter(rotating))
}

if !quiet {
    printers = append(printers, ui.NewTerminalPrinter())
}

printer := ui.NewMultiPrinter(printers...)
```

| log_file set | --quiet | Terminal output | Log file |
|---|---|---|---|
| yes | no | yes (no timestamps) | yes (timestamped) |
| yes | yes | no | yes |
| no | no | yes | — |
| no | yes | no | — |

Typical cron setup with log file configured:

```
*/5 * * * *  /usr/local/bin/router-configurator sync --quiet
```

**Responsibilities are separated:**

- `internal/logrotate/rotator.go` — `RotatingFile` implements `io.Writer`.
  Checks file size before each write; renames to `.1` and reopens when
  over the limit. No knowledge of formatting.
- `ui.FilePrinter` — formats with timestamps, writes to an `io.Writer`.
  No knowledge of rotation or file paths.
- `ui.TerminalPrinter` — formats without timestamps, writes to stdout.
- `ui.MultiPrinter` — fans output out to N printers.

`internal/sync/sync.go` calls `printer.Info()` / `printer.Warn()` — it
has no knowledge of how many outputs are active or what format they use.

`setup` manages the `sync` cron job in root's crontab based on
`monitor.check_interval`:

- **Set** → install (or update) the cron entry for `sync --quiet`
- **Absent / removed** → remove the cron entry if one exists

This is idempotent: every `setup` run reconciles the crontab to match the
current config. The cron entry is identified by a trailing comment tag so
`setup` can find and replace or remove it without touching other entries:

```
*/5 * * * *  /usr/local/bin/router-configurator sync --quiet  # managed-by:router-configurator
```

Pass `--no-cron` to skip crontab management entirely for a single run.

---

## Discover Merge Strategy

When `discover` runs and `network.yaml` already exists:

- Fields unrelated to discovery (`websocket`, `moonraker_port`,
  `cert_warn_days`, `domain_suffix`) are always preserved
- If a discovered IP **matches** the existing value → silent, no change
- If a discovered IP **differs** from the existing value → warn and show
  the diff, require explicit `[y/N]` confirmation before overwriting:

```
  ! mainsail  existing: 192.168.2.30  discovered: 192.168.2.31
    Update? [y/N]
```
