// Package renderer manages multiple DLNA renderer instances.
package renderer

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"

	"github.com/google/uuid"
	"github.com/kenyonj/airbridge/internal/discovery"
	"github.com/kenyonj/airbridge/internal/httpserver"
	"github.com/kenyonj/airbridge/internal/player"
	"github.com/kenyonj/airbridge/internal/ssdp"
	"github.com/kenyonj/airbridge/internal/state"
	"github.com/kenyonj/airbridge/pkg/config"
)

// Instance represents a single DLNA renderer instance.
type Instance struct {
	Device       *discovery.AirPlayDevice
	UUID         string
	FriendlyName string
	Port         int
	State        *state.PlayerState
	Player       *player.RAOPPlayer
	Server       *http.Server
	cancel       context.CancelFunc
}

// Manager manages multiple renderer instances.
type Manager struct {
	cfg       config.Config
	localIP   string
	basePort  int
	instances map[string]*Instance // keyed by device ID
	mu        sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewManager creates a new renderer manager.
func NewManager(cfg config.Config) *Manager {
	return &Manager{
		cfg:       cfg,
		basePort:  cfg.HTTPPort,
		instances: make(map[string]*Instance),
	}
}

// Start initializes the manager.
func (m *Manager) Start(ctx context.Context) error {
	m.ctx, m.cancel = context.WithCancel(ctx)

	// Get local IP
	m.localIP = getLocalIP()
	if m.localIP == "" {
		return fmt.Errorf("could not determine local IP address")
	}

	return nil
}

// Stop shuts down all renderer instances.
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, inst := range m.instances {
		m.stopInstance(inst)
	}

	if m.cancel != nil {
		m.cancel()
	}
}

// AddDevice creates a renderer for an AirPlay device.
func (m *Manager) AddDevice(device *discovery.AirPlayDevice) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if we should include this device
	if !m.cfg.ShouldIncludeDevice(device.Name) {
		log.Printf("Skipping device (filtered): %s", device.Name)
		return nil
	}

	// Check if already exists
	if _, exists := m.instances[device.DeviceID]; exists {
		return nil // Already have this device
	}

	// Determine port
	port := m.basePort + len(m.instances)
	deviceCfg := m.cfg.GetDeviceConfig(device.Name)
	if deviceCfg != nil && deviceCfg.Port > 0 {
		port = deviceCfg.Port
	}

	// Determine friendly name
	friendlyName := fmt.Sprintf("%s (%s)", m.cfg.NamePrefix, device.Name)
	if deviceCfg != nil && deviceCfg.Alias != "" {
		friendlyName = deviceCfg.Alias
	}

	// Create instance
	inst := &Instance{
		Device:       device,
		UUID:         uuid.New().String(),
		FriendlyName: friendlyName,
		Port:         port,
	}

	// Create context
	ctx, cancel := context.WithCancel(m.ctx)
	inst.cancel = cancel

	// Create state
	inst.State = state.New(ctx)

	// Create player
	inst.Player = player.NewRAOPPlayer(device)

	// Setup HTTP server
	baseURL := fmt.Sprintf("http://%s:%d", m.localIP, port)
	mux := http.NewServeMux()
	httpserver.RegisterHTTP(mux, baseURL, inst.UUID, friendlyName, "Airbridge", inst.State, inst.Player)

	inst.Server = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: httpserver.LogMiddleware(mux),
	}

	// Start SSDP
	serverName := "Airbridge/1.0"
	go ssdp.Announce(ctx, baseURL, "uuid:"+inst.UUID, serverName)
	go ssdp.SearchResponder(ctx, baseURL, "uuid:"+inst.UUID, serverName)

	// Start HTTP server
	go func() {
		log.Printf("Starting renderer: %s on port %d", friendlyName, port)
		if err := inst.Server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error for %s: %v", friendlyName, err)
		}
	}()

	m.instances[device.DeviceID] = inst
	log.Printf("Added renderer: %s -> %s:%d (UUID: %s)",
		friendlyName, device.Host, device.Port, inst.UUID)

	return nil
}

// RemoveDevice removes a renderer for a device.
func (m *Manager) RemoveDevice(deviceID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if inst, exists := m.instances[deviceID]; exists {
		m.stopInstance(inst)
		delete(m.instances, deviceID)
		log.Printf("Removed renderer: %s", inst.FriendlyName)
	}
}

// GetInstances returns all active instances.
func (m *Manager) GetInstances() []*Instance {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Instance, 0, len(m.instances))
	for _, inst := range m.instances {
		result = append(result, inst)
	}
	return result
}

// stopInstance shuts down a single instance.
func (m *Manager) stopInstance(inst *Instance) {
	if inst.cancel != nil {
		inst.cancel()
	}
	if inst.Server != nil {
		inst.Server.Shutdown(context.Background())
	}
	if inst.State != nil {
		inst.State.Stop()
	}
}

// UpdateDevices synchronizes renderer instances with discovered devices.
func (m *Manager) UpdateDevices(devices []*discovery.AirPlayDevice) {
	// Add new devices
	for _, device := range devices {
		m.AddDevice(device)
	}

	// Note: We don't remove devices when they disappear, as they may just be
	// temporarily unreachable. A timeout-based cleanup could be added later.
}

// getLocalIP returns the preferred local IP address.
func getLocalIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}
