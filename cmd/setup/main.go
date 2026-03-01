package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"lantern/internal/backup"
	"lantern/internal/certs"
	"lantern/internal/config"
	xec "lantern/internal/exec"
	"lantern/internal/logrotate"
	"lantern/internal/paths"
	"lantern/internal/ui"
	"lantern/internal/writers"
)

const defaultLogPath = "/var/log/lantern.log"
const defaultLogMaxSize = 10 * 1024 * 1024 // 10 MB

func main() {
	fs := flag.NewFlagSet("setup", flag.ExitOnError)
	cfgPath := fs.String("config", "network.yaml", "path to network.yaml")
	dryRun := fs.Bool("dry-run", false, "print every action without executing")
	logFile := fs.String("log-file", "", "override log file path")
	noCron := fs.Bool("no-cron", false, "skip cron job installation")
	fs.Parse(os.Args[1:])

	if os.Getuid() != 0 {
		ui.NewTerminalPrinter().Info("Error: setup must be run with sudo")
		os.Exit(1)
	}

	// Wire log + terminal printers.
	logPath := defaultLogPath
	if *logFile != "" {
		logPath = *logFile
	}
	var printerList []ui.Printer
	if !*dryRun {
		rotating := logrotate.New(logPath, defaultLogMaxSize)
		printerList = append(printerList, ui.NewFilePrinter(rotating))
	}
	printerList = append(printerList, ui.NewTerminalPrinter())
	printer := ui.NewMultiPrinter(printerList...)

	prefix := ""
	if *dryRun {
		prefix = "[DRY RUN] "
	}

	// Read + validate config.
	printer.Info(prefix + "Reading " + *cfgPath + "...")
	cfg, err := config.Read(*cfgPath)
	if err != nil {
		printer.Info("Error: " + err.Error())
		os.Exit(1)
	}
	if err := cfg.Validate(); err != nil {
		printer.Info("Error: " + err.Error())
		os.Exit(1)
	}
	for _, svc := range cfg.Services {
		printer.Info(fmt.Sprintf("  ✓ %-12s %s:%d  → %s.%s",
			svc.Name, svc.IP, svc.Port, svc.Name, cfg.DomainSuffix))
	}

	// Wire executor.
	var executor xec.Executor
	if *dryRun {
		executor = xec.NewDryRunExecutor(func(cmd string) {
			printer.Info("[DRY RUN] would run: " + cmd)
		})
	} else {
		executor = xec.NewRealExecutor()
	}

	p := paths.Default()

	if *dryRun {
		printDryRun(cfg, p, *noCron, printer)
		printer.Info("\nNo changes made.")
		return
	}

	// Check tools.
	printer.Info("\nChecking tools...")
	for _, tool := range []string{"nginx", "dnsmasq", "mkcert"} {
		if err := executor.Run("which", tool); err != nil {
			printer.Info("  ✗ " + tool + " not installed")
			os.Exit(1)
		}
		printer.Info("  ✓ " + tool + "  installed")
	}

	// Backup.
	printer.Info("\nBacking up existing config...")
	backupDir, err := backup.Create(p)
	if err != nil {
		printer.Info("Error creating backup: " + err.Error())
		os.Exit(1)
	}
	printer.Info("  → " + backupDir + "/")

	// Install mkcert CA.
	if err := executor.Run("mkcert", "-install"); err != nil {
		printer.Info("Error installing mkcert CA: " + err.Error())
		os.Exit(1)
	}

	// Generate certs.
	printer.Info("\nGenerating certificates...")
	if err := certs.Renew(p, cfg, executor, 0, true); err != nil {
		restoreOrDie(backupDir, p, cfg, *cfgPath, err, printer)
	}
	for _, svc := range cfg.Services {
		printer.Info("  ✓ " + svc.Name + "." + cfg.DomainSuffix)
	}

	// Write nginx configs.
	printer.Info("\nWriting Nginx config...")
	nw := writers.NewNginx(p, executor)
	if err := nw.Write(cfg); err != nil {
		restoreOrDie(backupDir, p, cfg, *cfgPath, err, printer)
	}
	printer.Info("  ✓")

	// Write dnsmasq config.
	printer.Info("Writing dnsmasq config...")
	dw := writers.NewDnsmasq(p, executor)
	if err := dw.Write(cfg); err != nil {
		restoreOrDie(backupDir, p, cfg, *cfgPath, err, printer)
	}
	printer.Info("  ✓")

	// Restart services.
	printer.Info("Restarting services...")
	if err := nw.Reload(); err != nil {
		restoreOrDie(backupDir, p, cfg, *cfgPath, err, printer)
	}
	if err := dw.Reload(); err != nil {
		restoreOrDie(backupDir, p, cfg, *cfgPath, err, printer)
	}
	printer.Info("  ✓")

	// Cron job.
	if !*noCron {
		if err := reconcileCron(cfg, executor, printer); err != nil {
			printer.Warn("cron", err.Error())
		}
	}

	printer.Info("\nSetup complete.")
	printNextSteps(cfg, printer)
}

// reconcileCron adds, updates, or removes the sync cron entry in root's crontab.
func reconcileCron(cfg config.Config, executor xec.Executor, printer ui.Printer) error {
	const tag = "# managed-by:lantern"

	// Read current crontab.
	out, _ := executor.Output("crontab", "-l")
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = nil
	}

	// Remove managed entry.
	var filtered []string
	for _, l := range lines {
		if !strings.Contains(l, tag) {
			filtered = append(filtered, l)
		}
	}

	if cfg.Monitor.CheckInterval == "" {
		// No interval → remove entry only.
		if len(filtered) == len(lines) {
			return nil // nothing to remove
		}
	} else {
		// Parse interval → cron expression.
		cronExpr, err := intervalToCron(cfg.Monitor.CheckInterval)
		if err != nil {
			return fmt.Errorf("invalid check_interval %q: %w", cfg.Monitor.CheckInterval, err)
		}
		bin, _ := os.Executable()
		entry := fmt.Sprintf("%s  %s sync --quiet  %s", cronExpr, bin, tag)
		filtered = append(filtered, entry)
		printer.Info("Installing cron job...    ✓  (" + cronExpr + " ... sync --quiet)")
	}

	newCrontab := strings.Join(filtered, "\n")
	if newCrontab != "" {
		newCrontab += "\n"
	}
	// Write back via `crontab -`.
	return executor.Run("sh", "-c", fmt.Sprintf("echo %q | crontab -", newCrontab))
}

// intervalToCron converts "5m", "10m", "1h" etc. to a cron expression.
func intervalToCron(interval string) (string, error) {
	interval = strings.TrimSpace(interval)
	if strings.HasSuffix(interval, "m") {
		mins, err := strconv.Atoi(strings.TrimSuffix(interval, "m"))
		if err != nil || mins < 1 || mins > 59 {
			return "", fmt.Errorf("invalid minute interval")
		}
		return fmt.Sprintf("*/%d * * * *", mins), nil
	}
	if strings.HasSuffix(interval, "h") {
		hrs, err := strconv.Atoi(strings.TrimSuffix(interval, "h"))
		if err != nil || hrs < 1 || hrs > 23 {
			return "", fmt.Errorf("invalid hour interval")
		}
		return fmt.Sprintf("0 */%d * * *", hrs), nil
	}
	return "", fmt.Errorf("unsupported interval format (use Nm or Nh)")
}

func restoreOrDie(backupDir string, p paths.Paths, cfg config.Config, cfgPath string, originalErr error, printer ui.Printer) {
	printer.Warn("error", originalErr.Error())
	printer.Info("Auto-restoring from " + backupDir + " ...")
	if err := backup.Restore(backupDir, p); err != nil {
		printer.Info("Restore also failed: " + err.Error())
		printer.Info("Backup is at: " + backupDir)
		os.Exit(1)
	}
	printer.Info("Restored successfully.")
	os.Exit(1)
}

func printDryRun(cfg config.Config, p paths.Paths, noCron bool, printer ui.Printer) {
	printer.Info("\n[DRY RUN] Would back up existing configs")

	printer.Info("\n[DRY RUN] Would run:")
	printer.Info("  mkcert -install")
	for _, svc := range cfg.Services {
		domain := svc.Name + "." + cfg.DomainSuffix
		printer.Info(fmt.Sprintf("  mkcert -cert-file %s -key-file %s %s",
			p.CertFileFor(svc.Name, cfg.DomainSuffix),
			p.KeyFileFor(svc.Name, cfg.DomainSuffix),
			domain))
	}

	printer.Info("\n[DRY RUN] Would write:")
	for _, svc := range cfg.Services {
		printer.Info("  " + p.NginxConfFor(svc.Name, cfg.DomainSuffix))
	}
	printer.Info("  " + p.DnsmasqConf)

	printer.Info("\n[DRY RUN] Would restart: nginx, dnsmasq")

	if !noCron && cfg.Monitor.CheckInterval != "" {
		cronExpr, _ := intervalToCron(cfg.Monitor.CheckInterval)
		printer.Info("\n[DRY RUN] Would install cron job (root crontab):")
		printer.Info("  " + cronExpr + "  /usr/local/bin/lantern sync --quiet")
	}
}

func printNextSteps(cfg config.Config, printer ui.Printer) {
	printer.Info(`
─── Next steps ────────────────────────────────────────────
1. Point your Bell router DNS to this machine:
     Router: http://192.168.2.1 → Advanced → DNS → Primary DNS
     Set to: ` + cfg.ProxyIP + `

2. Install the CA certificate on each client device:
     CA file: /home/user/.local/share/mkcert/rootCA.pem

   Windows : double-click → install to "Trusted Root CAs"
   macOS   : Keychain Access → import → set to Always Trust
   iOS     : download file → Settings → trust profile
   Android : Settings → Security → Install certificate
───────────────────────────────────────────────────────────`)
}
