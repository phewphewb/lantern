package writers_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lantern/internal/config"
	xec "lantern/internal/exec"
	"lantern/internal/paths"
	"lantern/internal/writers"
)

func testPaths(t *testing.T) paths.Paths {
	t.Helper()
	dir := t.TempDir()
	p := paths.Default()
	p.NginxSitesDir = dir + "/nginx/"
	p.DnsmasqConf = filepath.Join(dir, "local-services.conf")
	p.CertDir = dir + "/certs/"
	os.MkdirAll(p.NginxSitesDir, 0755)
	os.MkdirAll(p.CertDir, 0755)
	return p
}

func testConfig() config.Config {
	return config.Config{
		Version:      1,
		DomainSuffix: "home",
		ProxyIP:      "192.168.2.10",
		CertWarnDays: 30,
		Services: []config.Service{
			{Name: "frigate", IP: "192.168.2.10", Port: 5000, WebSocket: true},
			{Name: "truenas", IP: "192.168.2.20", Port: 80},
		},
	}
}

// --- Nginx writer ---

func TestNginxWriter_CreatesConfPerService(t *testing.T) {
	p := testPaths(t)
	cfg := testConfig()
	var cmds []string
	e := xec.NewDryRunExecutor(func(c string) { cmds = append(cmds, c) })

	w := writers.NewNginx(p, e)
	if err := w.Write(cfg); err != nil {
		t.Fatalf("Write: %v", err)
	}

	for _, svc := range cfg.Services {
		confPath := p.NginxConfFor(svc.Name, cfg.DomainSuffix)
		if _, err := os.Stat(confPath); os.IsNotExist(err) {
			t.Errorf("expected conf file %q to exist", confPath)
		}
	}
}

func TestNginxWriter_WebSocket_HasUpgradeHeaders(t *testing.T) {
	p := testPaths(t)
	cfg := testConfig()
	e := xec.NewDryRunExecutor(func(string) {})

	w := writers.NewNginx(p, e)
	if err := w.Write(cfg); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// frigate has websocket: true
	frigateConf := p.NginxConfFor("frigate", cfg.DomainSuffix)
	data, _ := os.ReadFile(frigateConf)
	if !strings.Contains(string(data), "proxy_set_header Upgrade") {
		t.Errorf("websocket service conf missing Upgrade header: %s", data)
	}
}

func TestNginxWriter_NonWebSocket_NoUpgradeHeaders(t *testing.T) {
	p := testPaths(t)
	cfg := testConfig()
	e := xec.NewDryRunExecutor(func(string) {})

	w := writers.NewNginx(p, e)
	if err := w.Write(cfg); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// truenas has websocket: false
	truenasConf := p.NginxConfFor("truenas", cfg.DomainSuffix)
	data, _ := os.ReadFile(truenasConf)
	if strings.Contains(string(data), "proxy_set_header Upgrade") {
		t.Errorf("non-websocket service conf has unexpected Upgrade header\nfile content:\n%s", data)
	}
}

func TestNginxWriter_Reload_CallsSystemctl(t *testing.T) {
	p := testPaths(t)
	var cmds []string
	e := xec.NewDryRunExecutor(func(c string) { cmds = append(cmds, c) })

	w := writers.NewNginx(p, e)
	if err := w.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	found := false
	for _, c := range cmds {
		if strings.Contains(c, "systemctl") && strings.Contains(c, "nginx") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected systemctl nginx command, got: %v", cmds)
	}
}

func TestNginxWriter_WriteCA_CreatesConfWithIPAndPath(t *testing.T) {
	p := testPaths(t)
	e := xec.NewDryRunExecutor(func(string) {})

	w := writers.NewNginx(p, e)
	if err := w.WriteCA("192.168.2.10", "/home/user/.local/share/mkcert"); err != nil {
		t.Fatalf("WriteCA: %v", err)
	}

	data, err := os.ReadFile(p.NginxCAConf())
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "192.168.2.10") {
		t.Errorf("CA conf missing proxy IP: %s", content)
	}
	if !strings.Contains(content, "/home/user/.local/share/mkcert/rootCA.pem") {
		t.Errorf("CA conf missing CA cert path: %s", content)
	}
	if !strings.Contains(content, "/ca.crt") {
		t.Errorf("CA conf missing /ca.crt location: %s", content)
	}
}

// --- Dnsmasq writer ---

func TestDnsmasqWriter_OneLinePerService(t *testing.T) {
	p := testPaths(t)
	cfg := testConfig()
	e := xec.NewDryRunExecutor(func(string) {})

	w := writers.NewDnsmasq(p, e)
	if err := w.Write(cfg); err != nil {
		t.Fatalf("Write: %v", err)
	}

	data, err := os.ReadFile(p.DnsmasqConf)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)

	for _, svc := range cfg.Services {
		expected := svc.IP
		if !strings.Contains(content, expected) {
			t.Errorf("dnsmasq conf missing IP %q for service %q", expected, svc.Name)
		}
		domain := svc.Name + "." + cfg.DomainSuffix
		if !strings.Contains(content, domain) {
			t.Errorf("dnsmasq conf missing domain %q", domain)
		}
	}
}

func TestDnsmasqWriter_Reload_CallsSystemctl(t *testing.T) {
	p := testPaths(t)
	var cmds []string
	e := xec.NewDryRunExecutor(func(c string) { cmds = append(cmds, c) })

	w := writers.NewDnsmasq(p, e)
	if err := w.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	found := false
	for _, c := range cmds {
		if strings.Contains(c, "systemctl") && strings.Contains(c, "dnsmasq") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected systemctl dnsmasq command, got: %v", cmds)
	}
}
