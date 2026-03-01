package fingerprints

import (
	"context"
	"fmt"
	"net/http"

	"lantern/internal/scanner"
)

// MainsailFingerprinter probes the Moonraker API that backs Mainsail.
type MainsailFingerprinter struct {
	client *http.Client
}

func NewMainsail(client *http.Client) *MainsailFingerprinter {
	return &MainsailFingerprinter{client: client}
}

func (f *MainsailFingerprinter) Name() string { return "mainsail" }

func (f *MainsailFingerprinter) Probe(ctx context.Context, ip string) (scanner.Result, bool) {
	url := fmt.Sprintf("http://%s:7125/printer/info", ip)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return scanner.Result{}, false
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return scanner.Result{}, false
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return scanner.Result{Name: "mainsail", IP: ip, Port: 7125}, true
	}
	return scanner.Result{}, false
}
