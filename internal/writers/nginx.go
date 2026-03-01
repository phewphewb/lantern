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

//go:embed templates/nginx-service.conf.tmpl
var nginxTmplFS embed.FS

type NginxWriter struct {
	paths    paths.Paths
	executor xec.Executor
	tmpl     *template.Template
}

func NewNginx(p paths.Paths, e xec.Executor) *NginxWriter {
	tmpl := template.Must(
		template.ParseFS(nginxTmplFS, "templates/nginx-service.conf.tmpl"),
	)
	return &NginxWriter{paths: p, executor: e, tmpl: tmpl}
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

func (w *NginxWriter) Reload() error {
	return w.executor.Run("systemctl", "restart", "nginx")
}
