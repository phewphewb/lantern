package config

import (
	"fmt"
	"net"
	"os"

	"gopkg.in/yaml.v3"
)

const supportedVersion = 1

type Monitor struct {
	CheckInterval string `yaml:"check_interval"`
	LogFile       string `yaml:"log_file,omitempty"`
	LogMaxSize    string `yaml:"log_max_size,omitempty"`
}

type Service struct {
	Name          string `yaml:"name"`
	IP            string `yaml:"ip"`
	Port          int    `yaml:"port"`
	WebSocket     bool   `yaml:"websocket,omitempty"`
	MoonrakerPort int    `yaml:"moonraker_port,omitempty"`
}

type Config struct {
	Version      int       `yaml:"version"`
	DomainSuffix string    `yaml:"domain_suffix"`
	ProxyIP      string    `yaml:"proxy_ip"`
	CertWarnDays int       `yaml:"cert_warn_days"`
	Monitor      Monitor   `yaml:"monitor"`
	Services     []Service `yaml:"services"`
}

// Read parses a network.yaml file. Returns an error if the file cannot be
// read, the YAML is malformed, or the version field is unsupported.
func Read(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("reading config file: %w", err)
	}

	// Peek at the version before full unmarshal.
	var peek struct {
		Version int `yaml:"version"`
	}
	if err := yaml.Unmarshal(data, &peek); err != nil {
		return Config{}, fmt.Errorf("parsing config: %w", err)
	}
	if peek.Version != supportedVersion {
		return Config{}, fmt.Errorf("unsupported version %d (supported: %d)", peek.Version, supportedVersion)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing config: %w", err)
	}
	return cfg, nil
}

// Write serialises the config to YAML and writes it to path.
func (c Config) Write(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshalling config: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}
	return nil
}

// Validate checks the config for semantic correctness. It does not touch
// the filesystem or network.
func (c Config) Validate() error {
	if c.Version != supportedVersion {
		return fmt.Errorf("unsupported version %d (supported: %d)", c.Version, supportedVersion)
	}
	if c.DomainSuffix == "" {
		return fmt.Errorf("domain_suffix is required")
	}
	if c.ProxyIP == "" {
		return fmt.Errorf("proxy_ip is required")
	}
	if net.ParseIP(c.ProxyIP) == nil {
		return fmt.Errorf("proxy_ip %q is not a valid IPv4 address", c.ProxyIP)
	}
	if len(c.Services) == 0 {
		return fmt.Errorf("at least one service must be defined")
	}

	seen := make(map[string]bool)
	for i, svc := range c.Services {
		if svc.Name == "" {
			return fmt.Errorf("service[%d]: name is required", i)
		}
		if seen[svc.Name] {
			return fmt.Errorf("duplicate service name %q", svc.Name)
		}
		seen[svc.Name] = true
		if net.ParseIP(svc.IP) == nil {
			return fmt.Errorf("service %q: ip %q is not a valid IPv4 address", svc.Name, svc.IP)
		}
		if svc.Port < 1 || svc.Port > 65535 {
			return fmt.Errorf("service %q: port %d is not valid (must be 1–65535)", svc.Name, svc.Port)
		}
	}
	return nil
}
