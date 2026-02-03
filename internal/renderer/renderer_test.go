package renderer

import (
	"testing"

	"github.com/kenyonj/airbridge/pkg/config"
)

func TestGenerateDeterministicUUID(t *testing.T) {
	tests := []struct {
		name     string
		deviceID string
	}{
		{"simple id", "AABBCCDD1234"},
		{"with colons", "AA:BB:CC:DD:12:34"},
		{"with name", "C6F4891B35E7@Living Room"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uuid := generateDeterministicUUID(tt.deviceID)

			// Should be a valid UUID format (36 chars)
			if len(uuid) != 36 {
				t.Errorf("UUID length = %d, want 36", len(uuid))
			}

			// Should be deterministic
			uuid2 := generateDeterministicUUID(tt.deviceID)
			if uuid != uuid2 {
				t.Error("UUID should be deterministic")
			}

			// Different inputs should produce different outputs
			uuid3 := generateDeterministicUUID(tt.deviceID + "x")
			if uuid == uuid3 {
				t.Error("different inputs should produce different UUIDs")
			}
		})
	}
}

func TestNewManager(t *testing.T) {
	cfg := config.Config{
		HTTPPort:     8200,
		AutoDiscover: true,
		NamePrefix:   "Airbridge",
	}

	manager := NewManager(cfg)

	if manager == nil {
		t.Fatal("expected non-nil manager")
	}
	if manager.basePort != 8200 {
		t.Errorf("basePort = %d, want 8200", manager.basePort)
	}
	if manager.instances == nil {
		t.Error("instances map should be initialized")
	}
}

func TestManager_GetInstances_Empty(t *testing.T) {
	cfg := config.Config{HTTPPort: 8200}
	manager := NewManager(cfg)

	instances := manager.GetInstances()
	if len(instances) != 0 {
		t.Errorf("expected empty slice, got %d instances", len(instances))
	}
}

func TestInstance_Struct(t *testing.T) {
	// Test that Instance struct can be created
	inst := &Instance{
		UUID:         "test-uuid",
		FriendlyName: "Test Renderer",
		Port:         8200,
	}

	if inst.UUID != "test-uuid" {
		t.Errorf("UUID = %q, want %q", inst.UUID, "test-uuid")
	}
	if inst.FriendlyName != "Test Renderer" {
		t.Errorf("FriendlyName = %q, want %q", inst.FriendlyName, "Test Renderer")
	}
}
