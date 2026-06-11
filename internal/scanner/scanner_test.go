package scanner_test

import (
	"context"
	"testing"

	"lantern/internal/scanner"
)

// mockFingerprinter matches a single known IP.
type mockFingerprinter struct {
	name string
	ip   string
	port int
}

func (m *mockFingerprinter) Name() string { return m.name }

func (m *mockFingerprinter) Probe(ctx context.Context, ip string) (scanner.Result, bool) {
	if ip == m.ip {
		return scanner.Result{Name: m.name, IP: ip, Port: m.port}, true
	}
	return scanner.Result{}, false
}

func TestRegistry_IdentifiesMatch(t *testing.T) {
	reg := &scanner.Registry{}
	reg.Register(&mockFingerprinter{name: "frigate", ip: "192.168.2.10", port: 5000})
	reg.Register(&mockFingerprinter{name: "truenas", ip: "192.168.2.20", port: 80})

	result, ok := reg.Identify(context.Background(), "192.168.2.10")
	if !ok {
		t.Fatal("expected match, got none")
	}
	if result.Name != "frigate" {
		t.Errorf("Name=%q, want frigate", result.Name)
	}
}

func TestRegistry_NoMatch(t *testing.T) {
	reg := &scanner.Registry{}
	reg.Register(&mockFingerprinter{name: "frigate", ip: "192.168.2.10", port: 5000})

	_, ok := reg.Identify(context.Background(), "10.0.0.1")
	if ok {
		t.Error("expected no match for unknown IP")
	}
}

func TestRun_FindsKnownService(t *testing.T) {
	reg := &scanner.Registry{}
	reg.Register(&mockFingerprinter{name: "frigate", ip: "192.168.2.10", port: 5000})

	results, err := scanner.Run(context.Background(), "192.168.2.0/24", reg)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	var found bool
	for _, r := range results {
		if r.Result.Name == "frigate" && r.IP == "192.168.2.10" {
			found = true
		}
	}
	if !found {
		t.Errorf("frigate not found in scan results: %+v", results)
	}
}

func TestRunDevices_IncludesUnidentifiedActiveHosts(t *testing.T) {
	reg := &scanner.Registry{}
	reg.Register(&mockFingerprinter{name: "frigate", ip: "192.168.2.1", port: 5000})
	hostProbe := func(ctx context.Context, ip string) bool {
		return ip == "192.168.2.2"
	}

	results, err := scanner.RunDevices(context.Background(), "192.168.2.0/30", reg, hostProbe)
	if err != nil {
		t.Fatalf("RunDevices: %v", err)
	}

	var foundKnown, foundUnknown bool
	for _, r := range results {
		switch r.IP {
		case "192.168.2.1":
			if r.Identified && r.Result.Name == "frigate" {
				foundKnown = true
			}
		case "192.168.2.2":
			if !r.Identified {
				foundUnknown = true
			}
		}
	}
	if !foundKnown {
		t.Errorf("known service not found in scan results: %+v", results)
	}
	if !foundUnknown {
		t.Errorf("unidentified active host not found in scan results: %+v", results)
	}
}

func TestRunDevices_AttachesMetadata(t *testing.T) {
	reg := &scanner.Registry{}
	reg.Register(&mockFingerprinter{name: "frigate", ip: "192.168.2.1", port: 5000})
	hostProbe := func(ctx context.Context, ip string) bool {
		return ip == "192.168.2.2"
	}
	metadataLookup := func(ctx context.Context, ip string) scanner.Metadata {
		switch ip {
		case "192.168.2.1":
			return scanner.Metadata{Hostname: "frigate.local"}
		case "192.168.2.2":
			return scanner.Metadata{Hostname: "printer.local"}
		default:
			return scanner.Metadata{}
		}
	}

	results, err := scanner.RunDevices(context.Background(), "192.168.2.0/30", reg, hostProbe, metadataLookup)
	if err != nil {
		t.Fatalf("RunDevices: %v", err)
	}

	hostnames := make(map[string]string)
	for _, r := range results {
		hostnames[r.IP] = r.Metadata.Hostname
	}
	if hostnames["192.168.2.1"] != "frigate.local" {
		t.Errorf("known host metadata=%q, want frigate.local", hostnames["192.168.2.1"])
	}
	if hostnames["192.168.2.2"] != "printer.local" {
		t.Errorf("unknown host metadata=%q, want printer.local", hostnames["192.168.2.2"])
	}
}

func TestRun_ExcludesUnidentifiedActiveHosts(t *testing.T) {
	reg := &scanner.Registry{}

	results, err := scanner.Run(context.Background(), "192.168.2.0/30", reg)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Run returned unidentified hosts: %+v", results)
	}
}

func TestRun_ContextCancellation(t *testing.T) {
	// A fingerprinter that blocks until context is cancelled.
	blocking := &blockingFingerprinter{}
	reg := &scanner.Registry{}
	reg.Register(blocking)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// Should complete without hanging.
	results, err := scanner.Run(ctx, "192.168.2.0/24", reg)
	if err != nil {
		t.Fatalf("Run with cancelled ctx: %v", err)
	}
	_ = results // may be empty; that's fine
}

func TestRun_InvalidSubnet(t *testing.T) {
	reg := &scanner.Registry{}
	_, err := scanner.Run(context.Background(), "not-a-cidr", reg)
	if err == nil {
		t.Error("expected error for invalid subnet, got nil")
	}
}

// blockingFingerprinter returns immediately when ctx is done.
type blockingFingerprinter struct{}

func (b *blockingFingerprinter) Name() string { return "blocking" }
func (b *blockingFingerprinter) Probe(ctx context.Context, ip string) (scanner.Result, bool) {
	<-ctx.Done()
	return scanner.Result{}, false
}
