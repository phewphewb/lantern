package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"lantern/internal/config"
	"lantern/internal/fingerprints"
	"lantern/internal/scanner"
	"lantern/internal/ui"
)

func main() {
	fs := flag.NewFlagSet("discover", flag.ExitOnError)
	cfgPath := fs.String("config", "network.yaml", "path to network.yaml")
	fs.Parse(os.Args[1:])

	printer := ui.NewTerminalPrinter()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	subnet, err := scanner.DetectSubnet()
	if err != nil {
		printer.Info("Error detecting subnet: " + err.Error())
		os.Exit(1)
	}

	printer.Info("Scanning " + subnet + "...")

	client := &http.Client{Timeout: 2 * time.Second}
	reg := &scanner.Registry{}
	reg.Register(fingerprints.NewFrigate(client))
	reg.Register(fingerprints.NewTrueNAS(client))
	reg.Register(fingerprints.NewMainsail(client))

	results, err := scanner.Run(ctx, subnet, reg)
	if err != nil {
		printer.Info("Error scanning: " + err.Error())
		os.Exit(1)
	}

	if len(results) == 0 {
		printer.Info("No services identified. Check your network interface.")
		os.Exit(0)
	}

	printer.Info(fmt.Sprintf("Found %d active hosts\n", len(results)))
	printer.Info("Identified services:")
	for _, r := range results {
		printer.Info(fmt.Sprintf("  ✓ %-12s %s  (port %d)", r.Result.Name, r.IP, r.Result.Port))
	}

	printer.Info("\nWrite to " + *cfgPath + "? [y/N]")
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	if !strings.EqualFold(strings.TrimSpace(line), "y") {
		printer.Info("Aborted.")
		os.Exit(0)
	}

	// Load existing config or create a minimal one.
	cfg, err := config.Read(*cfgPath)
	if err != nil {
		cfg = config.Config{Version: 1}
	}

	// Merge discovered IPs (preserve existing fields, update IPs).
	existing := make(map[string]*config.Service)
	for i := range cfg.Services {
		existing[cfg.Services[i].Name] = &cfg.Services[i]
	}
	for _, r := range results {
		if svc, ok := existing[r.Result.Name]; ok {
			svc.IP = r.IP
			svc.Port = r.Result.Port
		} else {
			cfg.Services = append(cfg.Services, config.Service{
				Name: r.Result.Name,
				IP:   r.IP,
				Port: r.Result.Port,
			})
		}
	}

	if err := cfg.Write(*cfgPath); err != nil {
		printer.Info("Error writing config: " + err.Error())
		os.Exit(1)
	}
	printer.Success(*cfgPath, "updated")
}
