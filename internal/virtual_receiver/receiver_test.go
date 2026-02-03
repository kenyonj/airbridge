package virtual_receiver

import (
	"context"
	"testing"
)

// mockPlayer is a mock implementation of Player for testing.
type mockPlayer struct {
	playCallCount int
	stopCallCount int
	lastVolume    int
}

func (m *mockPlayer) Play(ctx context.Context, uri string, volume int) error {
	m.playCallCount++
	m.lastVolume = volume
	return nil
}

func (m *mockPlayer) Stop(ctx context.Context) error {
	m.stopCallCount++
	return nil
}

func (m *mockPlayer) SetVolume(ctx context.Context, volume int) error {
	m.lastVolume = volume
	return nil
}

func TestNewReceiver(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: Config{
				Name:   "Test Receiver",
				Player: &mockPlayer{},
			},
			wantErr: false,
		},
		{
			name: "missing name",
			config: Config{
				Player: &mockPlayer{},
			},
			wantErr: true,
		},
		{
			name: "missing player",
			config: Config{
				Name: "Test",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			receiver, err := NewReceiver(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewReceiver() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && receiver == nil {
				t.Error("NewReceiver() returned nil receiver")
			}
		})
	}
}

func TestReceiver_Name(t *testing.T) {
	config := Config{
		Name:   "My Receiver",
		Player: &mockPlayer{},
	}
	receiver, err := NewReceiver(config)
	if err != nil {
		t.Fatalf("NewReceiver() failed: %v", err)
	}

	if got := receiver.Name(); got != "My Receiver" {
		t.Errorf("Name() = %v, want %v", got, "My Receiver")
	}
}

func TestReceiver_IsRunning(t *testing.T) {
	config := Config{
		Name:   "Test",
		Player: &mockPlayer{},
	}
	receiver, err := NewReceiver(config)
	if err != nil {
		t.Fatalf("NewReceiver() failed: %v", err)
	}

	if receiver.IsRunning() {
		t.Error("IsRunning() = true, want false before Start()")
	}

	// Note: We can't actually test Start() without shairport-sync installed
	// In a real test environment, we would mock the exec.Command
}
