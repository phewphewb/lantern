package main

import (
	"flag"
	"fmt"
	"os"

	"lantern/internal/certs"
	"lantern/internal/config"
	xec "lantern/internal/exec"
	"lantern/internal/paths"
	"lantern/internal/ui"
)

func main() {
	fs := flag.NewFlagSet("certs", flag.ExitOnError)
	cfgPath := fs.String("config", "network.yaml", "path to network.yaml")
	all := fs.Bool("all", false, "renew all certs regardless of expiry")
	dryRun := fs.Bool("dry-run", false, "print what would be renewed without doing it")
	fs.Parse(os.Args[1:])

	subcommand := "status"
	if fs.NArg() > 0 {
		subcommand = fs.Arg(0)
	}

	printer := ui.NewTerminalPrinter()
	p := paths.Default()

	cfg, err := config.Read(*cfgPath)
	if err != nil {
		printer.Info("Error reading config: " + err.Error())
		os.Exit(1)
	}

	switch subcommand {
	case "status", "":
		runStatus(cfg, p, printer)
	case "renew":
		runRenew(cfg, p, printer, *all, *dryRun)
	default:
		printer.Info("Unknown subcommand: " + subcommand + ". Use: status, renew")
		os.Exit(1)
	}
}

func runStatus(cfg config.Config, p paths.Paths, printer ui.Printer) {
	statuses, err := certs.Status(p, cfg)
	if err != nil {
		printer.Info("Error reading certs: " + err.Error())
		os.Exit(1)
	}

	printer.Info("Certificate status:")
	for _, s := range statuses {
		domain := s.Name + "." + cfg.DomainSuffix
		if !s.Found {
			printer.Info(fmt.Sprintf("  - %-20s not found (run setup first)", domain))
			continue
		}
		expiry := s.NotAfter.Format("2006-01-02")
		line := fmt.Sprintf("  ✓ %-20s expires %s  (%d days)", domain, expiry, s.DaysLeft)
		if s.DaysLeft <= cfg.CertWarnDays {
			line = fmt.Sprintf("  ! %-20s expires %s  (%d days)  ← expiring soon", domain, expiry, s.DaysLeft)
		}
		printer.Info(line)
	}
}

func runRenew(cfg config.Config, p paths.Paths, printer ui.Printer, all, dryRun bool) {
	threshold := cfg.CertWarnDays
	if all {
		printer.Info("Renewing all certificates...")
	} else {
		printer.Info(fmt.Sprintf("Renewing expiring certificates (threshold: %d days)...", threshold))
	}

	var executor xec.Executor
	if dryRun {
		executor = xec.NewDryRunExecutor(func(cmd string) {
			printer.Info("  [DRY RUN] would run: " + cmd)
		})
	} else {
		executor = xec.NewRealExecutor()
	}

	if err := certs.Renew(p, cfg, executor, threshold, all); err != nil {
		printer.Info("Error renewing certs: " + err.Error())
		os.Exit(1)
	}

	if !dryRun {
		printer.Info("\nDone. Run setup to reload nginx with the new certificates.")
	}
}
