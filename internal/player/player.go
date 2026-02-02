// Package player provides the RAOP audio player implementation.
package player

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"

	"github.com/kenyonj/airbridge/internal/discovery"
	"github.com/kenyonj/airbridge/pkg/raop"
)

// RAOPPlayer streams audio to an AirPlay device via RAOP.
type RAOPPlayer struct {
	mu sync.Mutex

	device   *discovery.AirPlayDevice
	streamer *raop.Streamer
	cancel   context.CancelFunc

	// Current state
	currentURI string
	volume     int
}

// NewRAOPPlayer creates a new RAOP player for the given AirPlay device.
func NewRAOPPlayer(device *discovery.AirPlayDevice) *RAOPPlayer {
	return &RAOPPlayer{
		device: device,
		volume: 80,
	}
}

// Play starts streaming audio from the given URI to the AirPlay device.
func (p *RAOPPlayer) Play(ctx context.Context, uri string, volume int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Stop any existing stream
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}

	p.currentURI = uri
	p.volume = volume

	log.Printf("RAOP Play: %s -> %s:%d (volume=%d)", uri, p.device.Host, p.device.Port, volume)

	// Create a new context for this playback
	playCtx, cancel := context.WithCancel(ctx)
	p.cancel = cancel

	// Create streamer for the device
	streamer, err := raop.NewStreamer(raop.StreamConfig{
		Host:   p.device.Host,
		Port:   p.device.Port,
		Volume: volume,
	})
	if err != nil {
		cancel()
		return fmt.Errorf("failed to create streamer: %w", err)
	}
	p.streamer = streamer

	// Fetch the audio stream
	resp, err := http.Get(uri)
	if err != nil {
		cancel()
		return fmt.Errorf("failed to fetch audio: %w", err)
	}

	// Start streaming in a goroutine
	go func() {
		defer resp.Body.Close()
		defer func() {
			p.mu.Lock()
			p.cancel = nil
			p.mu.Unlock()
		}()

		if err := p.streamer.Start(playCtx, resp.Body); err != nil {
			if err != context.Canceled {
				log.Printf("RAOP streaming error: %v", err)
			}
		}
	}()

	return nil
}

// Pause pauses the current stream.
// Note: RAOP doesn't support true pause, so we just stop.
func (p *RAOPPlayer) Pause(ctx context.Context) error {
	return p.Stop(ctx)
}

// Stop stops the current stream.
func (p *RAOPPlayer) Stop(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	log.Printf("RAOP Stop")

	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}

	if p.streamer != nil {
		p.streamer.Stop()
		p.streamer = nil
	}

	return nil
}

// SetVolume adjusts the volume.
func (p *RAOPPlayer) SetVolume(ctx context.Context, volume int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	log.Printf("RAOP SetVolume: %d", volume)
	p.volume = volume

	// Note: Changing volume mid-stream would require restarting cliraop
	// For now, just record the value for the next Play call

	return nil
}

// NullPlayer is a no-op player for testing.
type NullPlayer struct{}

func (NullPlayer) Play(ctx context.Context, uri string, volume int) error {
	log.Printf("[NullPlayer] Play: %s (volume=%d)", uri, volume)
	// Simulate fetching to verify URI works
	resp, err := http.Get(uri)
	if err != nil {
		return fmt.Errorf("fetch failed: %w", err)
	}
	defer resp.Body.Close()
	
	n, _ := io.Copy(io.Discard, resp.Body)
	log.Printf("[NullPlayer] Streamed %d bytes", n)
	return nil
}

func (NullPlayer) Pause(ctx context.Context) error {
	log.Printf("[NullPlayer] Pause")
	return nil
}

func (NullPlayer) Stop(ctx context.Context) error {
	log.Printf("[NullPlayer] Stop")
	return nil
}

func (NullPlayer) SetVolume(ctx context.Context, volume int) error {
	log.Printf("[NullPlayer] SetVolume: %d", volume)
	return nil
}
