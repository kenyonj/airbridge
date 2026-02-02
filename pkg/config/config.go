// Package config provides configuration management for airbridge.
package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config holds the application configuration.
type Config struct {
	// HTTPPort is the base port for the HTTP server.
	// Each device renderer will use an incremental port starting from this.
	HTTPPort int `yaml:"http_port"`

	// AutoDiscover enables automatic discovery of AirPlay devices.
	// When true, a renderer is created for each discovered device.
	AutoDiscover bool `yaml:"auto_discover"`

	// DeviceFilter is a list of device names or patterns to include.
	// Empty means include all devices.
	DeviceFilter []string `yaml:"device_filter"`

	// Devices is a list of explicit device configurations.
	// These override auto-discovered settings.
	Devices []DeviceConfig `yaml:"devices"`

	// NamePrefix is prepended to the DLNA friendly name.
	NamePrefix string `yaml:"name_prefix"`
}

// DeviceConfig holds configuration for a single device.
type DeviceConfig struct {
	// Name is the AirPlay device name to target.
	Name string `yaml:"name"`

	// Alias is an optional friendly name for the DLNA renderer.
	// If empty, uses "Airbridge (<Name>)".
	Alias string `yaml:"alias"`

	// Port is an optional specific HTTP port for this renderer.
	Port int `yaml:"port"`

	// Enabled can be set to false to skip this device.
	Enabled *bool `yaml:"enabled"`

	// Volume is the default volume (0-100).
	Volume int `yaml:"volume"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		HTTPPort:     8200,
		AutoDiscover: true,
		NamePrefix:   "Airbridge",
	}
}

// Load reads configuration from a file.
func Load(path string) (Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil // Use defaults if no config file
		}
		return cfg, err
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}

// FindConfigFile looks for a config file in standard locations.
func FindConfigFile() string {
	// Check common locations
	paths := []string{
		"config.yaml",
		"config.yml",
		"airbridge.yaml",
		"airbridge.yml",
	}

	// Add user config dir
	if configDir, err := os.UserConfigDir(); err == nil {
		paths = append(paths,
			filepath.Join(configDir, "airbridge", "config.yaml"),
			filepath.Join(configDir, "airbridge", "config.yml"),
		)
	}

	// Add home dir
	if homeDir, err := os.UserHomeDir(); err == nil {
		paths = append(paths,
			filepath.Join(homeDir, ".config", "airbridge", "config.yaml"),
			filepath.Join(homeDir, ".airbridge.yaml"),
		)
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	return ""
}

// GetDeviceConfig returns configuration for a specific device.
// Returns nil if no explicit config exists.
func (c *Config) GetDeviceConfig(name string) *DeviceConfig {
	for i := range c.Devices {
		if c.Devices[i].Name == name {
			return &c.Devices[i]
		}
	}
	return nil
}

// ShouldIncludeDevice returns true if the device should be bridged.
func (c *Config) ShouldIncludeDevice(name string) bool {
	// Check explicit device configs first
	if dc := c.GetDeviceConfig(name); dc != nil {
		if dc.Enabled != nil && !*dc.Enabled {
			return false
		}
		return true
	}

	// If we have device filter, check it
	if len(c.DeviceFilter) > 0 {
		for _, filter := range c.DeviceFilter {
			if matchFilter(name, filter) {
				return true
			}
		}
		return false
	}

	// Auto-discover includes all devices by default
	return c.AutoDiscover
}

// matchFilter checks if a name matches a filter pattern.
// Simple substring match for now; could be expanded to glob patterns.
func matchFilter(name, filter string) bool {
	// Exact match
	if name == filter {
		return true
	}
	// Simple contains match
	if len(filter) > 0 && filter[0] == '*' && len(filter) > 1 {
		return contains(name, filter[1:])
	}
	if len(filter) > 0 && filter[len(filter)-1] == '*' {
		return hasPrefix(name, filter[:len(filter)-1])
	}
	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && (s[0:len(substr)] == substr || contains(s[1:], substr)))
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[0:len(prefix)] == prefix
}
