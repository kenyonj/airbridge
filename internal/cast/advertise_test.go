package cast

import (
	"testing"
)

func TestGenerateDeviceID(t *testing.T) {
	tests := []struct {
		name string
		uuid string
	}{
		{"simple uuid", "abc123"},
		{"full uuid", "550e8400-e29b-41d4-a716-446655440000"},
		{"another uuid", "renderer-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id1 := generateDeviceID(tt.uuid)
			id2 := generateDeviceID(tt.uuid)

			// Should be deterministic
			if id1 != id2 {
				t.Errorf("generateDeviceID not deterministic: %q != %q", id1, id2)
			}

			// Should be 32 hex chars
			if len(id1) != 32 {
				t.Errorf("expected 32 chars, got %d", len(id1))
			}
		})
	}

	// Different UUIDs should produce different IDs
	id1 := generateDeviceID("uuid1")
	id2 := generateDeviceID("uuid2")
	if id1 == id2 {
		t.Error("different UUIDs should produce different device IDs")
	}
}

func TestNewAdvertiser(t *testing.T) {
	a := NewAdvertiser()
	if a == nil {
		t.Fatal("expected non-nil advertiser")
	}
	if a.services == nil {
		t.Error("expected initialized services map")
	}
}

func TestAdvertiser_IsAdvertising_Empty(t *testing.T) {
	a := NewAdvertiser()
	if a.IsAdvertising("nonexistent") {
		t.Error("expected false for non-advertised device")
	}
}

func TestAdvertiser_AdvertisedCount_Empty(t *testing.T) {
	a := NewAdvertiser()
	if a.AdvertisedCount() != 0 {
		t.Errorf("expected 0, got %d", a.AdvertisedCount())
	}
}

func TestDeviceConfig(t *testing.T) {
	cfg := DeviceConfig{
		UUID:         "test-uuid",
		FriendlyName: "Living Room",
		Model:        "Airbridge",
		Port:         8009,
	}

	if cfg.UUID != "test-uuid" {
		t.Error("UUID mismatch")
	}
	if cfg.FriendlyName != "Living Room" {
		t.Error("FriendlyName mismatch")
	}
	if cfg.Model != "Airbridge" {
		t.Error("Model mismatch")
	}
	if cfg.Port != 8009 {
		t.Error("Port mismatch")
	}
}
