// Package player provides the RAOP audio player implementation.
package player

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"sync"

	"github.com/kenyonj/airbridge/internal/discovery"
	"github.com/kenyonj/airbridge/pkg/raop"
)

// VolumePassthrough configures external volume control API.
type VolumePassthrough struct {
	URL      string // URL with {{volume}} placeholder
	Method   string // HTTP method (GET, POST, PUT)
	Body     string // Request body with {{volume}} placeholder
	AuthUser string // Basic auth username
	AuthPass string // Basic auth password
}

// RAOPPlayer streams audio to an AirPlay device via RAOP.
type RAOPPlayer struct {
	mu sync.Mutex

	device   *discovery.Device
	streamer *raop.Streamer
	ffmpeg   *exec.Cmd
	cancel   context.CancelFunc

	// Current state
	currentURI string
	volume     int

	// Volume passthrough for external API control
	volumePassthrough *VolumePassthrough
}

// NewRAOPPlayer creates a new RAOP player for the given AirPlay device.
func NewRAOPPlayer(device *discovery.Device) *RAOPPlayer {
	return &RAOPPlayer{
		device: device,
		volume: 80,
	}
}

// SetVolumePassthrough configures external volume control API.
func (p *RAOPPlayer) SetVolumePassthrough(vp *VolumePassthrough) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.volumePassthrough = vp
}

// Play starts streaming audio from the given URI to the AirPlay device.
func (p *RAOPPlayer) Play(ctx context.Context, uri string, volume int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Stop any existing stream
	p.stopInternal()

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

	// Use FFmpeg to decode the audio stream to raw PCM
	// Output format: 16-bit signed little-endian, 44100 Hz, stereo
	// We add 1.5 seconds of silence at the start to compensate for player latency
	// (AirPlay buffers ~1250ms before playback starts)
	ffmpegArgs := []string{
		"-hide_banner",
		"-loglevel", "warning",
		"-f", "lavfi", "-i", "anullsrc=r=44100:cl=stereo:d=1.5", // 1.5s silence
		"-reconnect", "1",
		"-reconnect_streamed", "1",
		"-reconnect_delay_max", "5",
		"-i", uri,
		"-filter_complex", "[0][1]concat=n=2:v=0:a=1", // Concat silence + audio
		"-f", "s16le", // Raw PCM, 16-bit signed little-endian
		"-ar", "44100", // Sample rate: 44.1 kHz
		"-ac", "2", // Channels: stereo
		"-acodec", "pcm_s16le",
		"-", // Output to stdout
	}

	p.ffmpeg = exec.CommandContext(playCtx, "ffmpeg", ffmpegArgs...)
	ffmpegOut, err := p.ffmpeg.StdoutPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("failed to get ffmpeg stdout: %w", err)
	}

	// Capture stderr for logging - must be set up before Start()
	ffmpegStderr, err := p.ffmpeg.StderrPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("failed to get ffmpeg stderr: %w", err)
	}

	if err := p.ffmpeg.Start(); err != nil {
		cancel()
		return fmt.Errorf("failed to start ffmpeg: %w", err)
	}
	log.Printf("Started ffmpeg: ffmpeg %v", ffmpegArgs)

	// Log FFmpeg stderr in background
	go func() {
		buf := make([]byte, 256)
		for {
			n, err := ffmpegStderr.Read(buf)
			if n > 0 {
				log.Printf("[ffmpeg] %s", string(buf[:n]))
			}
			if err != nil {
				break
			}
		}
	}()

	// Start streaming in a goroutine
	go func() {
		// Start the RAOP streamer - this starts cliraop and pipes ffmpegOut to it
		if err := p.streamer.Start(playCtx, ffmpegOut); err != nil {
			if err != context.Canceled && err != io.EOF {
				log.Printf("RAOP streaming error: %v", err)
			}
		}

		// Wait for ffmpeg to finish (it will exit when input ends or context cancelled)
		_ = p.ffmpeg.Wait()
		log.Printf("FFmpeg finished, waiting for cliraop to drain buffer...")

		// Wait for cliraop to finish playing buffered audio
		// This gives it time to play out remaining samples
		_ = p.streamer.WaitForCompletion()
		log.Printf("Playback complete")

		// Now clean up
		p.mu.Lock()
		p.cancel = nil
		p.ffmpeg = nil
		p.streamer = nil
		p.mu.Unlock()
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
	p.stopInternal()
	return nil
}

// stopInternal stops all resources without locking.
func (p *RAOPPlayer) stopInternal() {
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}

	if p.ffmpeg != nil && p.ffmpeg.Process != nil {
		_ = p.ffmpeg.Process.Kill()
		_ = p.ffmpeg.Wait()
		p.ffmpeg = nil
	}

	if p.streamer != nil {
		_ = p.streamer.Stop()
		p.streamer = nil
	}
}

// SetVolume adjusts the volume.
// Note: RAOP volume is set at connection time. Changes take effect on next Play.
// If volume passthrough is configured, it will also call the external API.
func (p *RAOPPlayer) SetVolume(ctx context.Context, volume int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	log.Printf("RAOP SetVolume: %d", volume)
	p.volume = volume

	// Call volume passthrough API if configured
	if p.volumePassthrough != nil && p.volumePassthrough.URL != "" {
		if err := p.callVolumePassthrough(ctx, volume); err != nil {
			log.Printf("Volume passthrough error: %v", err)
			// Don't return error - passthrough is best-effort
		}
	}

	return nil
}

// callVolumePassthrough makes the HTTP request to the external volume API.
func (p *RAOPPlayer) callVolumePassthrough(ctx context.Context, volume int) error {
	vp := p.volumePassthrough
	volumeStr := fmt.Sprintf("%d", volume)

	// Replace {{volume}} placeholder in URL and body
	url := strings.ReplaceAll(vp.URL, "{{volume}}", volumeStr)
	body := strings.ReplaceAll(vp.Body, "{{volume}}", volumeStr)

	method := vp.Method
	if method == "" {
		method = "PUT"
	}

	var bodyReader io.Reader
	if body != "" {
		bodyReader = bytes.NewBufferString(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	// Set content type for JSON body
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	// Set basic auth if configured
	if vp.AuthUser != "" {
		req.SetBasicAuth(vp.AuthUser, vp.AuthPass)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned %d: %s", resp.StatusCode, string(respBody))
	}

	log.Printf("Volume passthrough: %s %s -> %d", method, url, resp.StatusCode)
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
