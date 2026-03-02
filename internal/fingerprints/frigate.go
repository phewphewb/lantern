package fingerprints

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

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
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		// Frigate's /api/version returns a JSON-encoded version string (e.g. "0.14.1").
		// Require the body to start with '"' to reject HTML catch-all responses.
		if resp.StatusCode == http.StatusOK && strings.HasPrefix(strings.TrimSpace(string(body)), `"`) {
			return scanner.Result{Name: "frigate", IP: ip, Port: p.port}, true
		}
	}
	return scanner.Result{}, false
}
