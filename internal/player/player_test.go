package player

import (
	"context"
	"testing"

	"github.com/kenyonj/airbridge/internal/discovery"
)

func TestNewRAOPPlayer(t *testing.T) {
	device := &discovery.AirPlayDevice{
		Name:     "Living Room",
		DeviceID: "AABBCCDD1234",
		Host:     "192.168.1.100",
		Port:     7000,
		Model:    "Shairport Sync",
	}

	player := NewRAOPPlayer(device)

	if player == nil {
		t.Fatal("expected non-nil player")
	}
	if player.device != device {
		t.Error("player device should match input device")
	}
	if player.volume != 80 {
		t.Errorf("default volume = %d, want 80", player.volume)
	}
}

func TestRAOPPlayer_SetVolume(t *testing.T) {
	device := &discovery.AirPlayDevice{
		Host: "192.168.1.100",
		Port: 7000,
	}

	player := NewRAOPPlayer(device)

	// SetVolume should store the volume for next play
	err := player.SetVolume(context.Background(), 50)
	if err != nil {
		t.Errorf("SetVolume failed: %v", err)
	}
	if player.volume != 50 {
		t.Errorf("volume = %d, want 50", player.volume)
	}
}

func TestNullPlayer(t *testing.T) {
	var player NullPlayer
	ctx := context.Background()

	// All methods should return nil without error (except Play which needs a valid URL)
	if err := player.Pause(ctx); err != nil {
		t.Errorf("Pause failed: %v", err)
	}

	if err := player.Stop(ctx); err != nil {
		t.Errorf("Stop failed: %v", err)
	}

	if err := player.SetVolume(ctx, 75); err != nil {
		t.Errorf("SetVolume failed: %v", err)
	}
}

func TestRAOPPlayer_Stop_NoActiveStream(t *testing.T) {
	device := &discovery.AirPlayDevice{
		Host: "192.168.1.100",
		Port: 7000,
	}

	player := NewRAOPPlayer(device)

	// Stop should not panic when there's no active stream
	err := player.Stop(context.Background())
	if err != nil {
		t.Errorf("Stop on idle player failed: %v", err)
	}
}

func TestRAOPPlayer_Pause_NoActiveStream(t *testing.T) {
	device := &discovery.AirPlayDevice{
		Host: "192.168.1.100",
		Port: 7000,
	}

	player := NewRAOPPlayer(device)

	// Pause should not panic when there's no active stream
	err := player.Pause(context.Background())
	if err != nil {
		t.Errorf("Pause on idle player failed: %v", err)
	}
}
