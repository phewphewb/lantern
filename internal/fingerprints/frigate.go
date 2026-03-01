package fingerprints

import (
	"context"
	"fmt"
	"net/http"

	"lantern/internal/scanner"
)

// FrigateFingerprinter probes the Frigate NVR API.
// It tries the modern port (8971) then the legacy port (5000).
type FrigateFingerprinter struct {
	client *http.Client
}

func NewFrigate(client *http.Client) *FrigateFingerprinter {
	return &FrigateFingerprinter{client: client}
}

func (f *FrigateFingerprinter) Name() string { return "frigate" }

func (f *FrigateFingerprinter) Probe(ctx context.Context, ip string) (scanner.Result, bool) {
	type probe struct {
		scheme string
		port   int
	}
	for _, p := range []probe{{"https", 8971}, {"http", 5000}} {
		url := fmt.Sprintf("%s://%s:%d/api/version", p.scheme, ip, p.port)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			continue
		}
		resp, err := f.client.Do(req)
		if err != nil {
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return scanner.Result{Name: "frigate", IP: ip, Port: p.port}, true
		}
	}
	return scanner.Result{}, false
}
