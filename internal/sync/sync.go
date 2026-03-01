package sync

import (
	"context"
	"fmt"
	"net"

	"lantern/internal/backup"
	"lantern/internal/config"
	"lantern/internal/paths"
	"lantern/internal/scanner"
	"lantern/internal/ui"
)

// Identifier resolves a service name and IP for a given host IP.
// scanner.Registry satisfies this interface.
type Identifier interface {
	Identify(ctx context.Context, ip string) (scanner.Result, bool)
}

// Run probes each host in the subnet, diffs the results against cfg,
// and runs setup if any service IP has changed. configPath is the path to
// network.yaml so it can be updated on disk.
func Run(
	ctx context.Context,
	cfg config.Config,
	configPath string,
	p paths.Paths,
	id Identifier,
	setup func(config.Config) error,
	printer ui.Printer,
) error {
	// Build a map of service name → current configured IP.
	configured := make(map[string]config.Service, len(cfg.Services))
	for _, svc := range cfg.Services {
		configured[svc.Name] = svc
	}

	// Scan all hosts in the subnet inferred from the proxy IP.
	subnet, err := subnetFor(cfg.ProxyIP)
	if err != nil {
		return fmt.Errorf("detecting subnet: %w", err)
	}
	ips, err := hostsInSubnet(subnet)
	if err != nil {
		return fmt.Errorf("listing hosts: %w", err)
	}

	// Probe each host and collect results by service name.
	found := make(map[string]scanner.Result) // name → result
	for _, ip := range ips {
		result, ok := id.Identify(ctx, ip)
		if !ok {
			continue
		}
		found[result.Name] = result
	}

	// Diff found IPs against configured IPs.
	var changes []ipChange
	for _, svc := range cfg.Services {
		result, ok := found[svc.Name]
		if !ok {
			printer.Warn(svc.Name, "not found during scan — skipping")
			continue
		}
		if result.IP != svc.IP {
			changes = append(changes, ipChange{name: svc.Name, oldIP: svc.IP, newIP: result.IP})
		}
	}

	if len(changes) == 0 {
		printer.Info("All service IPs unchanged. Nothing to do.")
		return nil
	}

	// Report changes.
	printer.Info("IP change detected:")
	for _, c := range changes {
		printer.Info(fmt.Sprintf("  %s  %s → %s", c.name, c.oldIP, c.newIP))
	}

	// Back up existing config before modifying.
	backupDir, err := backup.Create(p)
	if err != nil {
		return fmt.Errorf("creating backup: %w", err)
	}

	// Apply changes to config.
	updated := cfg
	services := make([]config.Service, len(cfg.Services))
	copy(services, cfg.Services)
	for i, svc := range services {
		for _, c := range changes {
			if svc.Name == c.name {
				services[i].IP = c.newIP
			}
		}
	}
	updated.Services = services

	// Persist updated config.
	printer.Info("Updating network.yaml...")
	if err := updated.Write(configPath); err != nil {
		return fmt.Errorf("writing updated config: %w", err)
	}
	printer.Success("network.yaml", "updated")

	// Run setup with updated config.
	printer.Info("Running setup...")
	if err := setup(updated); err != nil {
		printer.Warn("setup", "failed — restoring backup")
		// Restore original config.
		if restoreErr := updated.Write(configPath); restoreErr != nil {
			// Can't easily restore config — best effort.
			_ = restoreErr
		}
		// Restore config file from backup (original YAML).
		_ = backup.Restore(backupDir, p)
		// Rewrite original config on disk.
		if writeErr := cfg.Write(configPath); writeErr != nil {
			return fmt.Errorf("setup failed: %w; also failed to restore config: %v", err, writeErr)
		}
		return fmt.Errorf("setup failed: %w", err)
	}
	printer.Success("setup", "ok")
	printer.Info("Reconfigured successfully.")
	return nil
}

type ipChange struct {
	name  string
	oldIP string
	newIP string
}

// subnetFor returns the /24 CIDR for the given IP address.
// e.g. "192.168.2.10" → "192.168.2.0/24"
func subnetFor(ip string) (string, error) {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return "", fmt.Errorf("invalid IP %q", ip)
	}
	ip4 := parsed.To4()
	if ip4 == nil {
		return "", fmt.Errorf("not an IPv4 address: %q", ip)
	}
	return fmt.Sprintf("%d.%d.%d.0/24", ip4[0], ip4[1], ip4[2]), nil
}

// hostsInSubnet returns all usable host IPs in a CIDR.
func hostsInSubnet(cidr string) ([]string, error) {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}
	var hosts []string
	for addr := cloneIP(network.IP); network.Contains(addr); incrementIP(addr) {
		host := addr.String()
		if host == network.IP.String() {
			continue
		}
		broadcast := broadcastAddr(network)
		if host == broadcast {
			continue
		}
		hosts = append(hosts, host)
	}
	return hosts, nil
}

func cloneIP(ip net.IP) net.IP {
	c := make(net.IP, len(ip))
	copy(c, ip)
	return c
}

func incrementIP(ip net.IP) {
	for i := len(ip) - 1; i >= 0; i-- {
		ip[i]++
		if ip[i] != 0 {
			break
		}
	}
}

func broadcastAddr(network *net.IPNet) string {
	ip := cloneIP(network.IP)
	for i := range ip {
		ip[i] |= ^network.Mask[i]
	}
	return ip.String()
}
