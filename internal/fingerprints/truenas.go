package fingerprints

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"lantern/internal/scanner"
)

// TrueNASFingerprinter probes the TrueNAS Scale REST API.
type TrueNASFingerprinter struct {
	client *http.Client
}

func NewTrueNAS(client *http.Client) *TrueNASFingerprinter {
	return &TrueNASFingerprinter{client: client}
}

func (f *TrueNASFingerprinter) Name() string { return "truenas" }

func (f *TrueNASFingerprinter) Probe(ctx context.Context, ip string) (scanner.Result, bool) {
	url := fmt.Sprintf("http://%s:80/api/v2.0/system/version", ip)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return scanner.Result{}, false
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return scanner.Result{}, false
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK && strings.Contains(string(body), "TrueNAS") {
		return scanner.Result{Name: "truenas", IP: ip, Port: 80}, true
	}
	return scanner.Result{}, false
}
