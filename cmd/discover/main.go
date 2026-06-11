package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"sort"
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
	listOnly := fs.Bool("list", false, "list discovered devices without writing network.yaml")
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

	client := &http.Client{
		Timeout:   2 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
	}
	reg := &scanner.Registry{}
	reg.Register(fingerprints.NewFrigate(client))
	reg.Register(fingerprints.NewTrueNAS(client))
	reg.Register(fingerprints.NewMainsail(client))

	results, err := scanner.RunDevices(
		ctx,
		subnet,
		reg,
		scanner.NewTCPHostProbe(300*time.Millisecond),
		scanner.NewHostnameLookup(500*time.Millisecond),
	)
	if err != nil {
		printer.Info("Error scanning: " + err.Error())
		os.Exit(1)
	}
	sortScanResults(results)

	if len(results) == 0 {
		printer.Info("No active hosts found. Check your network interface.")
		os.Exit(0)
	}

	printer.Info(fmt.Sprintf("Found %d active hosts\n", len(results)))
	identified := identifiedResults(results)
	unidentified := unidentifiedResults(results)

	if len(identified) > 0 {
		printer.Info("Identified services:")
		for _, r := range identified {
			printer.Info(fmt.Sprintf("  ✓ %-12s %s  (port %d)%s", r.Result.Name, r.IP, r.Result.Port, metadataDetail(r.Metadata)))
		}
	} else {
		printer.Info("No known services identified.")
	}

	if len(unidentified) > 0 {
		printer.Info("\nCould not identify:")
		for _, r := range unidentified {
			printer.Info(fmt.Sprintf("  ? %s%s", r.IP, metadataDetail(r.Metadata)))
		}
	}

	if *listOnly {
		os.Exit(0)
	}

	if len(identified) == 0 {
		printer.Info("\nNo services to write to " + *cfgPath + ".")
		os.Exit(0)
	}

	printer.Info("\nWrite identified services to " + *cfgPath + "? [y/N]")
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
	for _, r := range identified {
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

func identifiedResults(results []scanner.ScanResult) []scanner.ScanResult {
	var identified []scanner.ScanResult
	for _, r := range results {
		if r.Identified {
			identified = append(identified, r)
		}
	}
	return identified
}

func unidentifiedResults(results []scanner.ScanResult) []scanner.ScanResult {
	var unidentified []scanner.ScanResult
	for _, r := range results {
		if !r.Identified {
			unidentified = append(unidentified, r)
		}
	}
	return unidentified
}

func sortScanResults(results []scanner.ScanResult) {
	sort.Slice(results, func(i, j int) bool {
		left, leftErr := netip.ParseAddr(results[i].IP)
		right, rightErr := netip.ParseAddr(results[j].IP)
		if leftErr == nil && rightErr == nil {
			return left.Less(right)
		}
		return results[i].IP < results[j].IP
	})
}

func metadataDetail(metadata scanner.Metadata) string {
	if metadata.Hostname == "" {
		return ""
	}
	return "  host: " + metadata.Hostname
}
