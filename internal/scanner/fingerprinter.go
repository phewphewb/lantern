package scanner

import "context"

// Result holds the identification outcome for a single host.
type Result struct {
	Name string
	IP   string
	Port int
}

// Fingerprinter attempts to identify a known service at a given IP.
type Fingerprinter interface {
	// Name returns the service label (e.g. "frigate").
	Name() string

	// Probe attempts to identify the service at ip.
	// Returns (result, true) on match, (zero, false) on no match or error.
	Probe(ctx context.Context, ip string) (Result, bool)
}

// Registry holds all known Fingerprinters and identifies hosts against them.
type Registry struct {
	fingerprinters []Fingerprinter
}

// Register adds a Fingerprinter to the registry.
func (r *Registry) Register(f Fingerprinter) {
	r.fingerprinters = append(r.fingerprinters, f)
}

// Identify tries each registered fingerprinter in order.
// Returns the first match, or (zero, false) if none matched.
func (r *Registry) Identify(ctx context.Context, ip string) (Result, bool) {
	for _, f := range r.fingerprinters {
		if result, ok := f.Probe(ctx, ip); ok {
			return result, true
		}
	}
	return Result{}, false
}
