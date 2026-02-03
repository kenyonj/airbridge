package bridge

import (
	"testing"

	"github.com/kenyonj/airbridge/internal/database"
	"github.com/kenyonj/airbridge/internal/discovery"
)

func TestNewBridge(t *testing.T) {
	db, err := database.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	disco := discovery.NewService()
	bridge := NewBridge(db, disco, 8200)

	if bridge == nil {
		t.Fatal("expected non-nil bridge")
	}
	if bridge.port != 8200 {
		t.Errorf("port = %d, want 8200", bridge.port)
	}
	if bridge.renderers == nil {
		t.Error("renderers map should be initialized")
	}
}

func TestBridge_GetRenderer_NotFound(t *testing.T) {
	db, _ := database.Open(":memory:")
	defer db.Close()

	disco := discovery.NewService()
	bridge := NewBridge(db, disco, 8200)

	r := bridge.GetRenderer("nonexistent")
	if r != nil {
		t.Error("expected nil for nonexistent renderer")
	}
}

func TestBridge_GetRenderers_Empty(t *testing.T) {
	db, _ := database.Open(":memory:")
	defer db.Close()

	disco := discovery.NewService()
	bridge := NewBridge(db, disco, 8200)

	renderers := bridge.GetRenderers()
	if len(renderers) != 0 {
		t.Errorf("expected empty slice, got %d renderers", len(renderers))
	}
}

func TestBridge_IsRunning_NotFound(t *testing.T) {
	db, _ := database.Open(":memory:")
	defer db.Close()

	disco := discovery.NewService()
	bridge := NewBridge(db, disco, 8200)

	if bridge.IsRunning("nonexistent") {
		t.Error("expected IsRunning to be false for nonexistent renderer")
	}
}

func TestBridge_RunningCount_Empty(t *testing.T) {
	db, _ := database.Open(":memory:")
	defer db.Close()

	disco := discovery.NewService()
	bridge := NewBridge(db, disco, 8200)

	if bridge.RunningCount() != 0 {
		t.Errorf("RunningCount = %d, want 0", bridge.RunningCount())
	}
}

func TestRendererInstance_Struct(t *testing.T) {
	inst := &RendererInstance{
		ID:          "test-uuid",
		Name:        "Test Renderer",
		AirPlayName: "Test Speaker",
	}

	if inst.ID != "test-uuid" {
		t.Errorf("ID = %q, want %q", inst.ID, "test-uuid")
	}
	if inst.Name != "Test Renderer" {
		t.Errorf("Name = %q, want %q", inst.Name, "Test Renderer")
	}
	if inst.AirPlayName != "Test Speaker" {
		t.Errorf("AirPlayName = %q, want %q", inst.AirPlayName, "Test Speaker")
	}
}
