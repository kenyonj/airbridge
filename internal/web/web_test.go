package web

import (
	"testing"
	"time"
)

func TestGenerateID(t *testing.T) {
	tests := []struct {
		name     string
		deviceID string
	}{
		{"simple id", "AABBCCDD1234"},
		{"with colons", "AA:BB:CC:DD:12:34"},
		{"long id", "C6F4891B35E7@Living Room"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := generateID(tt.deviceID)

			// Should be a valid UUID format
			if len(id) != 36 {
				t.Errorf("generateID() length = %d, want 36", len(id))
			}

			// Should be deterministic
			id2 := generateID(tt.deviceID)
			if id != id2 {
				t.Error("generateID() should be deterministic")
			}

			// Different inputs should produce different outputs
			id3 := generateID(tt.deviceID + "x")
			if id == id3 {
				t.Error("different inputs should produce different IDs")
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		want     string
	}{
		{"zero", 0, "0s"},
		{"seconds only", 45 * time.Second, "45s"},
		{"minutes and seconds", 5*time.Minute + 30*time.Second, "5m 30s"},
		{"hours minutes seconds", 2*time.Hour + 15*time.Minute + 45*time.Second, "2h 15m 45s"},
		{"just minutes", 10 * time.Minute, "10m 0s"},
		{"just hours", 3 * time.Hour, "3h 0m 0s"},
		{"rounds to nearest second", 5*time.Second + 500*time.Millisecond, "6s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDuration(tt.duration)
			if got != tt.want {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.duration, got, tt.want)
			}
		})
	}
}

func TestDeviceInfo(t *testing.T) {
	// Test that DeviceInfo struct can be created
	info := DeviceInfo{
		DeviceID:   "AABBCCDD1234",
		Name:       "Living Room",
		Host:       "192.168.1.100",
		Port:       7000,
		Model:      "Shairport Sync",
		Configured: true,
	}

	if info.DeviceID != "AABBCCDD1234" {
		t.Errorf("DeviceID = %q, want %q", info.DeviceID, "AABBCCDD1234")
	}
	if !info.Configured {
		t.Error("expected Configured to be true")
	}
}

func TestRendererView(t *testing.T) {
	// Test that RendererView embeds Renderer correctly
	view := RendererView{
		Running: true,
		DLNAURL: "http://192.168.1.50:8200/device.xml",
	}
	view.ID = "test-id"
	view.Name = "Test Renderer"

	if view.ID != "test-id" {
		t.Errorf("ID = %q, want %q", view.ID, "test-id")
	}
	if !view.Running {
		t.Error("expected Running to be true")
	}
}
