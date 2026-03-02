package writers

import (
	"embed"
	"fmt"
	"os"
	"text/template"

	"lantern/internal/config"
	xec "lantern/internal/exec"
	"lantern/internal/paths"
)

//go:embed templates/nginx-service.conf.tmpl templates/nginx-ca.conf.tmpl
var nginxTmplFS embed.FS

type NginxWriter struct {
	paths    paths.Paths
	executor xec.Executor
	tmpl     *template.Template
	caTmpl   *template.Template
}

func NewNginx(p paths.Paths, e xec.Executor) *NginxWriter {
	tmpl := template.Must(
		template.ParseFS(nginxTmplFS, "templates/nginx-service.conf.tmpl"),
	)
	caTmpl := template.Must(
		template.ParseFS(nginxTmplFS, "templates/nginx-ca.conf.tmpl"),
	)
	return &NginxWriter{paths: p, executor: e, tmpl: tmpl, caTmpl: caTmpl}
}

type nginxData struct {
	Domain    string
	IP        string
	Port      int
	CertFile  string
	KeyFile   string
	WebSocket bool
}

func (w *NginxWriter) Write(cfg config.Config) error {
	if err := os.MkdirAll(w.paths.NginxSitesDir, 0755); err != nil {
		return fmt.Errorf("creating nginx sites dir: %w", err)
	}
	for _, svc := range cfg.Services {
		domain := svc.Name + "." + cfg.DomainSuffix
		data := nginxData{
			Domain:    domain,
			IP:        svc.IP,
			Port:      svc.Port,
			CertFile:  w.paths.CertFileFor(svc.Name, cfg.DomainSuffix),
			KeyFile:   w.paths.KeyFileFor(svc.Name, cfg.DomainSuffix),
			WebSocket: svc.WebSocket,
		}
		confPath := w.paths.NginxConfFor(svc.Name, cfg.DomainSuffix)
		f, err := os.Create(confPath)
		if err != nil {
			return fmt.Errorf("creating nginx conf for %s: %w", svc.Name, err)
		}
		if err := w.tmpl.Execute(f, data); err != nil {
			f.Close()
			return fmt.Errorf("rendering nginx template for %s: %w", svc.Name, err)
		}
		f.Close()
	}
	return nil
}

type nginxCAData struct {
	ProxyIP   string
	CARootDir string
}

// WriteCA writes a dedicated nginx config that serves the mkcert CA certificate
// at http://<proxyIP>/ca.crt so client devices can download and trust it.
// caRootDir is the directory returned by `mkcert -CAROOT`.
func (w *NginxWriter) WriteCA(proxyIP, caRootDir string) error {
	if caRootDir != "" && caRootDir[len(caRootDir)-1] != '/' {
		caRootDir += "/"
	}
	data := nginxCAData{ProxyIP: proxyIP, CARootDir: caRootDir}
	f, err := os.Create(w.paths.NginxCAConf())
	if err != nil {
		return fmt.Errorf("creating nginx CA conf: %w", err)
	}
	defer f.Close()
	if err := w.caTmpl.Execute(f, data); err != nil {
		return fmt.Errorf("rendering nginx CA template: %w", err)
	}
	return nil
}

func (w *NginxWriter) Reload() error {
	return w.executor.Run("systemctl", "restart", "nginx")
}
