package paths

import "fmt"

// Paths is the single source of truth for every file and directory the tool
// manages. No other package hardcodes path strings.
type Paths struct {
	NginxSitesDir string // /etc/nginx/sites-enabled/
	DnsmasqConf   string // /etc/dnsmasq.d/local-services.conf
	CertDir       string // /etc/ssl/local/
	BackupRoot    string // /var/backups/lantern/
	MkcertCARoot  string // ~/.local/share/mkcert/
}

// Default returns the production path set.
func Default() Paths {
	return Paths{
		NginxSitesDir: "/etc/nginx/sites-enabled/",
		DnsmasqConf:   "/etc/dnsmasq.d/local-services.conf",
		CertDir:       "/etc/ssl/local/",
		BackupRoot:    "/var/backups/lantern/",
		MkcertCARoot:  "~/.local/share/mkcert/",
	}
}

func (p Paths) NginxConfFor(service, domainSuffix string) string {
	return fmt.Sprintf("%s%s.%s.conf", p.NginxSitesDir, service, domainSuffix)
}

func (p Paths) CertFileFor(service, domainSuffix string) string {
	return fmt.Sprintf("%s%s.%s.crt", p.CertDir, service, domainSuffix)
}

func (p Paths) KeyFileFor(service, domainSuffix string) string {
	return fmt.Sprintf("%s%s.%s.key", p.CertDir, service, domainSuffix)
}
