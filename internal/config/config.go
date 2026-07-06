// Package config loads, validates, and writes the Goblin Cloud TOML config.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config is the whole configuration file.
type Config struct {
	Server  Server  `toml:"server"`
	Auth    Auth    `toml:"auth"`
	Storage Storage `toml:"storage"`
	FTP     FTP     `toml:"ftp"`

	// path is where this config was loaded from (not serialized).
	path string
}

// Server holds HTTP / TLS settings.
type Server struct {
	Listen        string `toml:"listen"`
	Domain        string `toml:"domain"`
	AutocertEmail string `toml:"autocert_email"`
	AutocertCache string `toml:"autocert_cache"`
}

// Auth holds the single shared credential.
type Auth struct {
	Enabled      bool   `toml:"enabled"`
	PasswordHash string `toml:"password_hash"`
}

// Storage holds the balancing roots.
type Storage struct {
	Paths   []string `toml:"paths"`
	MinFree string   `toml:"min_free"`
}

// FTP holds the FTP front-door settings.
type FTP struct {
	Enabled      bool   `toml:"enabled"`
	Listen       string `toml:"listen"`
	TLS          bool   `toml:"tls"`
	PassivePorts string `toml:"passive_ports"`
}

// Default returns a config populated with the documented defaults.
func Default() Config {
	return Config{
		Server: Server{
			Listen:        ":8080",
			Domain:        "",
			AutocertEmail: "",
			AutocertCache: "/var/lib/goblin/certs",
		},
		Auth: Auth{
			Enabled:      true,
			PasswordHash: "",
		},
		Storage: Storage{
			Paths:   []string{"/var/lib/goblin/data"},
			MinFree: "1GB",
		},
		FTP: FTP{
			Enabled:      true,
			Listen:       ":2121",
			TLS:          false,
			PassivePorts: "30000-30100",
		},
	}
}

// Path returns the file this config was loaded from.
func (c Config) Path() string { return c.path }

// Global reports whether the server runs in domain/HTTPS mode.
func (c Config) Global() bool { return strings.TrimSpace(c.Server.Domain) != "" }

// Load reads and decodes the config at path, filling unset fields with defaults.
func Load(path string) (Config, error) {
	cfg := Default()
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}
	cfg.path = path
	return cfg, nil
}

// Resolve determines which config path to use, following:
// flag override, $GOBLIN_CONFIG, /etc/goblin/config.toml, ./config.toml.
func Resolve(flag string) string {
	if flag != "" {
		return flag
	}
	if env := os.Getenv("GOBLIN_CONFIG"); env != "" {
		return env
	}
	const system = "/etc/goblin/config.toml"
	if _, err := os.Stat(system); err == nil {
		return system
	}
	return "config.toml"
}

// MinFreeBytes parses Storage.MinFree into bytes.
func (c Config) MinFreeBytes() (uint64, error) {
	return ParseSize(c.Storage.MinFree)
}

// PassivePortRange parses FTP.PassivePorts ("start-end") into two ports.
func (c Config) PassivePortRange() (start, end int, err error) {
	s := strings.TrimSpace(c.FTP.PassivePorts)
	lo, hi, ok := strings.Cut(s, "-")
	if !ok {
		return 0, 0, fmt.Errorf("passive_ports %q must be \"start-end\"", s)
	}
	start, err = strconv.Atoi(strings.TrimSpace(lo))
	if err != nil {
		return 0, 0, fmt.Errorf("passive_ports start: %w", err)
	}
	end, err = strconv.Atoi(strings.TrimSpace(hi))
	if err != nil {
		return 0, 0, fmt.Errorf("passive_ports end: %w", err)
	}
	if start <= 0 || end <= 0 || start > end {
		return 0, 0, fmt.Errorf("passive_ports %q is not a valid range", s)
	}
	return start, end, nil
}

// ParseSize turns "1GB", "512MB", "2TB" (powers of 1024) into bytes.
func ParseSize(s string) (uint64, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	if s == "" {
		return 0, nil
	}
	units := []struct {
		suffix string
		mult   uint64
	}{
		{"TB", 1 << 40}, {"GB", 1 << 30}, {"MB", 1 << 20},
		{"KB", 1 << 10}, {"B", 1}, {"T", 1 << 40}, {"G", 1 << 30},
		{"M", 1 << 20}, {"K", 1 << 10},
	}
	for _, u := range units {
		if strings.HasSuffix(s, u.suffix) {
			num := strings.TrimSpace(strings.TrimSuffix(s, u.suffix))
			v, err := strconv.ParseFloat(num, 64)
			if err != nil {
				return 0, fmt.Errorf("size %q: %w", s, err)
			}
			return uint64(v * float64(u.mult)), nil
		}
	}
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("size %q: %w", s, err)
	}
	return v, nil
}

// Validate checks the config and returns a slice of problems (empty = valid).
func (c Config) Validate() []string {
	var probs []string

	if c.Global() && strings.TrimSpace(c.Server.AutocertEmail) == "" {
		probs = append(probs, "server.autocert_email is required when server.domain is set")
	}
	if c.Global() && strings.TrimSpace(c.Server.AutocertCache) == "" {
		probs = append(probs, "server.autocert_cache is required when server.domain is set")
	}
	if c.Auth.Enabled && strings.TrimSpace(c.Auth.PasswordHash) == "" {
		probs = append(probs, "auth.password_hash is empty; run `gcloud set password` or set auth.enabled=false")
	}
	if len(c.Storage.Paths) == 0 {
		probs = append(probs, "storage.paths must list at least one directory")
	}
	for _, p := range c.Storage.Paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			probs = append(probs, fmt.Sprintf("storage path %q is invalid: %v", p, err))
			continue
		}
		info, err := os.Stat(abs)
		if err != nil {
			probs = append(probs, fmt.Sprintf("storage path %q does not exist", p))
			continue
		}
		if !info.IsDir() {
			probs = append(probs, fmt.Sprintf("storage path %q is not a directory", p))
		}
	}
	if _, err := c.MinFreeBytes(); err != nil {
		probs = append(probs, fmt.Sprintf("storage.min_free: %v", err))
	}
	if c.FTP.Enabled {
		if _, _, err := c.PassivePortRange(); err != nil {
			probs = append(probs, fmt.Sprintf("ftp.%v", err))
		}
	}
	return probs
}
