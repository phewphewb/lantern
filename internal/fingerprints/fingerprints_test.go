package fingerprints_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"lantern/internal/fingerprints"
)

// redirectTransport rewrites any request's host:port to the given target,
// keeping the path intact. This lets fingerprinters probe "real" IPs in tests
// while actually hitting the httptest.Server.
type redirectTransport struct {
	target string // e.g. "http://127.0.0.1:PORT"
	inner  http.RoundTripper
}

func (t *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base, _ := url.Parse(t.target)
	req2 := req.Clone(req.Context())
	req2.URL.Host = base.Host
	req2.URL.Scheme = base.Scheme
	inner := t.inner
	if inner == nil {
		inner = http.DefaultTransport
	}
	return inner.RoundTrip(req2)
}

func testClient(serverURL string) *http.Client {
	return &http.Client{Transport: &redirectTransport{target: serverURL}}
}

func testTLSClient(ts *httptest.Server) *http.Client {
	return &http.Client{Transport: &redirectTransport{target: ts.URL, inner: ts.Client().Transport}}
}

// --- Frigate ---

func TestFrigateFingerprinter_MatchesOnPort5000(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/version" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`"0.14.1"`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	f := fingerprints.NewFrigate(testClient(ts.URL))
	result, ok := f.Probe(context.Background(), "192.168.2.10")
	if !ok {
		t.Fatal("expected match, got none")
	}
	if result.Name != "frigate" {
		t.Errorf("Name=%q, want frigate", result.Name)
	}
	if result.IP != "192.168.2.10" {
		t.Errorf("IP=%q, want 192.168.2.10", result.IP)
	}
}

func TestFrigateFingerprinter_NoMatch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	f := fingerprints.NewFrigate(testClient(ts.URL))
	_, ok := f.Probe(context.Background(), "192.168.2.10")
	if ok {
		t.Error("expected no match but got one")
	}
}

func TestFrigateFingerprinter_MatchesOnPort8971TLS(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/version" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`"0.14.1"`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	f := fingerprints.NewFrigate(testTLSClient(ts))
	result, ok := f.Probe(context.Background(), "192.168.2.10")
	if !ok {
		t.Fatal("expected match on HTTPS port 8971, got none")
	}
	if result.Port != 8971 {
		t.Errorf("Port=%d, want 8971", result.Port)
	}
}

// Regression: a device returning 200 with an HTML body on port 5000/8971
// must not be identified as Frigate.
func TestFrigateFingerprinter_NoMatchOnCatchAll200(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<html><body>some device</body></html>`))
	}))
	defer ts.Close()

	f := fingerprints.NewFrigate(testClient(ts.URL))
	_, ok := f.Probe(context.Background(), "192.168.2.10")
	if ok {
		t.Error("expected no match for device returning HTML on 200, got match")
	}
}

// --- TrueNAS ---

func TestTrueNASFingerprinter_Match(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2.0/system/version" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`"TrueNAS-SCALE-24.10.2"`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	f := fingerprints.NewTrueNAS(testClient(ts.URL))
	result, ok := f.Probe(context.Background(), "192.168.2.20")
	if !ok {
		t.Fatal("expected match, got none")
	}
	if result.Name != "truenas" {
		t.Errorf("Name=%q, want truenas", result.Name)
	}
	if result.Port != 80 {
		t.Errorf("Port=%d, want 80", result.Port)
	}
}

func TestTrueNASFingerprinter_NoMatch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer ts.Close()

	f := fingerprints.NewTrueNAS(testClient(ts.URL))
	_, ok := f.Probe(context.Background(), "192.168.2.20")
	if ok {
		t.Error("expected no match but got one")
	}
}

// Regression: a Reolink camera returns HTTP 200 for many paths but the body
// does not contain "TrueNAS", so the fingerprinter must not match it.
func TestTrueNASFingerprinter_NoMatchOnCatchAll200(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulates a device (e.g. Reolink camera) that returns 200 for everything.
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<html><body>Reolink</body></html>`))
	}))
	defer ts.Close()

	f := fingerprints.NewTrueNAS(testClient(ts.URL))
	_, ok := f.Probe(context.Background(), "192.168.2.50")
	if ok {
		t.Error("expected no match for device returning 200 without TrueNAS body, got match")
	}
}

// --- Mainsail ---

func TestMainsailFingerprinter_Match(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/printer/info" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"result":{"state":"ready","klipper_path":"/home/pi/klipper"}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	f := fingerprints.NewMainsail(testClient(ts.URL))
	result, ok := f.Probe(context.Background(), "192.168.2.30")
	if !ok {
		t.Fatal("expected match, got none")
	}
	if result.Name != "mainsail" {
		t.Errorf("Name=%q, want mainsail", result.Name)
	}
	if result.Port != 7125 {
		t.Errorf("Port=%d, want 7125", result.Port)
	}
}

func TestMainsailFingerprinter_NoMatch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	f := fingerprints.NewMainsail(testClient(ts.URL))
	_, ok := f.Probe(context.Background(), "192.168.2.30")
	if ok {
		t.Error("expected no match but got one")
	}
}

// Regression: a device returning 200 with non-Moonraker body on port 7125
// must not be identified as Mainsail.
func TestMainsailFingerprinter_NoMatchOnCatchAll200(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<html><body>some device</body></html>`))
	}))
	defer ts.Close()

	f := fingerprints.NewMainsail(testClient(ts.URL))
	_, ok := f.Probe(context.Background(), "192.168.2.30")
	if ok {
		t.Error("expected no match for device returning HTML on 200, got match")
	}
}
