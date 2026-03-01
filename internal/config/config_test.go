package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lantern/internal/config"
)

// --- Validate ---

func TestValidate_ValidConfig(t *testing.T) {
	cfg := validConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidate_MissingDomainSuffix(t *testing.T) {
	cfg := validConfig()
	cfg.DomainSuffix = ""
	assertValidateError(t, cfg, "domain_suffix")
}

func TestValidate_MissingProxyIP(t *testing.T) {
	cfg := validConfig()
	cfg.ProxyIP = ""
	assertValidateError(t, cfg, "proxy_ip")
}

func TestValidate_InvalidProxyIP(t *testing.T) {
	cfg := validConfig()
	cfg.ProxyIP = "999.999.999.999"
	assertValidateError(t, cfg, "proxy_ip")
}

func TestValidate_NoServices(t *testing.T) {
	cfg := validConfig()
	cfg.Services = nil
	assertValidateError(t, cfg, "service")
}

func TestValidate_ServiceMissingName(t *testing.T) {
	cfg := validConfig()
	cfg.Services[0].Name = ""
	assertValidateError(t, cfg, "name")
}

func TestValidate_ServiceInvalidIP(t *testing.T) {
	cfg := validConfig()
	cfg.Services[0].IP = "not-an-ip"
	assertValidateError(t, cfg, "ip")
}

func TestValidate_ServicePortZero(t *testing.T) {
	cfg := validConfig()
	cfg.Services[0].Port = 0
	assertValidateError(t, cfg, "port")
}

func TestValidate_ServicePortTooHigh(t *testing.T) {
	cfg := validConfig()
	cfg.Services[0].Port = 99999
	assertValidateError(t, cfg, "port")
}

func TestValidate_DuplicateServiceNames(t *testing.T) {
	cfg := validConfig()
	cfg.Services = append(cfg.Services, cfg.Services[0])
	assertValidateError(t, cfg, "duplicate")
}

func TestValidate_UnsupportedVersion(t *testing.T) {
	cfg := validConfig()
	cfg.Version = 99
	assertValidateError(t, cfg, "version")
}

// --- Read / Write round-trip ---

func TestReadWrite_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "network.yaml")

	original := validConfig()
	if err := original.Write(path); err != nil {
		t.Fatalf("Write: %v", err)
	}

	loaded, err := config.Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if loaded.DomainSuffix != original.DomainSuffix {
		t.Errorf("DomainSuffix: got %q want %q", loaded.DomainSuffix, original.DomainSuffix)
	}
	if loaded.ProxyIP != original.ProxyIP {
		t.Errorf("ProxyIP: got %q want %q", loaded.ProxyIP, original.ProxyIP)
	}
	if len(loaded.Services) != len(original.Services) {
		t.Errorf("Services len: got %d want %d", len(loaded.Services), len(original.Services))
	}
	if loaded.Services[0].Name != original.Services[0].Name {
		t.Errorf("Service name: got %q want %q", loaded.Services[0].Name, original.Services[0].Name)
	}
}

func TestRead_UnsupportedVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "network.yaml")
	if err := os.WriteFile(path, []byte("version: 99\ndomain_suffix: home\n"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := config.Read(path)
	if err == nil {
		t.Fatal("expected error for unsupported version, got nil")
	}
}

func TestRead_MissingFile(t *testing.T) {
	_, err := config.Read("/nonexistent/network.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

// --- helpers ---

func validConfig() config.Config {
	return config.Config{
		Version:      1,
		DomainSuffix: "home",
		ProxyIP:      "192.168.2.10",
		CertWarnDays: 30,
		Monitor: config.Monitor{
			CheckInterval: "5m",
		},
		Services: []config.Service{
			{Name: "frigate", IP: "192.168.2.10", Port: 5000, WebSocket: true},
			{Name: "truenas", IP: "192.168.2.20", Port: 80},
		},
	}
}

func assertValidateError(t *testing.T, cfg config.Config, wantSubstr string) {
	t.Helper()
	err := cfg.Validate()
	if err == nil {
		t.Fatalf("expected validation error containing %q, got nil", wantSubstr)
	}
	if !strings.Contains(err.Error(), wantSubstr) {
		t.Errorf("expected error containing %q, got: %v", wantSubstr, err)
	}
}
