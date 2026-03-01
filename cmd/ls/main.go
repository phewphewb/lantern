package main

import (
	"crypto/x509"
	"encoding/pem"
	"flag"
	"fmt"
	"os"
	"time"

	"lantern/internal/config"
	"lantern/internal/paths"
	"lantern/internal/ui"
)

func main() {
	fs := flag.NewFlagSet("ls", flag.ExitOnError)
	cfgPath := fs.String("config", "network.yaml", "path to network.yaml")
	fs.Parse(os.Args[1:])

	printer := ui.NewTerminalPrinter()
	p := paths.Default()

	cfg, err := config.Read(*cfgPath)
	if err != nil {
		printer.Info("Note: could not read " + *cfgPath + " — showing default paths")
		cfg = config.Config{DomainSuffix: "home"}
	}

	printer.Info("Managed paths:\n")

	// Config.
	printer.Info("Config:")
	printFile(printer, *cfgPath, cfg.CertWarnDays)

	// Certificates.
	printer.Info("\nCertificates:")
	for _, svc := range cfg.Services {
		crtPath := p.CertFileFor(svc.Name, cfg.DomainSuffix)
		printCert(printer, crtPath, cfg.CertWarnDays)
	}

	// Nginx configs.
	printer.Info("\nNginx config:")
	for _, svc := range cfg.Services {
		printFile(printer, p.NginxConfFor(svc.Name, cfg.DomainSuffix), 0)
	}

	// DNS config.
	printer.Info("\nDNS config:")
	printFile(printer, p.DnsmasqConf, 0)

	// Backups.
	printer.Info("\nBackups:")
	printer.Info("  " + p.BackupRoot)
	entries, _ := os.ReadDir(p.BackupRoot)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		subEntries, _ := os.ReadDir(p.BackupRoot + e.Name())
		var totalSize int64
		for _, se := range subEntries {
			info, _ := se.Info()
			if info != nil {
				totalSize += info.Size()
			}
		}
		printer.Info(fmt.Sprintf("  ✓ %s/   %d files   %.1f KB", e.Name(), len(subEntries), float64(totalSize)/1024))
	}
}

func printFile(printer ui.Printer, path string, _ int) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		printer.Info(fmt.Sprintf("  - %s  (not yet created)", path))
	} else {
		printer.Info(fmt.Sprintf("  ✓ %s", path))
	}
}

func printCert(printer ui.Printer, path string, warnDays int) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		printer.Info(fmt.Sprintf("  - %s  (not yet created)", path))
		return
	}
	if err != nil {
		printer.Info(fmt.Sprintf("  ? %s  (error reading)", path))
		return
	}
	block, _ := pem.Decode(data)
	if block == nil {
		printer.Info(fmt.Sprintf("  ? %s  (invalid PEM)", path))
		return
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		printer.Info(fmt.Sprintf("  ? %s  (invalid cert)", path))
		return
	}
	daysLeft := int(time.Until(cert.NotAfter).Hours() / 24)
	expiry := cert.NotAfter.Format("2006-01-02")
	if warnDays > 0 && daysLeft <= warnDays {
		printer.Info(fmt.Sprintf("  ! %s      expires %s  (%d days)", path, expiry, daysLeft))
	} else {
		printer.Info(fmt.Sprintf("  ✓ %s      expires %s  (%d days)", path, expiry, daysLeft))
	}
}
