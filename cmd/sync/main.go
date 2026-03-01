package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"time"

	"lantern/internal/config"
	xec "lantern/internal/exec"
	"lantern/internal/fingerprints"
	"lantern/internal/logrotate"
	"lantern/internal/paths"
	"lantern/internal/scanner"
	syncp "lantern/internal/sync"
	"lantern/internal/ui"
	"lantern/internal/writers"
)

const defaultLogPath = "/var/log/lantern.log"
const defaultLogMaxSize = 10 * 1024 * 1024 // 10 MB

func main() {
	fs := flag.NewFlagSet("sync", flag.ExitOnError)
	cfgPath := fs.String("config", "network.yaml", "path to network.yaml")
	quiet := fs.Bool("quiet", false, "suppress terminal output")
	dryRun := fs.Bool("dry-run", false, "show changes without applying")
	logFile := fs.String("log-file", "", "override log file path")
	fs.Parse(os.Args[1:])

	if os.Getuid() != 0 {
		ui.NewTerminalPrinter().Info("Error: sync must be run with sudo")
		os.Exit(1)
	}

	cfg, err := config.Read(*cfgPath)
	if err != nil {
		ui.NewTerminalPrinter().Info("Error reading config: " + err.Error())
		os.Exit(1)
	}

	// Resolve log file path.
	logPath := defaultLogPath
	if cfg.Monitor.LogFile != "" {
		logPath = cfg.Monitor.LogFile
	}
	if *logFile != "" {
		logPath = *logFile
	}

	// Wire printers.
	var printers []ui.Printer
	rotating := logrotate.New(logPath, defaultLogMaxSize)
	printers = append(printers, ui.NewFilePrinter(rotating))
	if !*quiet {
		printers = append(printers, ui.NewTerminalPrinter())
	}
	printer := ui.NewMultiPrinter(printers...)

	var executor xec.Executor
	if *dryRun {
		executor = xec.NewDryRunExecutor(func(cmd string) {
			printer.Info("[DRY RUN] would run: " + cmd)
		})
	} else {
		executor = xec.NewRealExecutor()
	}

	p := paths.Default()
	client := &http.Client{Timeout: 2 * time.Second}
	reg := &scanner.Registry{}
	reg.Register(fingerprints.NewFrigate(client))
	reg.Register(fingerprints.NewTrueNAS(client))
	reg.Register(fingerprints.NewMainsail(client))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	setup := func(updated config.Config) error {
		nw := writers.NewNginx(p, executor)
		if err := nw.Write(updated); err != nil {
			return err
		}
		dw := writers.NewDnsmasq(p, executor)
		if err := dw.Write(updated); err != nil {
			return err
		}
		if err := nw.Reload(); err != nil {
			return err
		}
		return dw.Reload()
	}

	if err := syncp.Run(ctx, cfg, *cfgPath, p, reg, setup, printer); err != nil {
		printer.Info("Error: " + err.Error())
		os.Exit(1)
	}
}
