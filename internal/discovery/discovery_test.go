package discovery

import (
	"context"
	"testing"
	"time"

	"github.com/grandcat/zeroconf"
)

func TestNewService(t *testing.T) {
	s := NewService()
	if s == nil {
		t.Fatal("expected non-nil service")
	}
	if s.devices == nil {
		t.Error("expected initialized devices map")
	}
}

func TestService_GetDevices_Empty(t *testing.T) {
	s := NewService()
	devices := s.GetDevices()
	if len(devices) != 0 {
		t.Errorf("expected empty list, got %d devices", len(devices))
	}
}

func TestService_GetDevice_NotFound(t *testing.T) {
	s := NewService()
	device := s.GetDevice("nonexistent")
	if device != nil {
		t.Error("expected nil for nonexistent device")
	}
}

func TestService_GetDeviceByName_NotFound(t *testing.T) {
	s := NewService()
	device := s.GetDeviceByName("Kitchen")
	if device != nil {
		t.Error("expected nil for nonexistent device")
	}
}

func TestService_ManualAddAndGet(t *testing.T) {
	s := NewService()

	// Manually add a device (simulating what browseLoop does)
	device := &AirPlayDevice{
		Name:     "Living Room",
		DeviceID: "C6F4891B35E7",
		Host:     "192.168.1.100",
		Port:     7000,
		Model:    "Shairport Sync",
		LastSeen: time.Now(),
	}

	s.mu.Lock()
	s.devices[device.DeviceID] = device
	s.mu.Unlock()

	// Test GetDevices
	devices := s.GetDevices()
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}
	if devices[0].Name != "Living Room" {
		t.Errorf("expected 'Living Room', got %q", devices[0].Name)
	}

	// Test GetDevice by ID
	got := s.GetDevice("C6F4891B35E7")
	if got == nil {
		t.Fatal("expected device, got nil")
	}
	if got.Host != "192.168.1.100" {
		t.Errorf("expected host 192.168.1.100, got %q", got.Host)
	}

	// Test GetDeviceByName
	got = s.GetDeviceByName("Living Room")
	if got == nil {
		t.Fatal("expected device, got nil")
	}
	if got.DeviceID != "C6F4891B35E7" {
		t.Errorf("expected ID C6F4891B35E7, got %q", got.DeviceID)
	}

	// Test case insensitive name lookup
	got = s.GetDeviceByName("living room")
	if got == nil {
		t.Error("expected case insensitive match")
	}
}

func TestParseRAOPEntry(t *testing.T) {
	s := NewService()

	// Helper to create ServiceEntry with instance name
	makeEntry := func(instance string, port int, hostName string, text []string) *zeroconf.ServiceEntry {
		entry := zeroconf.NewServiceEntry(instance, "_raop._tcp", "local")
		entry.Port = port
		entry.HostName = hostName
		entry.Text = text
		return entry
	}

	tests := []struct {
		name       string
		entry      *zeroconf.ServiceEntry
		wantNil    bool
		wantName   string
		wantID     string
		wantModel  string
		wantPort   int
	}{
		{
			name:      "valid entry",
			entry:     makeEntry("C6F4891B35E7@Living Room", 7000, "living-room.local", []string{"am=Shairport Sync", "ft=0x1"}),
			wantNil:   false,
			wantName:  "Living Room",
			wantID:    "C6F4891B35E7",
			wantModel: "Shairport Sync",
			wantPort:  7000,
		},
		{
			name:     "escaped spaces in name",
			entry:    makeEntry("AABBCCDD1234@Dining\\ Room", 7000, "dining.local", []string{"am=HomePod"}),
			wantNil:  false,
			wantName: "Dining Room",
			wantID:   "AABBCCDD1234",
		},
		{
			name:    "missing @ separator",
			entry:   makeEntry("InvalidInstanceName", 7000, "", nil),
			wantNil: true,
		},
		{
			name:      "no model in TXT",
			entry:     makeEntry("1234567890AB@Kitchen", 7000, "kitchen.local", []string{}),
			wantNil:   false,
			wantName:  "Kitchen",
			wantID:    "1234567890AB",
			wantModel: "Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.parseRAOPEntry(tt.entry)
			if tt.wantNil {
				if got != nil {
					t.Errorf("expected nil, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected device, got nil")
			}
			if got.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", got.Name, tt.wantName)
			}
			if got.DeviceID != tt.wantID {
				t.Errorf("DeviceID = %q, want %q", got.DeviceID, tt.wantID)
			}
			if tt.wantModel != "" && got.Model != tt.wantModel {
				t.Errorf("Model = %q, want %q", got.Model, tt.wantModel)
			}
			if tt.wantPort != 0 && got.Port != tt.wantPort {
				t.Errorf("Port = %d, want %d", got.Port, tt.wantPort)
			}
		})
	}
}

func TestCleanMDNSName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Living\\ Room", "Living Room"},
		{"Kitchen", "Kitchen"},
		{"Dining\\ Room\\ Speaker", "Dining Room Speaker"},
		{"Test\\Name", "TestName"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := cleanMDNSName(tt.input)
			if got != tt.want {
				t.Errorf("cleanMDNSName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestAirPlayDevice_String(t *testing.T) {
	d := &AirPlayDevice{
		Name:     "Living Room",
		DeviceID: "C6F4891B35E7",
		Host:     "192.168.1.100",
		Port:     7000,
		Model:    "Shairport Sync",
	}

	s := d.String()
	expected := "Living Room (C6F4891B35E7) at 192.168.1.100:7000 [Shairport Sync]"
	if s != expected {
		t.Errorf("String() = %q, want %q", s, expected)
	}
}

func TestAirPlayDevice_EncryptionTypes(t *testing.T) {
	tests := []struct {
		name      string
		txtRecord map[string]string
		want      string
	}{
		{"with et", map[string]string{"et": "0,1"}, "0,1"},
		{"without et", map[string]string{}, "0"},
		{"nil map", nil, "0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &AirPlayDevice{TXTRecord: tt.txtRecord}
			got := d.EncryptionTypes()
			if got != tt.want {
				t.Errorf("EncryptionTypes() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAirPlayDevice_SupportsALAC(t *testing.T) {
	tests := []struct {
		name      string
		txtRecord map[string]string
		want      bool
	}{
		{"supports ALAC", map[string]string{"cn": "0,1,2"}, true},
		{"no ALAC", map[string]string{"cn": "0,2,3"}, false},
		{"no cn key", map[string]string{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &AirPlayDevice{TXTRecord: tt.txtRecord}
			got := d.SupportsALAC()
			if got != tt.want {
				t.Errorf("SupportsALAC() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestService_StartStop(t *testing.T) {
	s := NewService()
	ctx, cancel := context.WithCancel(context.Background())

	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Give it a moment to start
	time.Sleep(50 * time.Millisecond)

	// Stop via cancel
	cancel()
	s.Stop()

	// Should not panic or hang
}
