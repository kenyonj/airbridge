// Package webhook implements a generic webhook volume control plugin.
package webhook

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kenyonj/airbridge/internal/plugins"
)

const PluginType = "webhook"

// Plugin implements a generic webhook for volume control.
// Useful for Home Assistant REST commands or other HTTP APIs.
type Plugin struct {
	url         string
	method      string
	body        string
	headers     map[string]string
	authUser    string
	authPass    string
	client      *http.Client
}

// Config keys for webhook plugin.
const (
	ConfigURL      = "url"       // URL with {{volume}} placeholder
	ConfigMethod   = "method"    // HTTP method (GET, POST, PUT)
	ConfigBody     = "body"      // Request body with {{volume}} placeholder
	ConfigHeaders  = "headers"   // Comma-separated key:value pairs
	ConfigAuthUser = "auth_user" // Basic auth username
	ConfigAuthPass = "auth_pass" // Basic auth password
)

// New creates a new webhook plugin from configuration.
func New(config map[string]string) (plugins.VolumePlugin, error) {
	url := config[ConfigURL]
	if url == "" {
		return nil, fmt.Errorf("url is required")
	}

	method := config[ConfigMethod]
	if method == "" {
		method = "POST"
	}

	// Parse headers from comma-separated key:value pairs
	headers := make(map[string]string)
	if h := config[ConfigHeaders]; h != "" {
		for _, pair := range strings.Split(h, ",") {
			parts := strings.SplitN(strings.TrimSpace(pair), ":", 2)
			if len(parts) == 2 {
				headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			}
		}
	}

	return &Plugin{
		url:      url,
		method:   strings.ToUpper(method),
		body:     config[ConfigBody],
		headers:  headers,
		authUser: config[ConfigAuthUser],
		authPass: config[ConfigAuthPass],
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}, nil
}

// Type returns the plugin type identifier.
func (p *Plugin) Type() string {
	return PluginType
}

// TestConnection attempts a volume set to verify configuration.
// For webhooks, we just validate the URL format.
func (p *Plugin) TestConnection() error {
	if !strings.HasPrefix(p.url, "http://") && !strings.HasPrefix(p.url, "https://") {
		return fmt.Errorf("URL must start with http:// or https://")
	}
	return nil
}

// ListDevices returns empty for webhook plugin (no discovery).
func (p *Plugin) ListDevices() ([]plugins.VolumeDevice, error) {
	return []plugins.VolumeDevice{}, nil
}

// SetVolume sends the webhook with the volume value.
func (p *Plugin) SetVolume(deviceID string, volume int) error {
	if volume < 0 {
		volume = 0
	}
	if volume > 100 {
		volume = 100
	}

	// Replace {{volume}} placeholder in URL and body
	volStr := strconv.Itoa(volume)
	url := strings.ReplaceAll(p.url, "{{volume}}", volStr)
	body := strings.ReplaceAll(p.body, "{{volume}}", volStr)

	// Also support {{volume_float}} for 0.0-1.0 range (Home Assistant)
	volFloat := fmt.Sprintf("%.2f", float64(volume)/100.0)
	url = strings.ReplaceAll(url, "{{volume_float}}", volFloat)
	body = strings.ReplaceAll(body, "{{volume_float}}", volFloat)

	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}

	req, err := http.NewRequest(p.method, url, bodyReader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	// Set headers
	for k, v := range p.headers {
		req.Header.Set(k, v)
	}

	// Default content type for POST/PUT with body
	if body != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	// Basic auth
	if p.authUser != "" {
		req.SetBasicAuth(p.authUser, p.authPass)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// Register registers the webhook plugin factory with the registry.
func Register(registry *plugins.Registry) {
	registry.RegisterFactory(PluginType, New)
}
