package certs_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"lantern/internal/certs"
	"lantern/internal/config"
	xec "lantern/internal/exec"
	"lantern/internal/paths"
)

// writeSelfSignedCert creates a self-signed cert that expires at notAfter
// and writes it to path in PEM format.
func writeSelfSignedCert(t *testing.T, path string, notAfter time.Time) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     notAfter,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create %s: %v", path, err)
	}
	defer f.Close()
	pem.Encode(f, &pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func testPaths(t *testing.T) paths.Paths {
	t.Helper()
	dir := t.TempDir()
	p := paths.Default()
	p.CertDir = dir + "/certs/"
	os.MkdirAll(p.CertDir, 0755)
	return p
}

func testConfig() config.Config {
	return config.Config{
		Version:      1,
		DomainSuffix: "home",
		CertWarnDays: 30,
		Services: []config.Service{
			{Name: "frigate", IP: "192.168.2.10", Port: 5000},
			{Name: "truenas", IP: "192.168.2.20", Port: 80},
		},
	}
}

// TestStatus_ParsesExpiryDays checks that Status reads the cert and returns
// the correct days-remaining value.
func TestStatus_ParsesExpiryDays(t *testing.T) {
	p := testPaths(t)
	cfg := testConfig()

	// frigate cert expires in 90 days.
	expiry := time.Now().Add(90 * 24 * time.Hour)
	certPath := p.CertFileFor("frigate", cfg.DomainSuffix)
	writeSelfSignedCert(t, certPath, expiry)

	results, err := certs.Status(p, cfg)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}

	var found bool
	for _, r := range results {
		if r.Name == "frigate" {
			found = true
			if !r.Found {
				t.Errorf("frigate cert: Found=false")
			}
			// Allow ±1 day for clock skew in CI.
			if r.DaysLeft < 89 || r.DaysLeft > 91 {
				t.Errorf("frigate cert: DaysLeft=%d, want ~90", r.DaysLeft)
			}
		}
	}
	if !found {
		t.Errorf("no result for frigate")
	}
}

// TestStatus_MissingCert checks that a missing cert file is reported as
// Found=false without error.
func TestStatus_MissingCert(t *testing.T) {
	p := testPaths(t)
	cfg := testConfig()
	// No cert files written.

	results, err := certs.Status(p, cfg)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}

	for _, r := range results {
		if r.Found {
			t.Errorf("service %q: expected Found=false for missing cert", r.Name)
		}
	}
}

// TestRenew_CallsMkcert checks that Renew invokes mkcert with the correct
// arguments for a cert that is expiring soon.
func TestRenew_CallsMkcert(t *testing.T) {
	p := testPaths(t)
	cfg := testConfig()

	// truenas cert expires in 10 days — within warnDays=30.
	certPath := p.CertFileFor("truenas", cfg.DomainSuffix)
	writeSelfSignedCert(t, certPath, time.Now().Add(10*24*time.Hour))

	// frigate cert is healthy (90 days).
	writeSelfSignedCert(t, p.CertFileFor("frigate", cfg.DomainSuffix), time.Now().Add(90*24*time.Hour))

	var cmds []string
	e := xec.NewDryRunExecutor(func(c string) { cmds = append(cmds, c) })

	if err := certs.Renew(p, cfg, e, cfg.CertWarnDays, false); err != nil {
		t.Fatalf("Renew: %v", err)
	}

	// Only truenas should be renewed.
	if len(cmds) != 1 {
		t.Fatalf("expected 1 mkcert call, got %d: %v", len(cmds), cmds)
	}
	if !strings.Contains(cmds[0], "truenas") {
		t.Errorf("mkcert call missing truenas: %s", cmds[0])
	}
	if strings.Contains(cmds[0], "frigate") {
		t.Errorf("mkcert call should not include frigate: %s", cmds[0])
	}
}

// TestRenew_All checks that Renew --all renews every cert regardless of expiry.
func TestRenew_All(t *testing.T) {
	p := testPaths(t)
	cfg := testConfig()

	// Both certs healthy (90 days).
	for _, svc := range cfg.Services {
		writeSelfSignedCert(t, p.CertFileFor(svc.Name, cfg.DomainSuffix), time.Now().Add(90*24*time.Hour))
	}

	var cmds []string
	e := xec.NewDryRunExecutor(func(c string) { cmds = append(cmds, c) })

	if err := certs.Renew(p, cfg, e, cfg.CertWarnDays, true); err != nil {
		t.Fatalf("Renew all: %v", err)
	}

	if len(cmds) != len(cfg.Services) {
		t.Errorf("expected %d mkcert calls (one per service), got %d: %v", len(cfg.Services), len(cmds), cmds)
	}
}

// TestRenew_MkcertArgs verifies the mkcert command uses -cert-file and -key-file flags.
func TestRenew_MkcertArgs(t *testing.T) {
	p := testPaths(t)
	cfg := config.Config{
		Version:      1,
		DomainSuffix: "home",
		CertWarnDays: 30,
		Services:     []config.Service{{Name: "frigate", IP: "192.168.2.10", Port: 5000}},
	}

	// Cert expires in 5 days.
	writeSelfSignedCert(t, p.CertFileFor("frigate", cfg.DomainSuffix), time.Now().Add(5*24*time.Hour))

	var cmds []string
	e := xec.NewDryRunExecutor(func(c string) { cmds = append(cmds, c) })

	if err := certs.Renew(p, cfg, e, cfg.CertWarnDays, false); err != nil {
		t.Fatalf("Renew: %v", err)
	}

	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	cmd := cmds[0]
	if !strings.Contains(cmd, "-cert-file") {
		t.Errorf("mkcert missing -cert-file: %s", cmd)
	}
	if !strings.Contains(cmd, "-key-file") {
		t.Errorf("mkcert missing -key-file: %s", cmd)
	}
	if !strings.Contains(cmd, "frigate.home") {
		t.Errorf("mkcert missing domain frigate.home: %s", cmd)
	}
}
