package backup

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"lantern/internal/paths"
)

// Create creates a timestamped backup directory under p.BackupRoot and copies
// all managed config files into it. Returns the path to the backup directory.
// Files that don't exist yet are silently skipped.
func Create(p paths.Paths) (string, error) {
	stamp := time.Now().Format("2006-01-02-150405")
	backupDir := filepath.Join(p.BackupRoot, stamp)
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", fmt.Errorf("creating backup dir: %w", err)
	}

	// Copy nginx .conf files.
	entries, err := os.ReadDir(p.NginxSitesDir)
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("reading nginx sites dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".conf" {
			continue
		}
		src := filepath.Join(p.NginxSitesDir, e.Name())
		dst := filepath.Join(backupDir, e.Name())
		if err := copyFile(src, dst); err != nil {
			return "", fmt.Errorf("backing up %s: %w", e.Name(), err)
		}
	}

	// Copy dnsmasq conf if it exists.
	if _, err := os.Stat(p.DnsmasqConf); err == nil {
		dst := filepath.Join(backupDir, filepath.Base(p.DnsmasqConf))
		if err := copyFile(p.DnsmasqConf, dst); err != nil {
			return "", fmt.Errorf("backing up dnsmasq conf: %w", err)
		}
	}

	return backupDir, nil
}

// Restore copies all files from backupDir back to their original managed
// locations. Nginx confs go to p.NginxSitesDir; the dnsmasq conf goes to
// p.DnsmasqConf (matched by base name).
func Restore(backupDir string, p paths.Paths) error {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return fmt.Errorf("reading backup dir: %w", err)
	}
	dnsmasqBase := filepath.Base(p.DnsmasqConf)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		src := filepath.Join(backupDir, name)
		var dst string
		if name == dnsmasqBase {
			dst = p.DnsmasqConf
		} else {
			dst = filepath.Join(p.NginxSitesDir, name)
		}
		if err := copyFile(src, dst); err != nil {
			return fmt.Errorf("restoring %s: %w", name, err)
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
