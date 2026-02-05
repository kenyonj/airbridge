// Package juke implements the Juke Audio volume control plugin.
package juke

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/kenyonj/airbridge/internal/plugins"
)

const PluginType = "juke_audio"

// Plugin implements the Juke Audio volume control integration.
type Plugin struct {
	host        string
	username    string
	password    string
	client      *http.Client
	webhookURL  string
	webhookLock sync.Mutex
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

// ZoneInfo represents a Juke Audio zone from /api/v3/zones/info endpoint.
// See API docs: https://sim.jukeaudio.com/api/v3/apidocs/
type ZoneInfo struct {
	ZoneID  string `json:"zone_id"`
	Name    string `json:"name"`
	Volume  int    `json:"volume"`
	Enabled bool   `json:"enabled"`
	Muted   bool   `json:"muted"`
}

// ListDevices returns all available zones from the Juke Audio system.
// Uses /api/v3/zones/info to get all zone details in one call.
func (p *Plugin) ListDevices() ([]plugins.VolumeDevice, error) {
	log.Printf("[Juke] ListDevices: host=%s, username=%s", p.host, p.username)

	req, err := http.NewRequest("GET", p.host+"/api/v3/zones/info", nil)
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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	log.Printf("[Juke] API response (%d bytes): %s", len(body), string(body[:min(500, len(body))]))

	// /api/v3/zones/info returns an array of ZoneInfo objects
	var zones []ZoneInfo
	if err := json.Unmarshal(body, &zones); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	log.Printf("[Juke] Found %d zones", len(zones))

	devices := make([]plugins.VolumeDevice, 0, len(zones))
	for _, z := range zones {
		name := z.Name
		if name == "" {
			name = z.ZoneID
		}
		devices = append(devices, plugins.VolumeDevice{
			ID:   z.ZoneID,
			Name: name,
		})
		log.Printf("[Juke] Zone: id=%s, name=%s", z.ZoneID, name)
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

// SubscribeWebhook registers a webhook URL to receive zone change notifications.
// Implements plugins.WebhookPlugin interface.
func (p *Plugin) SubscribeWebhook(callbackURL string) error {
	p.webhookLock.Lock()
	defer p.webhookLock.Unlock()

	log.Printf("[Juke] Subscribing webhook: %s", callbackURL)

	body := fmt.Sprintf(`{"url": "%s"}`, callbackURL)
	req, err := http.NewRequest("POST", p.host+"/api/v3/zones/subscribe", strings.NewReader(body))
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

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	p.webhookURL = callbackURL
	log.Printf("[Juke] Webhook subscribed successfully")
	return nil
}

// UnsubscribeWebhook removes the webhook subscription.
// Implements plugins.WebhookPlugin interface.
func (p *Plugin) UnsubscribeWebhook(callbackURL string) error {
	p.webhookLock.Lock()
	defer p.webhookLock.Unlock()

	log.Printf("[Juke] Unsubscribing webhook: %s", callbackURL)

	body := fmt.Sprintf(`{"url": "%s"}`, callbackURL)
	req, err := http.NewRequest("POST", p.host+"/api/v3/zones/unsubscribe", strings.NewReader(body))
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

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	p.webhookURL = ""
	log.Printf("[Juke] Webhook unsubscribed successfully")
	return nil
}

// ParseWebhookPayload parses a Juke webhook payload and returns zone updates.
// The Juke API posts an array of ZoneInfo objects when zones change.
// Note: The payload may be double-encoded (a JSON string containing JSON).
func ParseWebhookPayload(body []byte) ([]ZoneInfo, error) {
	var zones []ZoneInfo

	// Try direct parsing first
	if err := json.Unmarshal(body, &zones); err == nil {
		return zones, nil
	}

	// Try parsing as double-encoded JSON (string containing JSON)
	var jsonStr string
	if err := json.Unmarshal(body, &jsonStr); err != nil {
		return nil, fmt.Errorf("parse webhook payload: not valid JSON or JSON string")
	}

	if err := json.Unmarshal([]byte(jsonStr), &zones); err != nil {
		return nil, fmt.Errorf("parse webhook payload inner JSON: %w", err)
	}

	return zones, nil
}

// Register registers the Juke Audio plugin factory with the registry.
func Register(registry *plugins.Registry) {
	registry.RegisterFactory(PluginType, New)
}

// Ensure Plugin implements WebhookPlugin interface
var _ plugins.WebhookPlugin = (*Plugin)(nil)
