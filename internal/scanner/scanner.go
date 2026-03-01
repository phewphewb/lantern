package scanner

import (
	"context"
	"fmt"
	"net"
	"sync"
)

// ScanResult holds the outcome for a single probed IP.
type ScanResult struct {
	IP         string
	Result     Result
	Identified bool
}

// Run concurrently probes every host in subnet using registry.
// subnet must be a CIDR string (e.g. "192.168.2.0/24").
// Returns identified and unidentified active hosts.
func Run(ctx context.Context, subnet string, registry *Registry) ([]ScanResult, error) {
	ips, err := hostsInSubnet(subnet)
	if err != nil {
		return nil, fmt.Errorf("parsing subnet: %w", err)
	}

	results := make([]ScanResult, 0)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, ip := range ips {
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			result, ok := registry.Identify(ctx, ip)
			if ok {
				mu.Lock()
				results = append(results, ScanResult{IP: ip, Result: result, Identified: true})
				mu.Unlock()
			}
		}(ip)
	}

	wg.Wait()
	return results, nil
}

// DetectSubnet returns the first non-loopback IPv4 CIDR found on a local interface.
func DetectSubnet() (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", fmt.Errorf("listing interfaces: %w", err)
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			var network *net.IPNet
			switch v := addr.(type) {
			case *net.IPNet:
				ip, network = v.IP, v
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			if ip4 := ip.To4(); ip4 != nil && network != nil {
				return network.String(), nil
			}
		}
	}
	return "", fmt.Errorf("no active non-loopback IPv4 interface found")
}

// hostsInSubnet returns all usable host addresses in the given CIDR.
func hostsInSubnet(cidr string) ([]string, error) {
	ip, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}
	_ = ip
	var hosts []string
	for addr := cloneIP(network.IP); network.Contains(addr); incrementIP(addr) {
		// Skip network address and broadcast.
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
	clone := make(net.IP, len(ip))
	copy(clone, ip)
	return clone
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
