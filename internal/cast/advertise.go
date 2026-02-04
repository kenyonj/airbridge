// Package cast provides Chromecast receiver functionality.
package cast

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"sync"

	"github.com/grandcat/zeroconf"
)

// Advertiser manages mDNS advertisement for virtual Chromecast devices.
type Advertiser struct {
	mu       sync.RWMutex
	services map[string]*zeroconf.Server // keyed by renderer UUID
}

// NewAdvertiser creates a new Chromecast advertiser.
func NewAdvertiser() *Advertiser {
	return &Advertiser{
		services: make(map[string]*zeroconf.Server),
	}
}

// DeviceConfig contains the configuration for a virtual Chromecast device.
type DeviceConfig struct {
	UUID         string // Renderer UUID (used to generate device ID)
	FriendlyName string // Display name shown in cast dialogs
	Model        string // Device model (e.g., "Airbridge")
	Port         int    // Port for CASTV2 protocol (typically 8009)
}

// generateDeviceID creates a deterministic Chromecast device ID from the UUID.
func generateDeviceID(uuid string) string {
	hash := sha256.Sum256([]byte("airbridge-cast:" + uuid))
	return fmt.Sprintf("%x", hash[:16]) // 32 hex chars
}

// Advertise starts advertising a virtual Chromecast device.
func (a *Advertiser) Advertise(cfg DeviceConfig) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Stop any existing advertisement for this UUID
	if server, exists := a.services[cfg.UUID]; exists {
		server.Shutdown()
		delete(a.services, cfg.UUID)
	}

	deviceID := generateDeviceID(cfg.UUID)

	// TXT records required for Chromecast discovery
	txtRecords := []string{
		fmt.Sprintf("id=%s", deviceID),
		fmt.Sprintf("fn=%s", cfg.FriendlyName),
		fmt.Sprintf("md=%s", cfg.Model),
		"ve=02",           // Cast protocol version
		"ca=4101",         // Capabilities (audio support)
		"st=0",            // Status: idle
		"ic=/icon",        // Icon path (placeholder)
		"rs=",             // Receiver status
		"nf=1",            // Network function
		"bs=FA8FCA7EEA8F", // Placeholder
	}

	// Register the mDNS service
	server, err := zeroconf.Register(
		cfg.FriendlyName,   // Instance name
		"_googlecast._tcp", // Service type
		"local.",           // Domain
		cfg.Port,           // Port
		txtRecords,         // TXT records
		nil,                // Network interfaces (nil = all)
	)
	if err != nil {
		return fmt.Errorf("failed to register Chromecast service: %w", err)
	}

	a.services[cfg.UUID] = server
	log.Printf("Advertising Chromecast device: %s (id=%s) on port %d", cfg.FriendlyName, deviceID, cfg.Port)
	return nil
}

// Stop stops advertising a specific device.
func (a *Advertiser) Stop(uuid string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if server, exists := a.services[uuid]; exists {
		server.Shutdown()
		delete(a.services, uuid)
		log.Printf("Stopped advertising Chromecast device: %s", uuid)
	}
}

// StopAll stops advertising all devices.
func (a *Advertiser) StopAll() {
	a.mu.Lock()
	defer a.mu.Unlock()

	for uuid, server := range a.services {
		server.Shutdown()
		delete(a.services, uuid)
	}
	log.Println("Stopped all Chromecast advertisements")
}

// IsAdvertising returns true if the device is being advertised.
func (a *Advertiser) IsAdvertising(uuid string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	_, exists := a.services[uuid]
	return exists
}

// AdvertisedCount returns the number of devices being advertised.
func (a *Advertiser) AdvertisedCount() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.services)
}

// Run starts the advertiser and blocks until context is cancelled.
func (a *Advertiser) Run(ctx context.Context) {
	<-ctx.Done()
	a.StopAll()
}
