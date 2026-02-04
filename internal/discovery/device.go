// Package discovery provides mDNS-based discovery of AirPlay/RAOP and Chromecast devices.
package discovery

import (
	"fmt"
	"strings"
	"time"
)

// Device represents a discovered network audio device.
type Device struct {
	DeviceID   string            // Unique device identifier
	DeviceType DeviceType        // "airplay" or "chromecast"
	Name       string            // Friendly name (e.g., "Kitchen")
	Host       string            // Hostname or IP
	Port       int               // Service port
	Model      string            // Device model (e.g., "HomePod", "Chromecast Audio")
	Features   uint64            // Feature flags (AirPlay only)
	TXTRecord  map[string]string // Raw TXT record fields
	LastSeen   time.Time         // When the device was last discovered
}

// String returns a human-readable representation of the device.
func (d *Device) String() string {
	return fmt.Sprintf("%s (%s) at %s:%d [%s, %s]", d.Name, d.DeviceID, d.Host, d.Port, d.Model, d.DeviceType)
}

// IsAirPlay returns true if this is an AirPlay device.
func (d *Device) IsAirPlay() bool {
	return d.DeviceType == DeviceTypeAirPlay
}

// IsChromecast returns true if this is a Chromecast device.
func (d *Device) IsChromecast() bool {
	return d.DeviceType == DeviceTypeChromecast
}

// SupportsALAC returns true if the device supports Apple Lossless Audio Codec.
// Only applicable to AirPlay devices.
func (d *Device) SupportsALAC() bool {
	if d.DeviceType != DeviceTypeAirPlay {
		return false
	}
	if cn, ok := d.TXTRecord["cn"]; ok {
		return strings.Contains(cn, "1")
	}
	return false
}

// EncryptionTypes returns the encryption types supported by the device.
// Only applicable to AirPlay devices.
func (d *Device) EncryptionTypes() string {
	if d.DeviceType != DeviceTypeAirPlay {
		return "0"
	}
	if et, ok := d.TXTRecord["et"]; ok {
		return et
	}
	return "0"
}
