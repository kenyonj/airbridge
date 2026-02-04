package player

import (
	"context"
	"testing"

	"github.com/kenyonj/airbridge/internal/discovery"
)

func TestNewRAOPPlayer(t *testing.T) {
	device := &discovery.Device{
		Name:       "Living Room",
		DeviceID:   "AABBCCDD1234",
		DeviceType: discovery.DeviceTypeAirPlay,
		Host:       "192.168.1.100",
		Port:       7000,
		Model:      "Shairport Sync",
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
	device := &discovery.Device{
		DeviceType: discovery.DeviceTypeAirPlay,
		Host:       "192.168.1.100",
		Port:       7000,
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
	device := &discovery.Device{
		DeviceType: discovery.DeviceTypeAirPlay,
		Host:       "192.168.1.100",
		Port:       7000,
	}

	player := NewRAOPPlayer(device)

	// Stop should not panic when there's no active stream
	err := player.Stop(context.Background())
	if err != nil {
		t.Errorf("Stop on idle player failed: %v", err)
	}
}

func TestRAOPPlayer_Pause_NoActiveStream(t *testing.T) {
	device := &discovery.Device{
		DeviceType: discovery.DeviceTypeAirPlay,
		Host:       "192.168.1.100",
		Port:       7000,
	}

	player := NewRAOPPlayer(device)

	// Pause should not panic when there's no active stream
	err := player.Pause(context.Background())
	if err != nil {
		t.Errorf("Pause on idle player failed: %v", err)
	}
}

// Chromecast player tests

func TestNewChromecastPlayer(t *testing.T) {
	device := &discovery.Device{
		Name:       "Living Room TV",
		DeviceID:   "abc123",
		DeviceType: discovery.DeviceTypeChromecast,
		Host:       "192.168.1.101",
		Port:       8009,
		Model:      "Chromecast",
	}

	player := NewChromecastPlayer(device)

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

func TestChromecastPlayer_SetVolume_NoConnection(t *testing.T) {
	device := &discovery.Device{
		DeviceType: discovery.DeviceTypeChromecast,
		Host:       "192.168.1.101",
		Port:       8009,
	}

	player := NewChromecastPlayer(device)

	// SetVolume should store the volume for next play (no error when not connected)
	err := player.SetVolume(context.Background(), 50)
	if err != nil {
		t.Errorf("SetVolume failed: %v", err)
	}
	if player.volume != 50 {
		t.Errorf("volume = %d, want 50", player.volume)
	}
}

func TestChromecastPlayer_Stop_NoActiveStream(t *testing.T) {
	device := &discovery.Device{
		DeviceType: discovery.DeviceTypeChromecast,
		Host:       "192.168.1.101",
		Port:       8009,
	}

	player := NewChromecastPlayer(device)

	// Stop should not panic when there's no active connection
	err := player.Stop(context.Background())
	if err != nil {
		t.Errorf("Stop on idle player failed: %v", err)
	}
}

func TestChromecastPlayer_Pause_NoActiveStream(t *testing.T) {
	device := &discovery.Device{
		DeviceType: discovery.DeviceTypeChromecast,
		Host:       "192.168.1.101",
		Port:       8009,
	}

	player := NewChromecastPlayer(device)

	// Pause should not panic when there's no active connection
	err := player.Pause(context.Background())
	if err != nil {
		t.Errorf("Pause on idle player failed: %v", err)
	}
}

func TestChromecastPlayer_Unpause_NoActiveStream(t *testing.T) {
	device := &discovery.Device{
		DeviceType: discovery.DeviceTypeChromecast,
		Host:       "192.168.1.101",
		Port:       8009,
	}

	player := NewChromecastPlayer(device)

	// Unpause should not panic when there's no active connection
	err := player.Unpause(context.Background())
	if err != nil {
		t.Errorf("Unpause on idle player failed: %v", err)
	}
}

func TestDetectContentType(t *testing.T) {
	tests := []struct {
		uri  string
		want string
	}{
		{"http://example.com/audio.mp3", "audio/mpeg"},
		{"http://example.com/audio.flac", "audio/flac"},
		{"http://example.com/audio.wav", "audio/wav"},
		{"http://example.com/audio.ogg", "audio/ogg"},
		{"http://example.com/audio.m4a", "audio/mp4"},
		{"http://example.com/audio.aac", "audio/mp4"},
		{"http://example.com/video.mp4", "video/mp4"},
		{"http://example.com/video.mkv", "video/x-matroska"},
		{"http://example.com/video.webm", "video/webm"},
		{"http://example.com/unknown.xyz", "audio/mpeg"}, // default
		{"http://example.com/file.mp3?token=abc", "audio/mpeg"},
	}

	for _, tt := range tests {
		t.Run(tt.uri, func(t *testing.T) {
			got := detectContentType(tt.uri)
			if got != tt.want {
				t.Errorf("detectContentType(%q) = %q, want %q", tt.uri, got, tt.want)
			}
		})
	}
}

func TestHasExtension(t *testing.T) {
	tests := []struct {
		uri  string
		ext  string
		want bool
	}{
		{"http://example.com/file.mp3", ".mp3", true},
		{"http://example.com/file.mp3?token=abc", ".mp3", true},
		{"http://example.com/file.flac", ".mp3", false},
		{"http://example.com/file", ".mp3", false},
		{"", ".mp3", false},
	}

	for _, tt := range tests {
		t.Run(tt.uri, func(t *testing.T) {
			got := hasExtension(tt.uri, tt.ext)
			if got != tt.want {
				t.Errorf("hasExtension(%q, %q) = %v, want %v", tt.uri, tt.ext, got, tt.want)
			}
		})
	}
}
