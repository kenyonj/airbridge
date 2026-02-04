// Package player provides the RAOP audio player implementation.
package player

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"sync"

	"github.com/kenyonj/airbridge/internal/discovery"
	"github.com/kenyonj/airbridge/pkg/raop"
)

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
}

// NewRAOPPlayer creates a new RAOP player for the given AirPlay device.
func NewRAOPPlayer(device *discovery.Device) *RAOPPlayer {
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
func (p *RAOPPlayer) SetVolume(ctx context.Context, volume int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	log.Printf("RAOP SetVolume: %d (will apply on next stream)", volume)
	p.volume = volume

	// TODO: cliraop doesn't support dynamic volume changes.
	// Would need to either:
	// 1. Fork cliraop to add 'v' command to interactive mode
	// 2. Build native Go RAOP client with SET_PARAMETER support

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
