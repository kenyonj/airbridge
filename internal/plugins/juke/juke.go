// Package juke implements the Juke Audio volume control plugin.
package juke

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/kenyonj/airbridge/internal/plugins"
)

const PluginType = "juke_audio"

// Plugin implements the Juke Audio volume control integration.
type Plugin struct {
	host     string
	username string
	password string
	client   *http.Client
}

// Config keys for Juke Audio plugin.
const (
	ConfigHost     = "host"     // e.g., "http://192.168.1.100"
	ConfigUsername = "username" // e.g., "Admin"
	ConfigPassword = "password"
)

// New creates a new Juke Audio plugin from configuration.
func New(config map[string]string) (plugins.VolumePlugin, error) {
	host := config[ConfigHost]
	if host == "" {
		return nil, fmt.Errorf("host is required")
	}
	// Ensure host has scheme
	if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
		host = "http://" + host
	}
	// Remove trailing slash
	host = strings.TrimSuffix(host, "/")

	return &Plugin{
		host:     host,
		username: config[ConfigUsername],
		password: config[ConfigPassword],
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}, nil
}

// Type returns the plugin type identifier.
func (p *Plugin) Type() string {
	return PluginType
}

// TestConnection verifies the plugin can connect to the Juke Audio API.
func (p *Plugin) TestConnection() error {
	_, err := p.ListDevices()
	return err
}

// Zone represents a Juke Audio zone.
type Zone struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Volume int    `json:"volume"`
}

// ZonesResponse wraps the zones array if the API returns an object.
type ZonesResponse struct {
	Zones []Zone `json:"zones"`
}

// ListDevices returns all available zones from the Juke Audio system.
func (p *Plugin) ListDevices() ([]plugins.VolumeDevice, error) {
	req, err := http.NewRequest("GET", p.host+"/api/v3/zones", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if p.username != "" {
		req.SetBasicAuth(p.username, p.password)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	// Read body for flexible parsing
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	// Try to parse as direct array first
	var zones []Zone
	if err := json.Unmarshal(body, &zones); err != nil {
		// If that fails, try as wrapped object {"zones": [...]}
		var wrapped ZonesResponse
		if err := json.Unmarshal(body, &wrapped); err != nil {
			return nil, fmt.Errorf("decode response: %w (body: %s)", err, string(body[:min(200, len(body))]))
		}
		zones = wrapped.Zones
	}

	devices := make([]plugins.VolumeDevice, len(zones))
	for i, z := range zones {
		devices[i] = plugins.VolumeDevice{
			ID:   z.ID,
			Name: z.Name,
		}
	}
	return devices, nil
}

// SetVolume sets the volume for a specific zone.
func (p *Plugin) SetVolume(deviceID string, volume int) error {
	if volume < 0 {
		volume = 0
	}
	if volume > 100 {
		volume = 100
	}

	body := fmt.Sprintf(`{"volume": %d}`, volume)
	req, err := http.NewRequest("PUT", p.host+"/api/v3/zones/"+deviceID+"/volume", strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if p.username != "" {
		req.SetBasicAuth(p.username, p.password)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// Register registers the Juke Audio plugin factory with the registry.
func Register(registry *plugins.Registry) {
	registry.RegisterFactory(PluginType, New)
}
