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
}

func (t *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base, _ := url.Parse(t.target)
	req2 := req.Clone(req.Context())
	req2.URL.Host = base.Host
	req2.URL.Scheme = base.Scheme
	return http.DefaultTransport.RoundTrip(req2)
}

func testClient(serverURL string) *http.Client {
	return &http.Client{Transport: &redirectTransport{target: serverURL}}
}

// --- Frigate ---

func TestFrigateFingerprinter_MatchesOnPort5000(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/version" {
			w.WriteHeader(http.StatusOK)
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

// --- TrueNAS ---

func TestTrueNASFingerprinter_Match(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2.0/system/version" {
			w.WriteHeader(http.StatusOK)
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

// --- Mainsail ---

func TestMainsailFingerprinter_Match(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/printer/info" {
			w.WriteHeader(http.StatusOK)
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
