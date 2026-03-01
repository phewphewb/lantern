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

//go:embed templates/dnsmasq.conf.tmpl
var dnsmasqTmplFS embed.FS

type DnsmasqWriter struct {
	paths    paths.Paths
	executor xec.Executor
	tmpl     *template.Template
}

func NewDnsmasq(p paths.Paths, e xec.Executor) *DnsmasqWriter {
	tmpl := template.Must(
		template.ParseFS(dnsmasqTmplFS, "templates/dnsmasq.conf.tmpl"),
	)
	return &DnsmasqWriter{paths: p, executor: e, tmpl: tmpl}
}

type dnsmasqData struct {
	DomainSuffix string
	Services     []config.Service
}

func (w *DnsmasqWriter) Write(cfg config.Config) error {
	f, err := os.Create(w.paths.DnsmasqConf)
	if err != nil {
		return fmt.Errorf("creating dnsmasq conf: %w", err)
	}
	defer f.Close()

	data := dnsmasqData{
		DomainSuffix: cfg.DomainSuffix,
		Services:     cfg.Services,
	}
	if err := w.tmpl.Execute(f, data); err != nil {
		return fmt.Errorf("rendering dnsmasq template: %w", err)
	}
	return nil
}

func (w *DnsmasqWriter) Reload() error {
	return w.executor.Run("systemctl", "restart", "dnsmasq")
}
