// Package plugins defines the interface for volume control plugins.
package plugins

// VolumeDevice represents a device/zone that can have its volume controlled.
type VolumeDevice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// VolumePlugin is the interface that all volume control plugins must implement.
type VolumePlugin interface {
	// Type returns the plugin type identifier (e.g., "juke_audio", "webhook").
	Type() string

	// TestConnection verifies the plugin configuration is valid and can connect.
	TestConnection() error

	// ListDevices returns available devices/zones for this plugin.
	// Returns empty slice for plugins that don't support discovery (e.g., webhook).
	ListDevices() ([]VolumeDevice, error)

	// SetVolume sets the volume for the specified device (0-100).
	// For webhook plugins, deviceID may be empty.
	SetVolume(deviceID string, volume int) error
}

// WebhookPlugin is an optional interface for plugins that support webhook subscriptions
// for receiving real-time updates from external systems.
type WebhookPlugin interface {
	VolumePlugin

	// SubscribeWebhook registers a webhook URL to receive volume change notifications.
	// The callbackURL is where the external system should POST updates.
	SubscribeWebhook(callbackURL string) error

	// UnsubscribeWebhook removes the webhook subscription.
	UnsubscribeWebhook(callbackURL string) error
}

// VolumeUpdateCallback is called when volume is updated from an external source.
type VolumeUpdateCallback func(pluginID, deviceID string, volume int, muted bool)

// PluginConfig holds the stored configuration for a plugin instance.
type PluginConfig struct {
	ID      string            `json:"id"`
	Type    string            `json:"type"`
	Name    string            `json:"name"`
	Config  map[string]string `json:"config"`
	Enabled bool              `json:"enabled"`
}

// PluginFactory creates a new plugin instance from configuration.
type PluginFactory func(config map[string]string) (VolumePlugin, error)
