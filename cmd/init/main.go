package main

import (
	"flag"
	"os"

	"lantern/internal/ui"
)

const defaultConfig = `version: 1

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
`

func main() {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	cfgPath := fs.String("config", "network.yaml", "output path")
	force := fs.Bool("force", false, "overwrite if file already exists")
	fs.Parse(os.Args[1:])

	printer := ui.NewTerminalPrinter()

	if _, err := os.Stat(*cfgPath); err == nil && !*force {
		printer.Info("Error: " + *cfgPath + " already exists. Use --force to overwrite.")
		os.Exit(1)
	}

	printer.Info("Writing " + *cfgPath + "...")
	if err := os.WriteFile(*cfgPath, []byte(defaultConfig), 0644); err != nil {
		printer.Info("Error: " + err.Error())
		os.Exit(1)
	}
	printer.Success(*cfgPath, "created")

	printer.Info(`
Next steps:
  1. Run discover to fill in service IPs automatically:
       ./lantern discover
  2. Or edit ` + *cfgPath + ` manually, then validate:
       ./lantern validate --ping`)
}
