package sync_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"lantern/internal/config"
	"lantern/internal/paths"
	"lantern/internal/scanner"
	syncp "lantern/internal/sync"
	"lantern/internal/ui"
)

func testPaths(t *testing.T) paths.Paths {
	t.Helper()
	dir := t.TempDir()
	p := paths.Default()
	p.NginxSitesDir = dir + "/nginx/"
	p.DnsmasqConf = filepath.Join(dir, "local-services.conf")
	p.BackupRoot = dir + "/backups/"
	os.MkdirAll(p.NginxSitesDir, 0755)
	os.MkdirAll(p.BackupRoot, 0755)
	return p
}

func testConfig(t *testing.T, p paths.Paths) (config.Config, string) {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "network.yaml")
	cfg := config.Config{
		Version:      1,
		DomainSuffix: "home",
		ProxyIP:      "192.168.2.10",
		CertWarnDays: 30,
		Services: []config.Service{
			{Name: "frigate", IP: "192.168.2.10", Port: 5000, WebSocket: true},
			{Name: "truenas", IP: "192.168.2.20", Port: 80},
		},
	}
	if err := cfg.Write(cfgPath); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return cfg, cfgPath
}

// mockRegistry returns predetermined results per IP.
type mockRegistry struct {
	results map[string]scanner.Result // ip → result
}

func (m *mockRegistry) Identify(ctx context.Context, ip string) (scanner.Result, bool) {
	r, ok := m.results[ip]
	return r, ok
}

// TestRun_NoChange verifies that when all services are at their configured
// IPs, setup is not called and the config is not modified.
func TestRun_NoChange(t *testing.T) {
	p := testPaths(t)
	cfg, cfgPath := testConfig(t, p)

	reg := &mockRegistry{results: map[string]scanner.Result{
		"192.168.2.10": {Name: "frigate", IP: "192.168.2.10", Port: 5000},
		"192.168.2.20": {Name: "truenas", IP: "192.168.2.20", Port: 80},
	}}

	setupCalled := false
	setup := func(c config.Config) error {
		setupCalled = true
		return nil
	}

	if err := syncp.Run(context.Background(), cfg, cfgPath, p, reg, setup, ui.NewNullPrinter()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if setupCalled {
		t.Error("setup should not be called when no IPs changed")
	}
}

// TestRun_IPChange verifies that when a service IP changes, the config is
// updated and setup is called.
func TestRun_IPChange(t *testing.T) {
	p := testPaths(t)
	cfg, cfgPath := testConfig(t, p)

	// truenas moved from .20 to .21
	reg := &mockRegistry{results: map[string]scanner.Result{
		"192.168.2.10": {Name: "frigate", IP: "192.168.2.10", Port: 5000},
		"192.168.2.21": {Name: "truenas", IP: "192.168.2.21", Port: 80},
	}}

	setupCalled := false
	var setupCfg config.Config
	setup := func(c config.Config) error {
		setupCalled = true
		setupCfg = c
		return nil
	}

	if err := syncp.Run(context.Background(), cfg, cfgPath, p, reg, setup, ui.NewNullPrinter()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !setupCalled {
		t.Fatal("expected setup to be called after IP change")
	}

	// Config on disk should have updated IP.
	updated, err := config.Read(cfgPath)
	if err != nil {
		t.Fatalf("reading updated config: %v", err)
	}
	var found bool
	for _, svc := range updated.Services {
		if svc.Name == "truenas" && svc.IP == "192.168.2.21" {
			found = true
		}
	}
	if !found {
		t.Errorf("updated config does not have truenas at 192.168.2.21: %+v", updated.Services)
	}

	// Setup was called with updated config.
	for _, svc := range setupCfg.Services {
		if svc.Name == "truenas" && svc.IP != "192.168.2.21" {
			t.Errorf("setup called with stale IP %q for truenas", svc.IP)
		}
	}
}

// TestRun_UnreachableService verifies that a service not found during scan
// is skipped (not treated as an IP change) and no error is returned.
func TestRun_UnreachableService(t *testing.T) {
	p := testPaths(t)
	cfg, cfgPath := testConfig(t, p)

	// Only frigate responds; truenas is not found anywhere.
	reg := &mockRegistry{results: map[string]scanner.Result{
		"192.168.2.10": {Name: "frigate", IP: "192.168.2.10", Port: 5000},
	}}

	setupCalled := false
	setup := func(c config.Config) error {
		setupCalled = true
		return nil
	}

	if err := syncp.Run(context.Background(), cfg, cfgPath, p, reg, setup, ui.NewNullPrinter()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if setupCalled {
		t.Error("setup should not be called when only unreachable services differ")
	}
}

// TestRun_SetupFailureRestores verifies that when setup fails after an IP
// change, the config is restored from backup.
func TestRun_SetupFailureRestores(t *testing.T) {
	p := testPaths(t)
	cfg, cfgPath := testConfig(t, p)

	// truenas moved; setup will fail.
	reg := &mockRegistry{results: map[string]scanner.Result{
		"192.168.2.10": {Name: "frigate", IP: "192.168.2.10", Port: 5000},
		"192.168.2.21": {Name: "truenas", IP: "192.168.2.21", Port: 80},
	}}

	setupErr := errors.New("setup failed")
	setup := func(c config.Config) error { return setupErr }

	err := syncp.Run(context.Background(), cfg, cfgPath, p, reg, setup, ui.NewNullPrinter())
	if err == nil {
		t.Fatal("expected error when setup fails")
	}

	// Config should be restored to original IPs.
	restored, err := config.Read(cfgPath)
	if err != nil {
		t.Fatalf("reading restored config: %v", err)
	}
	for _, svc := range restored.Services {
		if svc.Name == "truenas" && svc.IP != "192.168.2.20" {
			t.Errorf("config not restored: truenas IP=%q, want 192.168.2.20", svc.IP)
		}
	}
}
