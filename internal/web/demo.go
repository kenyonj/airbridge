// Package web provides the admin web UI for Airbridge.
package web

import (
	"time"

	"github.com/kenyonj/airbridge/internal/database"
	"github.com/kenyonj/airbridge/internal/discovery"
)

// DemoController provides mock data for UI development.
type DemoController struct {
	startTime time.Time
}

// NewDemoController creates a new demo controller with mock data.
func NewDemoController() *DemoController {
	return &DemoController{
		startTime: time.Now(),
	}
}

// IsRunning returns mock running status.
func (d *DemoController) IsRunning(id string) bool {
	return true
}

// RunningCount returns mock running count.
func (d *DemoController) RunningCount() int {
	return 3
}

// Uptime returns time since demo started.
func (d *DemoController) Uptime() time.Duration {
	return time.Since(d.startTime)
}

// LocalIP returns a mock IP.
func (d *DemoController) LocalIP() string {
	return "192.168.1.100"
}

// StartRenderer is a no-op in demo mode.
func (d *DemoController) StartRenderer(id string) error {
	return nil
}

// StopRenderer is a no-op in demo mode.
func (d *DemoController) StopRenderer(id string) {}

// StopAll is a no-op in demo mode.
func (d *DemoController) StopAll() {}

// RestartAll is a no-op in demo mode.
func (d *DemoController) RestartAll() {}

// GetTransportState returns mock transport state.
func (d *DemoController) GetTransportState(id string) string {
	// Rotate states for visual variety
	states := []string{"PLAYING", "STOPPED", "STOPPED"}
	// Use ID to deterministically pick a state
	idx := 0
	for _, c := range id {
		idx += int(c)
	}
	return states[idx%len(states)]
}

// DemoDiscovery provides mock device discovery.
type DemoDiscovery struct{}

// NewDemoDiscovery creates a new demo discovery service.
func NewDemoDiscovery() *DemoDiscovery {
	return &DemoDiscovery{}
}

// GetDevices returns mock AirPlay devices.
func (d *DemoDiscovery) GetDevices() []*discovery.AirPlayDevice {
	return []*discovery.AirPlayDevice{
		{DeviceID: "airplay-1", Name: "Living Room HomePod", Host: "192.168.1.50", Port: 7000, Model: "AudioAccessory1,1"},
		{DeviceID: "airplay-2", Name: "Bedroom HomePod Mini", Host: "192.168.1.51", Port: 7000, Model: "AudioAccessory5,1"},
		{DeviceID: "airplay-3", Name: "Office Speaker", Host: "192.168.1.52", Port: 7000, Model: "AirPort Express"},
		{DeviceID: "airplay-4", Name: "Kitchen HomePod", Host: "192.168.1.53", Port: 7000, Model: "AudioAccessory1,1"},
		{DeviceID: "airplay-5", Name: "Patio Speaker", Host: "192.168.1.54", Port: 7000, Model: "AudioAccessory5,1"},
	}
}

// GetChromecasts returns mock Chromecast devices.
func (d *DemoDiscovery) GetChromecasts() []*discovery.ChromecastDevice {
	return []*discovery.ChromecastDevice{
		{DeviceID: "chromecast-1", Name: "Living Room TV", Host: "192.168.1.60", Port: 8009, Model: "Chromecast Ultra"},
		{DeviceID: "chromecast-2", Name: "Bedroom Display", Host: "192.168.1.61", Port: 8009, Model: "Nest Hub"},
		{DeviceID: "chromecast-3", Name: "Kitchen Display", Host: "192.168.1.62", Port: 8009, Model: "Nest Hub Max"},
	}
}

// GetDevice returns a mock AirPlay device by ID.
func (d *DemoDiscovery) GetDevice(deviceID string) *discovery.AirPlayDevice {
	for _, dev := range d.GetDevices() {
		if dev.DeviceID == deviceID {
			return dev
		}
	}
	return nil
}

// GetChromecast returns a mock Chromecast device by ID.
func (d *DemoDiscovery) GetChromecast(deviceID string) *discovery.ChromecastDevice {
	for _, dev := range d.GetChromecasts() {
		if dev.DeviceID == deviceID {
			return dev
		}
	}
	return nil
}

// GetAllDevices returns all mock devices as unified Device types.
func (d *DemoDiscovery) GetAllDevices() []*discovery.Device {
	var devices []*discovery.Device
	for _, dev := range d.GetDevices() {
		devices = append(devices, dev.ToDevice())
	}
	for _, dev := range d.GetChromecasts() {
		devices = append(devices, dev.ToDevice())
	}
	return devices
}

// GetDeviceUnified returns a mock device by ID.
func (d *DemoDiscovery) GetDeviceUnified(deviceID string) *discovery.Device {
	if dev := d.GetDevice(deviceID); dev != nil {
		return dev.ToDevice()
	}
	if dev := d.GetChromecast(deviceID); dev != nil {
		return dev.ToDevice()
	}
	return nil
}

// DemoDB provides mock database operations.
type DemoDB struct {
	renderers []database.Renderer
	nextPort  int
}

// NewDemoDB creates a new demo database with mock renderers.
func NewDemoDB() *DemoDB {
	return &DemoDB{
		renderers: []database.Renderer{
			{
				ID:              "demo-renderer-1",
				Name:            "Living Room Speaker",
				AirPlayDeviceID: "airplay-1",
				AirPlayName:     "Living Room HomePod",
				DeviceID:        "airplay-1",
				DeviceType:      "airplay",
				Port:            8201,
				Enabled:         true,
			},
			{
				ID:              "demo-renderer-2",
				Name:            "Kitchen Audio",
				AirPlayDeviceID: "chromecast-3",
				AirPlayName:     "Kitchen Display",
				DeviceID:        "chromecast-3",
				DeviceType:      "chromecast",
				Port:            8202,
				Enabled:         true,
			},
			{
				ID:              "demo-renderer-3",
				Name:            "Office Speaker",
				AirPlayDeviceID: "airplay-3",
				AirPlayName:     "Office Speaker",
				DeviceID:        "airplay-3",
				DeviceType:      "airplay",
				Port:            8203,
				Enabled:         true,
			},
		},
		nextPort: 8204,
	}
}

// ListRenderers returns mock renderers.
func (d *DemoDB) ListRenderers() ([]database.Renderer, error) {
	return d.renderers, nil
}

// GetRenderer returns a mock renderer by ID.
func (d *DemoDB) GetRenderer(id string) (*database.Renderer, error) {
	for _, r := range d.renderers {
		if r.ID == id {
			return &r, nil
		}
	}
	return nil, nil
}

// CreateRenderer adds a mock renderer.
func (d *DemoDB) CreateRenderer(r *database.Renderer) error {
	r.Port = d.nextPort
	d.nextPort++
	d.renderers = append(d.renderers, *r)
	return nil
}

// UpdateRenderer updates a mock renderer.
func (d *DemoDB) UpdateRenderer(r *database.Renderer) error {
	for i, existing := range d.renderers {
		if existing.ID == r.ID {
			d.renderers[i] = *r
			return nil
		}
	}
	return nil
}

// DeleteRenderer removes a mock renderer.
func (d *DemoDB) DeleteRenderer(id string) error {
	for i, r := range d.renderers {
		if r.ID == id {
			d.renderers = append(d.renderers[:i], d.renderers[i+1:]...)
			return nil
		}
	}
	return nil
}

// GetNextPort returns the next available port starting from basePort.
func (d *DemoDB) GetNextPort(basePort int) (int, error) {
	if d.nextPort < basePort {
		d.nextPort = basePort
	}
	port := d.nextPort
	d.nextPort++
	return port, nil
}

// ToggleRenderer toggles the enabled state of a renderer.
func (d *DemoDB) ToggleRenderer(id string) error {
	for i, r := range d.renderers {
		if r.ID == id {
			d.renderers[i].Enabled = !d.renderers[i].Enabled
			return nil
		}
	}
	return nil
}

// RenameRenderer changes the name of a renderer.
func (d *DemoDB) RenameRenderer(id, name string) error {
	for i, r := range d.renderers {
		if r.ID == id {
			d.renderers[i].Name = name
			return nil
		}
	}
	return nil
}

// Close is a no-op for demo DB.
func (d *DemoDB) Close() error {
	return nil
}
