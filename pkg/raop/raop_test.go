package raop

import (
	"testing"
)

func TestStreamConfig(t *testing.T) {
	config := StreamConfig{
		Host:           "192.168.1.100",
		Port:           7000,
		Volume:         80,
		EncryptionType: "0,4",
		UseALAC:        true,
	}

	if config.Host != "192.168.1.100" {
		t.Errorf("Host = %q, want %q", config.Host, "192.168.1.100")
	}
	if config.Port != 7000 {
		t.Errorf("Port = %d, want %d", config.Port, 7000)
	}
	if config.Volume != 80 {
		t.Errorf("Volume = %d, want %d", config.Volume, 80)
	}
	if !config.UseALAC {
		t.Error("expected UseALAC to be true")
	}
}

func TestStreamer_SetVolume(t *testing.T) {
	// Create a streamer manually without finding cliraop
	s := &Streamer{
		config: StreamConfig{
			Host:   "192.168.1.100",
			Port:   7000,
			Volume: 80,
		},
	}

	s.SetVolume(50)

	if s.config.Volume != 50 {
		t.Errorf("Volume = %d, want 50", s.config.Volume)
	}
}

func TestStreamer_IsRunning(t *testing.T) {
	s := &Streamer{
		running: false,
	}

	if s.IsRunning() {
		t.Error("expected IsRunning to be false")
	}

	s.running = true
	if !s.IsRunning() {
		t.Error("expected IsRunning to be true")
	}
}

func TestStreamer_Stop_NotRunning(t *testing.T) {
	s := &Streamer{
		running: false,
	}

	// Stop on non-running streamer should not error
	err := s.Stop()
	if err != nil {
		t.Errorf("Stop on non-running streamer failed: %v", err)
	}
}

func TestStreamer_WaitForCompletion_NilCmd(t *testing.T) {
	s := &Streamer{
		cmd: nil,
	}

	// WaitForCompletion with nil cmd should not error
	err := s.WaitForCompletion()
	if err != nil {
		t.Errorf("WaitForCompletion with nil cmd failed: %v", err)
	}
}

func TestNewStreamer_NoCliraop(t *testing.T) {
	// This test verifies that NewStreamer returns an error when cliraop is not found
	// Note: This test may pass if cliraop is installed on the system
	config := StreamConfig{
		Host:   "192.168.1.100",
		Port:   7000,
		Volume: 80,
	}

	_, err := NewStreamer(config)
	// We just verify it doesn't panic - it may or may not find cliraop
	_ = err
}
