// Package player provides the RAOP and Chromecast audio player implementations.
package player

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/kenyonj/airbridge/internal/discovery"
	"github.com/vishen/go-chromecast/application"
)

// ChromecastPlayer streams audio to a Chromecast device via CASTV2.
type ChromecastPlayer struct {
	mu sync.Mutex

	device *discovery.Device
	app    *application.Application

	// Current state
	currentURI string
	volume     int
}

// NewChromecastPlayer creates a new Chromecast player for the given device.
func NewChromecastPlayer(device *discovery.Device) *ChromecastPlayer {
	return &ChromecastPlayer{
		device: device,
		volume: 80,
	}
}

// Play starts streaming audio from the given URI to the Chromecast device.
func (p *ChromecastPlayer) Play(ctx context.Context, uri string, volume int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Stop any existing stream
	p.stopInternal()

	p.currentURI = uri
	p.volume = volume

	log.Printf("Chromecast Play: %s -> %s:%d (volume=%d)", uri, p.device.Host, p.device.Port, volume)

	// Resolve IP address
	addr, err := net.ResolveIPAddr("ip", p.device.Host)
	if err != nil {
		return fmt.Errorf("failed to resolve host: %w", err)
	}

	// Create a new Chromecast connection
	app := application.NewApplication(
		application.WithDebug(false),
		application.WithCacheDisabled(true),
	)

	// Connect to the device
	if err := app.Start(addr.IP.String(), p.device.Port); err != nil {
		return fmt.Errorf("failed to connect to Chromecast: %w", err)
	}
	p.app = app

	// Set volume (Chromecast uses 0.0-1.0 scale)
	volumeFloat := float32(volume) / 100.0
	if err := app.SetVolume(volumeFloat); err != nil {
		log.Printf("Failed to set Chromecast volume: %v", err)
		// Non-fatal, continue with playback
	}

	// Detect content type from URI (default to audio/mpeg)
	contentType := detectContentType(uri)

	// Load the media URL
	// Chromecast will fetch the media directly from the URL
	// Load signature: Load(url string, startTime int, contentType string, transcode bool, detach bool, forceDetach bool)
	if err := app.Load(uri, 0, contentType, false, false, false); err != nil {
		p.stopInternal()
		return fmt.Errorf("failed to load media: %w", err)
	}

	log.Printf("Chromecast loaded media: %s (type=%s)", uri, contentType)
	return nil
}

// detectContentType tries to detect the content type from the URI.
func detectContentType(uri string) string {
	// Simple extension-based detection
	switch {
	case hasExtension(uri, ".mp3"):
		return "audio/mpeg"
	case hasExtension(uri, ".flac"):
		return "audio/flac"
	case hasExtension(uri, ".wav"):
		return "audio/wav"
	case hasExtension(uri, ".ogg"):
		return "audio/ogg"
	case hasExtension(uri, ".m4a"), hasExtension(uri, ".aac"):
		return "audio/mp4"
	case hasExtension(uri, ".mp4"):
		return "video/mp4"
	case hasExtension(uri, ".mkv"):
		return "video/x-matroska"
	case hasExtension(uri, ".webm"):
		return "video/webm"
	default:
		return "audio/mpeg" // Default to MP3
	}
}

func hasExtension(uri, ext string) bool {
	// Simple check - look for extension before query string
	for i := len(uri) - 1; i >= 0; i-- {
		if uri[i] == '?' {
			uri = uri[:i]
			break
		}
	}
	if len(uri) < len(ext) {
		return false
	}
	return uri[len(uri)-len(ext):] == ext
}

// Pause pauses the current stream.
func (p *ChromecastPlayer) Pause(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	log.Printf("Chromecast Pause")
	if p.app == nil {
		return nil
	}

	if err := p.app.Pause(); err != nil {
		return fmt.Errorf("failed to pause: %w", err)
	}
	return nil
}

// Stop stops the current stream.
func (p *ChromecastPlayer) Stop(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	log.Printf("Chromecast Stop")
	p.stopInternal()
	return nil
}

// stopInternal stops all resources without locking.
func (p *ChromecastPlayer) stopInternal() {
	if p.app != nil {
		// Try to stop the media first
		_ = p.app.Stop()
		// Give it a moment to process
		time.Sleep(100 * time.Millisecond)
		// Close the connection (stopMedia=true)
		_ = p.app.Close(true)
		p.app = nil
	}
}

// SetVolume adjusts the volume.
func (p *ChromecastPlayer) SetVolume(ctx context.Context, volume int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	log.Printf("Chromecast SetVolume: %d", volume)
	p.volume = volume

	if p.app == nil {
		return nil // Will apply on next Play
	}

	volumeFloat := float32(volume) / 100.0
	if err := p.app.SetVolume(volumeFloat); err != nil {
		return fmt.Errorf("failed to set volume: %w", err)
	}
	return nil
}

// Unpause resumes a paused stream.
func (p *ChromecastPlayer) Unpause(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	log.Printf("Chromecast Unpause")
	if p.app == nil {
		return nil
	}

	if err := p.app.Unpause(); err != nil {
		return fmt.Errorf("failed to unpause: %w", err)
	}
	return nil
}

// Ensure ChromecastPlayer satisfies the same interface as RAOPPlayer
var _ interface {
	Play(ctx context.Context, uri string, volume int) error
	Pause(ctx context.Context) error
	Stop(ctx context.Context) error
	SetVolume(ctx context.Context, volume int) error
} = (*ChromecastPlayer)(nil)
