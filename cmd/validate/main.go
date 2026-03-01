package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"time"

	"lantern/internal/config"
	"lantern/internal/ui"
)

func main() {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	cfgPath := fs.String("config", "network.yaml", "path to network.yaml")
	ping := fs.Bool("ping", false, "verify each service IP is reachable on its port")
	fs.Parse(os.Args[1:])

	printer := ui.NewTerminalPrinter()

	printer.Info("Validating " + *cfgPath + "...")

	cfg, err := config.Read(*cfgPath)
	if err != nil {
		printer.Info("  ✗ " + err.Error())
		os.Exit(1)
	}
	printer.Info(fmt.Sprintf("  ✓ version supported (%d)", cfg.Version))

	if err := cfg.Validate(); err != nil {
		printer.Info("  ✗ " + err.Error())
		printer.Info("\n1 error found. Fix network.yaml and run validate again.")
		os.Exit(1)
	}
	printer.Info("  ✓ domain_suffix present")
	printer.Info("  ✓ proxy_ip valid (" + cfg.ProxyIP + ")")
	printer.Info(fmt.Sprintf("  ✓ %d services defined, no duplicate names", len(cfg.Services)))
	printer.Info("  ✓ all ports in valid range")

	if *ping {
		var failures int
		for _, svc := range cfg.Services {
			addr := fmt.Sprintf("%s:%d", svc.IP, svc.Port)
			conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
			if err != nil {
				printer.Info(fmt.Sprintf("  ✗ %s  unreachable  (%s)", addr, svc.Name))
				failures++
				continue
			}
			conn.Close()
			printer.Info(fmt.Sprintf("  ✓ %s  reachable  (%s)", addr, svc.Name))
		}
		if failures > 0 {
			os.Exit(1)
		}
	}

	printer.Info("\nAll checks passed.")
}
