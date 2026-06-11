package scanner

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// ScanResult holds the outcome for a single probed IP.
type ScanResult struct {
	IP         string
	Result     Result
	Identified bool
	Metadata   Metadata
}

// Metadata contains optional facts discovered about a host.
type Metadata struct {
	Hostname string
}

// HostProbe reports whether a host appears active on the network.
type HostProbe func(ctx context.Context, ip string) bool

// MetadataLookup returns optional metadata for a host.
type MetadataLookup func(ctx context.Context, ip string) Metadata

// Run concurrently probes every host in subnet using registry.
// subnet must be a CIDR string (e.g. "192.168.2.0/24").
// Returns identified hosts.
func Run(ctx context.Context, subnet string, registry *Registry) ([]ScanResult, error) {
	return RunDevices(ctx, subnet, registry, nil)
}

// RunDevices concurrently probes every host in subnet using registry and, when
// no known service matches, hostProbe. It returns both identified services and
// active unidentified hosts.
func RunDevices(ctx context.Context, subnet string, registry *Registry, hostProbe HostProbe, metadataLookups ...MetadataLookup) ([]ScanResult, error) {
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
				metadata := lookupMetadata(ctx, ip, metadataLookups)
				mu.Lock()
				results = append(results, ScanResult{
					IP:         ip,
					Result:     result,
					Identified: true,
					Metadata:   metadata,
				})
				mu.Unlock()
				return
			}
			if hostProbe != nil && hostProbe(ctx, ip) {
				metadata := lookupMetadata(ctx, ip, metadataLookups)
				mu.Lock()
				results = append(results, ScanResult{
					IP:       ip,
					Metadata: metadata,
				})
				mu.Unlock()
			}
		}(ip)
	}

	wg.Wait()
	return results, nil
}

func lookupMetadata(ctx context.Context, ip string, metadataLookups []MetadataLookup) Metadata {
	var metadata Metadata
	for _, lookup := range metadataLookups {
		next := lookup(ctx, ip)
		if metadata.Hostname == "" {
			metadata.Hostname = next.Hostname
		}
	}
	return metadata
}

// NewTCPHostProbe returns an unprivileged host probe that attempts TCP
// connections to common LAN service ports.
func NewTCPHostProbe(timeout time.Duration, ports ...int) HostProbe {
	if len(ports) == 0 {
		ports = []int{22, 53, 80, 443, 445, 5000, 7125, 8080, 8123, 8971}
	}
	return func(ctx context.Context, ip string) bool {
		for _, port := range ports {
			select {
			case <-ctx.Done():
				return false
			default:
			}

			dialer := net.Dialer{Timeout: timeout}
			conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(ip, fmt.Sprintf("%d", port)))
			if err != nil {
				continue
			}
			conn.Close()
			return true
		}
		return false
	}
}

// NewHostnameLookup returns a metadata lookup that tries reverse DNS for a host.
func NewHostnameLookup(timeout time.Duration) MetadataLookup {
	resolver := net.DefaultResolver
	return func(ctx context.Context, ip string) Metadata {
		lookupCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		names, err := resolver.LookupAddr(lookupCtx, ip)
		if err != nil || len(names) == 0 {
			return Metadata{}
		}
		return Metadata{Hostname: strings.TrimSuffix(names[0], ".")}
	}
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
