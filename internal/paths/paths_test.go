package paths_test

import (
	"strings"
	"testing"

	"lantern/internal/paths"
)

func TestDefaultPaths(t *testing.T) {
	p := paths.Default()

	if p.NginxSitesDir != "/etc/nginx/sites-enabled/" {
		t.Errorf("NginxSitesDir: got %q", p.NginxSitesDir)
	}
	if p.DnsmasqConf != "/etc/dnsmasq.d/local-services.conf" {
		t.Errorf("DnsmasqConf: got %q", p.DnsmasqConf)
	}
	if p.CertDir != "/etc/ssl/local/" {
		t.Errorf("CertDir: got %q", p.CertDir)
	}
	if p.BackupRoot != "/var/backups/lantern/" {
		t.Errorf("BackupRoot: got %q", p.BackupRoot)
	}
}

func TestNginxConfFor(t *testing.T) {
	p := paths.Default()
	got := p.NginxConfFor("frigate", "home")
	want := "/etc/nginx/sites-enabled/frigate.home.conf"
	if got != want {
		t.Errorf("NginxConfFor: got %q want %q", got, want)
	}
}

func TestCertFileFor(t *testing.T) {
	p := paths.Default()
	got := p.CertFileFor("truenas", "home")
	if !strings.HasSuffix(got, "truenas.home.crt") {
		t.Errorf("CertFileFor: got %q, want suffix truenas.home.crt", got)
	}
}

func TestKeyFileFor(t *testing.T) {
	p := paths.Default()
	got := p.KeyFileFor("truenas", "home")
	if !strings.HasSuffix(got, "truenas.home.key") {
		t.Errorf("KeyFileFor: got %q, want suffix tuenas.home.key", got)
	}
}
