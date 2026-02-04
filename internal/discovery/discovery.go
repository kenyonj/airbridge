// Package discovery provides mDNS-based discovery of AirPlay/RAOP and Chromecast devices.
package discovery

import (
	"context"
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

// Service manages discovery of AirPlay and Chromecast devices on the network.
type Service struct {
	devices map[string]*Device // keyed by DeviceID, contains all device types
	mu      sync.RWMutex
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewService creates a new discovery service.
func NewService() *Service {
	return &Service{
		devices: make(map[string]*Device),
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

// GetDevices returns all discovered devices.
func (s *Service) GetDevices() []*Device {
	s.mu.RLock()
	defer s.mu.RUnlock()

	devices := make([]*Device, 0, len(s.devices))
	for _, d := range s.devices {
		devices = append(devices, d)
	}
	return devices
}

// GetDevice returns a device by its ID.
func (s *Service) GetDevice(id string) *Device {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.devices[id]
}

// GetDeviceByName returns a device by its friendly name.
func (s *Service) GetDeviceByName(name string) *Device {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, d := range s.devices {
		if strings.EqualFold(d.Name, name) {
			return d
		}
	}
	return nil
}

// GetDevicesByType returns all devices of a specific type.
func (s *Service) GetDevicesByType(deviceType DeviceType) []*Device {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var devices []*Device
	for _, d := range s.devices {
		if d.DeviceType == deviceType {
			devices = append(devices, d)
		}
	}
	return devices
}

// GetAllDevices is an alias for GetDevices for interface compatibility.
func (s *Service) GetAllDevices() []*Device {
	return s.GetDevices()
}

// GetDeviceUnified is an alias for GetDevice for interface compatibility.
func (s *Service) GetDeviceUnified(id string) *Device {
	return s.GetDevice(id)
}

// parseRAOPEntry parses a zeroconf entry into a Device.
func (s *Service) parseRAOPEntry(entry *zeroconf.ServiceEntry) *Device {
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

	return &Device{
		DeviceID:   deviceID,
		DeviceType: DeviceTypeAirPlay,
		Name:       friendlyName,
		Host:       host,
		Port:       entry.Port,
		Model:      model,
		Features:   features,
		TXTRecord:  txtRecord,
		LastSeen:   time.Now(),
	}
}

// cleanMDNSName removes backslash escapes from mDNS names.
func cleanMDNSName(name string) string {
	// mDNS escapes spaces and special chars with backslashes
	result := strings.ReplaceAll(name, "\\ ", " ")
	result = strings.ReplaceAll(result, "\\", "")
	return result
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
					s.devices[device.DeviceID] = device
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

// parseChromecastEntry parses a zeroconf entry into a Device.
func (s *Service) parseChromecastEntry(entry *zeroconf.ServiceEntry) *Device {
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

	return &Device{
		DeviceID:   deviceID,
		DeviceType: DeviceTypeChromecast,
		Name:       friendlyName,
		Host:       host,
		Port:       entry.Port,
		Model:      model,
		Features:   0,
		TXTRecord:  txtRecord,
		LastSeen:   time.Now(),
	}
}
