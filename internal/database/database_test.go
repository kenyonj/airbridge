package database

import (
	"testing"
	"time"
)

func TestOpenClose(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Errorf("failed to close db: %v", err)
	}
}

func TestRendererCRUD(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	// Create
	renderer := &Renderer{
		ID:              "test-uuid-123",
		Name:            "Living Room",
		AirPlayDeviceID: "device-abc",
		AirPlayName:     "Living Room Speaker",
		Port:            8200,
		Enabled:         true,
		CreatedAt:       time.Now().Truncate(time.Second),
	}

	if err := db.CreateRenderer(renderer); err != nil {
		t.Fatalf("CreateRenderer failed: %v", err)
	}

	// Read
	got, err := db.GetRenderer("test-uuid-123")
	if err != nil {
		t.Fatalf("GetRenderer failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected renderer, got nil")
	}
	if got.Name != "Living Room" {
		t.Errorf("Name = %q, want %q", got.Name, "Living Room")
	}
	if got.Port != 8200 {
		t.Errorf("Port = %d, want %d", got.Port, 8200)
	}
	if !got.Enabled {
		t.Error("expected Enabled to be true")
	}

	// Update
	renderer.Name = "Kitchen"
	renderer.Port = 8201
	if err := db.UpdateRenderer(renderer); err != nil {
		t.Fatalf("UpdateRenderer failed: %v", err)
	}

	got, _ = db.GetRenderer("test-uuid-123")
	if got.Name != "Kitchen" {
		t.Errorf("after update, Name = %q, want %q", got.Name, "Kitchen")
	}
	if got.Port != 8201 {
		t.Errorf("after update, Port = %d, want %d", got.Port, 8201)
	}

	// Delete
	if err := db.DeleteRenderer("test-uuid-123"); err != nil {
		t.Fatalf("DeleteRenderer failed: %v", err)
	}

	got, _ = db.GetRenderer("test-uuid-123")
	if got != nil {
		t.Error("expected nil after delete, got renderer")
	}
}

func TestListRenderers(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	// Empty list
	list, err := db.ListRenderers()
	if err != nil {
		t.Fatalf("ListRenderers failed: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d items", len(list))
	}

	// Add renderers
	now := time.Now()
	for i, name := range []string{"Living Room", "Kitchen", "Bedroom"} {
		r := &Renderer{
			ID:              name + "-id",
			Name:            name,
			AirPlayDeviceID: name + "-device",
			AirPlayName:     name + " Speaker",
			Port:            8200 + i,
			Enabled:         true,
			CreatedAt:       now.Add(time.Duration(i) * time.Second),
		}
		if err := db.CreateRenderer(r); err != nil {
			t.Fatalf("CreateRenderer failed: %v", err)
		}
	}

	list, err = db.ListRenderers()
	if err != nil {
		t.Fatalf("ListRenderers failed: %v", err)
	}
	if len(list) != 3 {
		t.Errorf("expected 3 renderers, got %d", len(list))
	}

	// Verify order (by created_at ASC)
	if list[0].Name != "Living Room" {
		t.Errorf("first renderer should be 'Living Room', got %q", list[0].Name)
	}
}

func TestToggleRenderer(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	r := &Renderer{
		ID:              "toggle-test",
		Name:            "Test",
		AirPlayDeviceID: "device",
		Port:            8200,
		Enabled:         true,
		CreatedAt:       time.Now(),
	}
	_ = db.CreateRenderer(r)

	// Toggle off
	if err := db.ToggleRenderer("toggle-test"); err != nil {
		t.Fatalf("ToggleRenderer failed: %v", err)
	}
	got, _ := db.GetRenderer("toggle-test")
	if got.Enabled {
		t.Error("expected Enabled to be false after toggle")
	}

	// Toggle on
	_ = db.ToggleRenderer("toggle-test")
	got, _ = db.GetRenderer("toggle-test")
	if !got.Enabled {
		t.Error("expected Enabled to be true after second toggle")
	}
}

func TestRenameRenderer(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	r := &Renderer{
		ID:              "rename-test",
		Name:            "Original",
		AirPlayDeviceID: "device",
		Port:            8200,
		Enabled:         true,
		CreatedAt:       time.Now(),
	}
	_ = db.CreateRenderer(r)

	if err := db.RenameRenderer("rename-test", "New Name"); err != nil {
		t.Fatalf("RenameRenderer failed: %v", err)
	}

	got, _ := db.GetRenderer("rename-test")
	if got.Name != "New Name" {
		t.Errorf("Name = %q, want %q", got.Name, "New Name")
	}
}

func TestGetNextPort(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	// No renderers - returns base port
	port, err := db.GetNextPort(8200)
	if err != nil {
		t.Fatalf("GetNextPort failed: %v", err)
	}
	if port != 8200 {
		t.Errorf("expected 8200, got %d", port)
	}

	// Add renderer with port 8200
	_ = db.CreateRenderer(&Renderer{
		ID:              "r1",
		Name:            "R1",
		AirPlayDeviceID: "d1",
		Port:            8200,
		CreatedAt:       time.Now(),
	})

	port, _ = db.GetNextPort(8200)
	if port != 8201 {
		t.Errorf("expected 8201, got %d", port)
	}

	// Add renderer with port 8205
	_ = db.CreateRenderer(&Renderer{
		ID:              "r2",
		Name:            "R2",
		AirPlayDeviceID: "d2",
		Port:            8205,
		CreatedAt:       time.Now(),
	})

	port, _ = db.GetNextPort(8200)
	if port != 8206 {
		t.Errorf("expected 8206 (max+1), got %d", port)
	}
}

func TestSettings(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	// Non-existent setting
	val, err := db.GetSetting("nonexistent")
	if err != nil {
		t.Fatalf("GetSetting failed: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty string for nonexistent key, got %q", val)
	}

	// Set and get
	if err := db.SetSetting("theme", "dark"); err != nil {
		t.Fatalf("SetSetting failed: %v", err)
	}
	val, _ = db.GetSetting("theme")
	if val != "dark" {
		t.Errorf("expected 'dark', got %q", val)
	}

	// Update existing
	_ = db.SetSetting("theme", "light")
	val, _ = db.GetSetting("theme")
	if val != "light" {
		t.Errorf("expected 'light' after update, got %q", val)
	}
}

func TestGetRenderer_NotFound(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	got, err := db.GetRenderer("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Error("expected nil for nonexistent renderer")
	}
}
