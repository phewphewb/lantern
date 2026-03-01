package backup_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"lantern/internal/backup"
	"lantern/internal/paths"
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

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
}

// TestCreate_TimestampedDir checks that Create makes a directory whose name
// starts with a date stamp and returns its path.
func TestCreate_TimestampedDir(t *testing.T) {
	p := testPaths(t)

	before := time.Now().UTC().Format("2006-01-02")

	backupDir, err := backup.Create(p)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	info, err := os.Stat(backupDir)
	if err != nil {
		t.Fatalf("backup dir not found: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("backup path is not a directory: %s", backupDir)
	}
	if !strings.HasPrefix(filepath.Base(backupDir), before) {
		t.Errorf("backup dir %q does not start with today's date %q", backupDir, before)
	}
}

// TestCreate_CopiesNginxConfs checks that .conf files from NginxSitesDir
// are copied into the backup.
func TestCreate_CopiesNginxConfs(t *testing.T) {
	p := testPaths(t)
	writeFile(t, filepath.Join(p.NginxSitesDir, "frigate.home.conf"), "server { # frigate }")
	writeFile(t, filepath.Join(p.NginxSitesDir, "truenas.home.conf"), "server { # truenas }")

	backupDir, err := backup.Create(p)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	for _, name := range []string{"frigate.home.conf", "truenas.home.conf"} {
		dest := filepath.Join(backupDir, name)
		if _, err := os.Stat(dest); err != nil {
			t.Errorf("expected backup of %q but not found: %v", name, err)
		}
	}
}

// TestCreate_CopiesDnsmasqConf checks that the dnsmasq conf is backed up.
func TestCreate_CopiesDnsmasqConf(t *testing.T) {
	p := testPaths(t)
	writeFile(t, p.DnsmasqConf, "address=/frigate.home/192.168.2.10")

	backupDir, err := backup.Create(p)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	dest := filepath.Join(backupDir, filepath.Base(p.DnsmasqConf))
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("dnsmasq backup not found: %v", err)
	}
	if string(data) != "address=/frigate.home/192.168.2.10" {
		t.Errorf("unexpected dnsmasq backup content: %q", string(data))
	}
}

// TestCreate_EmptyDirs creates a backup when no managed files exist yet —
// should succeed with an empty backup dir.
func TestCreate_EmptyDirs(t *testing.T) {
	p := testPaths(t)
	// dnsmasq conf does not exist; nginx dir is empty

	backupDir, err := backup.Create(p)
	if err != nil {
		t.Fatalf("Create with empty dirs: %v", err)
	}
	if _, err := os.Stat(backupDir); err != nil {
		t.Errorf("backup dir not created: %v", err)
	}
}

// TestRestore_RestoresFiles verifies that Restore copies backup files back
// to their original locations.
func TestRestore_RestoresFiles(t *testing.T) {
	p := testPaths(t)

	// Write original files.
	origNginx := "server { # original }"
	origDns := "address=/old.home/192.168.2.10"
	writeFile(t, filepath.Join(p.NginxSitesDir, "svc.home.conf"), origNginx)
	writeFile(t, p.DnsmasqConf, origDns)

	backupDir, err := backup.Create(p)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Overwrite with new content.
	writeFile(t, filepath.Join(p.NginxSitesDir, "svc.home.conf"), "server { # new }")
	writeFile(t, p.DnsmasqConf, "address=/new.home/192.168.2.99")

	if err := backup.Restore(backupDir, p); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	// nginx conf should be restored.
	data, _ := os.ReadFile(filepath.Join(p.NginxSitesDir, "svc.home.conf"))
	if string(data) != origNginx {
		t.Errorf("nginx conf not restored: got %q", string(data))
	}

	// dnsmasq conf should be restored.
	data, _ = os.ReadFile(p.DnsmasqConf)
	if string(data) != origDns {
		t.Errorf("dnsmasq conf not restored: got %q", string(data))
	}
}
