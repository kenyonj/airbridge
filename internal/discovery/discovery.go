// Package discovery provides mDNS-based discovery of AirPlay/RAOP and Chromecast devices.
package discovery

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/grandcat/zeroconf"
)

// DeviceType identifies the type of discovered device.
type DeviceType string

const (
	DeviceTypeAirPlay    DeviceType = "airplay"
	DeviceTypeChromecast DeviceType = "chromecast"
)

// AirPlayDevice represents a discovered AirPlay device.
type AirPlayDevice struct {
	Name      string            // Friendly name (e.g., "Kitchen")
	DeviceID  string            // MAC address-based ID (e.g., "C6F4891B35E7")
	Host      string            // Hostname or IP
	Port      int               // RAOP port
	Model     string            // Device model (e.g., "Shairport Sync")
	Features  uint64            // Feature flags
	TXTRecord map[string]string // Raw TXT record fields
	LastSeen  time.Time         // When the device was last discovered
}

// Service manages discovery of AirPlay devices on the network.
type Service struct {
	devices     map[string]*AirPlayDevice    // keyed by DeviceID
	chromecasts map[string]*ChromecastDevice // keyed by DeviceID
	mu          sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewService creates a new discovery service.
func NewService() *Service {
	return &Service{
		devices:     make(map[string]*AirPlayDevice),
		chromecasts: make(map[string]*ChromecastDevice),
	}
}

// Start begins discovering AirPlay devices.
func (s *Service) Start(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)

	go s.browseLoop()
	go s.browseChromecastLoop()
	return nil
}

func (s *Service) browseLoop() {
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		// Create a fresh resolver for each browse cycle
		resolver, err := zeroconf.NewResolver(nil)
		if err != nil {
			log.Printf("Failed to create resolver: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		entries := make(chan *zeroconf.ServiceEntry)

		// Process entries in a goroutine
		done := make(chan struct{})
		go func() {
			for entry := range entries {
				device := s.parseRAOPEntry(entry)
				if device != nil {
					s.mu.Lock()
					s.devices[device.DeviceID] = device
					s.mu.Unlock()
					log.Printf("Discovered: %s", device)
				}
			}
			close(done)
		}()

		// Browse for 10 seconds
		browseCtx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
		err = resolver.Browse(browseCtx, "_raop._tcp", "local.", entries)
		if err != nil {
			log.Printf("Browse error: %v", err)
		}

		// Wait for context to finish
		<-browseCtx.Done()
		cancel()

		// Wait for entry processing to complete
		<-done

		// Sleep before next cycle
		select {
		case <-s.ctx.Done():
			return
		case <-time.After(20 * time.Second):
		}
	}
}

// Stop stops the discovery service.
func (s *Service) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
}

// GetDevices returns a copy of all discovered devices.
func (s *Service) GetDevices() []*AirPlayDevice {
	s.mu.RLock()
	defer s.mu.RUnlock()

	devices := make([]*AirPlayDevice, 0, len(s.devices))
	for _, d := range s.devices {
		devices = append(devices, d)
	}
	return devices
}

// GetDevice returns a device by its ID.
func (s *Service) GetDevice(id string) *AirPlayDevice {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.devices[id]
}

// GetDeviceByName returns a device by its friendly name.
func (s *Service) GetDeviceByName(name string) *AirPlayDevice {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, d := range s.devices {
		if strings.EqualFold(d.Name, name) {
			return d
		}
	}
	return nil
}

// GetChromecasts returns a copy of all discovered Chromecast devices.
func (s *Service) GetChromecasts() []*ChromecastDevice {
	s.mu.RLock()
	defer s.mu.RUnlock()

	devices := make([]*ChromecastDevice, 0, len(s.chromecasts))
	for _, d := range s.chromecasts {
		devices = append(devices, d)
	}
	return devices
}

// GetChromecast returns a Chromecast device by its ID.
func (s *Service) GetChromecast(id string) *ChromecastDevice {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.chromecasts[id]
}

// GetChromecastByName returns a Chromecast device by its friendly name.
func (s *Service) GetChromecastByName(name string) *ChromecastDevice {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, d := range s.chromecasts {
		if strings.EqualFold(d.Name, name) {
			return d
		}
	}
	return nil
}

// GetAllDevices returns all discovered devices (AirPlay and Chromecast) as unified Device types.
func (s *Service) GetAllDevices() []*Device {
	s.mu.RLock()
	defer s.mu.RUnlock()

	devices := make([]*Device, 0, len(s.devices)+len(s.chromecasts))
	for _, d := range s.devices {
		devices = append(devices, d.ToDevice())
	}
	for _, d := range s.chromecasts {
		devices = append(devices, d.ToDevice())
	}
	return devices
}

// GetDeviceUnified returns a device by its ID, checking both AirPlay and Chromecast.
func (s *Service) GetDeviceUnified(id string) *Device {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if d, ok := s.devices[id]; ok {
		return d.ToDevice()
	}
	if d, ok := s.chromecasts[id]; ok {
		return d.ToDevice()
	}
	return nil
}

// GetDeviceByNameUnified returns a device by its friendly name, checking both types.
func (s *Service) GetDeviceByNameUnified(name string) *Device {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, d := range s.devices {
		if strings.EqualFold(d.Name, name) {
			return d.ToDevice()
		}
	}
	for _, d := range s.chromecasts {
		if strings.EqualFold(d.Name, name) {
			return d.ToDevice()
		}
	}
	return nil
}

// parseRAOPEntry parses a zeroconf entry into an AirPlayDevice.
func (s *Service) parseRAOPEntry(entry *zeroconf.ServiceEntry) *AirPlayDevice {
	// RAOP instance names are formatted as "DEVICEID@FriendlyName"
	// e.g., "C6F4891B35E7@Dining Room"
	parts := strings.SplitN(entry.Instance, "@", 2)
	if len(parts) != 2 {
		return nil
	}

	deviceID := parts[0]
	// Clean up backslash escapes from mDNS names
	friendlyName := cleanMDNSName(parts[1])

	// Parse TXT record
	txtRecord := make(map[string]string)
	for _, txt := range entry.Text {
		if idx := strings.Index(txt, "="); idx > 0 {
			txtRecord[txt[:idx]] = txt[idx+1:]
		}
	}

	// Get model
	model := txtRecord["am"]
	if model == "" {
		model = "Unknown"
	}

	// Parse features
	var features uint64
	if ft := txtRecord["ft"]; ft != "" {
		// Features may be comma-separated hex values
		ftParts := strings.Split(ft, ",")
		for i, part := range ftParts {
			val, err := strconv.ParseUint(strings.TrimPrefix(part, "0x"), 16, 64)
			if err == nil {
				if i == 0 {
					features = val
				} else {
					features |= val << 32
				}
			}
		}
	}

	// Get host
	host := entry.HostName
	if len(entry.AddrIPv4) > 0 {
		host = entry.AddrIPv4[0].String()
	} else if len(entry.AddrIPv6) > 0 {
		host = entry.AddrIPv6[0].String()
	}

	return &AirPlayDevice{
		Name:      friendlyName,
		DeviceID:  deviceID,
		Host:      host,
		Port:      entry.Port,
		Model:     model,
		Features:  features,
		TXTRecord: txtRecord,
		LastSeen:  time.Now(),
	}
}

// String returns a human-readable representation of the device.
func (d *AirPlayDevice) String() string {
	return fmt.Sprintf("%s (%s) at %s:%d [%s]", d.Name, d.DeviceID, d.Host, d.Port, d.Model)
}

// cleanMDNSName removes backslash escapes from mDNS names.
func cleanMDNSName(name string) string {
	// mDNS escapes spaces and special chars with backslashes
	result := strings.ReplaceAll(name, "\\ ", " ")
	result = strings.ReplaceAll(result, "\\", "")
	return result
}

// EncryptionTypes returns the encryption types supported by the device.
func (d *AirPlayDevice) EncryptionTypes() string {
	if et, ok := d.TXTRecord["et"]; ok {
		return et
	}
	return "0"
}

// SupportsALAC returns true if the device supports ALAC audio.
func (d *AirPlayDevice) SupportsALAC() bool {
	if cn, ok := d.TXTRecord["cn"]; ok {
		return strings.Contains(cn, "1")
	}
	return false
}

// ChromecastDevice represents a discovered Chromecast device.
type ChromecastDevice struct {
	Name      string            // Friendly name (e.g., "Living Room TV")
	DeviceID  string            // Unique device ID
	Host      string            // Hostname or IP
	Port      int               // Cast port (usually 8009)
	Model     string            // Device model (e.g., "Chromecast")
	TXTRecord map[string]string // Raw TXT record fields
	LastSeen  time.Time         // When the device was last discovered
}

// String returns a human-readable representation of the Chromecast device.
func (d *ChromecastDevice) String() string {
	return fmt.Sprintf("%s (%s) at %s:%d [%s]", d.Name, d.DeviceID, d.Host, d.Port, d.Model)
}

// browseChromecastLoop continuously discovers Chromecast devices.
func (s *Service) browseChromecastLoop() {
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		resolver, err := zeroconf.NewResolver(nil)
		if err != nil {
			log.Printf("Failed to create Chromecast resolver: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		entries := make(chan *zeroconf.ServiceEntry)

		done := make(chan struct{})
		go func() {
			for entry := range entries {
				device := s.parseChromecastEntry(entry)
				if device != nil {
					s.mu.Lock()
					s.chromecasts[device.DeviceID] = device
					s.mu.Unlock()
					log.Printf("Discovered Chromecast: %s", device)
				}
			}
			close(done)
		}()

		browseCtx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
		err = resolver.Browse(browseCtx, "_googlecast._tcp", "local.", entries)
		if err != nil {
			log.Printf("Chromecast browse error: %v", err)
		}

		<-browseCtx.Done()
		cancel()
		<-done

		select {
		case <-s.ctx.Done():
			return
		case <-time.After(20 * time.Second):
		}
	}
}

// parseChromecastEntry parses a zeroconf entry into a ChromecastDevice.
func (s *Service) parseChromecastEntry(entry *zeroconf.ServiceEntry) *ChromecastDevice {
	// Parse TXT record
	txtRecord := make(map[string]string)
	for _, txt := range entry.Text {
		if idx := strings.Index(txt, "="); idx > 0 {
			txtRecord[txt[:idx]] = txt[idx+1:]
		}
	}

	// Get device ID from TXT record
	deviceID := txtRecord["id"]
	if deviceID == "" {
		// Fall back to instance name if no ID
		deviceID = entry.Instance
	}

	// Get friendly name from TXT record (fn = friendly name)
	friendlyName := txtRecord["fn"]
	if friendlyName == "" {
		friendlyName = entry.Instance
	}
	friendlyName = cleanMDNSName(friendlyName)

	// Get model from TXT record (md = model description)
	model := txtRecord["md"]
	if model == "" {
		model = "Chromecast"
	}

	// Get host
	host := entry.HostName
	if len(entry.AddrIPv4) > 0 {
		host = entry.AddrIPv4[0].String()
	} else if len(entry.AddrIPv6) > 0 {
		host = entry.AddrIPv6[0].String()
	}

	return &ChromecastDevice{
		Name:      friendlyName,
		DeviceID:  deviceID,
		Host:      host,
		Port:      entry.Port,
		Model:     model,
		TXTRecord: txtRecord,
		LastSeen:  time.Now(),
	}
}
