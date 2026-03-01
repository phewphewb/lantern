package writers

import "lantern/internal/config"

// ConfigWriter generates and applies config files for a single service
// (nginx, dnsmasq, etc.). The setup engine works against this interface —
// it has no knowledge of the concrete format.
type ConfigWriter interface {
	Write(cfg config.Config) error
	Reload() error
}
