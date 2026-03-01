package certs

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"time"

	"lantern/internal/config"
	xec "lantern/internal/exec"
	"lantern/internal/paths"
)

// CertStatus describes the expiry state of a single service certificate.
type CertStatus struct {
	Name     string
	CertFile string
	Found    bool
	DaysLeft int
	NotAfter time.Time
}

// Status reads each service's certificate and returns its expiry information.
// Missing certs are returned with Found=false; no error is returned for them.
func Status(p paths.Paths, cfg config.Config) ([]CertStatus, error) {
	results := make([]CertStatus, 0, len(cfg.Services))
	for _, svc := range cfg.Services {
		certFile := p.CertFileFor(svc.Name, cfg.DomainSuffix)
		cs := CertStatus{Name: svc.Name, CertFile: certFile}

		data, err := os.ReadFile(certFile)
		if os.IsNotExist(err) {
			results = append(results, cs)
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("reading cert for %s: %w", svc.Name, err)
		}

		block, _ := pem.Decode(data)
		if block == nil {
			return nil, fmt.Errorf("no PEM block in cert for %s", svc.Name)
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parsing cert for %s: %w", svc.Name, err)
		}

		cs.Found = true
		cs.NotAfter = cert.NotAfter
		cs.DaysLeft = int(time.Until(cert.NotAfter).Hours() / 24)
		results = append(results, cs)
	}
	return results, nil
}

// Renew runs mkcert for services whose certs expire within warnDays.
// If all is true, every service cert is renewed regardless of expiry.
func Renew(p paths.Paths, cfg config.Config, e xec.Executor, warnDays int, all bool) error {
	statuses, err := Status(p, cfg)
	if err != nil {
		return err
	}

	for _, cs := range statuses {
		if !all && cs.Found && cs.DaysLeft > warnDays {
			continue
		}
		domain := cs.Name + "." + cfg.DomainSuffix
		certFile := p.CertFileFor(cs.Name, cfg.DomainSuffix)
		keyFile := p.KeyFileFor(cs.Name, cfg.DomainSuffix)
		if err := e.Run("mkcert",
			"-cert-file", certFile,
			"-key-file", keyFile,
			domain,
		); err != nil {
			return fmt.Errorf("mkcert for %s: %w", cs.Name, err)
		}
	}
	return nil
}
