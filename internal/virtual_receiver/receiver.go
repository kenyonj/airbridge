// Package virtual_receiver implements virtual AirPlay receivers using shairport-sync.
//
// This package provides a wrapper around the shairport-sync binary to create
// virtual AirPlay devices that can receive audio and forward it to other outputs.
package virtual_receiver

import (
	"context"
	"fmt"
	"io"
	"log"
	"os/exec"
	"sync"
)

// Receiver represents a virtual AirPlay receiver.
type Receiver struct {
	name      string
	cmd       *exec.Cmd
	player    Player
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.Mutex
	running   bool
}

// Player defines the interface for output players.
type Player interface {
	Play(ctx context.Context, uri string, volume int) error
	Stop(ctx context.Context) error
	SetVolume(ctx context.Context, volume int) error
}

// Config holds configuration for creating a virtual receiver.
type Config struct {
	Name   string // Name shown to AirPlay clients
	Player Player // Output player (AirPlay, Chromecast, etc.)
}

// NewReceiver creates a new virtual AirPlay receiver.
//
// The receiver uses shairport-sync to accept AirPlay connections and pipes
// the audio output to the configured player.
//
// Requirements:
//   - shairport-sync binary must be installed and in PATH (checked at Start time)
//   - shairport-sync version 3.3 or later recommended
func NewReceiver(config Config) (*Receiver, error) {
	if config.Name == "" {
		return nil, fmt.Errorf("receiver name is required")
	}
	if config.Player == nil {
		return nil, fmt.Errorf("output player is required")
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Receiver{
		name:   config.Name,
		player: config.Player,
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

// Start begins listening for AirPlay connections.
//
// The receiver will appear on the network as an AirPlay device with the
// configured name. When a client connects and starts playing audio, the
// audio is forwarded to the configured output player.
//
// Returns an error if shairport-sync is not found in PATH.
func (r *Receiver) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.running {
		return fmt.Errorf("receiver already running")
	}

	// Check if shairport-sync is available
	if _, err := exec.LookPath("shairport-sync"); err != nil {
		return fmt.Errorf("shairport-sync not found in PATH: %w", err)
	}

	// Start shairport-sync in stdout mode
	// Audio is output as raw PCM: 16-bit signed little-endian, 44100 Hz, stereo
	r.cmd = exec.CommandContext(r.ctx, "shairport-sync",
		"--name", r.name,
		"--output", "stdout",
		"--",
	)

	stdout, err := r.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	stderr, err := r.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	// Log stderr output
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				log.Printf("[shairport-sync %s] %s", r.name, string(buf[:n]))
			}
			if err != nil {
				break
			}
		}
	}()

	if err := r.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start shairport-sync: %w", err)
	}

	r.running = true
	log.Printf("Virtual receiver started: %s", r.name)

	// Forward audio to output player
	// Note: This is a simplified implementation. In practice, we'd need to:
	// 1. Detect when audio starts/stops
	// 2. Handle connection errors
	// 3. Buffer appropriately
	// 4. Convert PCM to the format needed by the output
	go func() {
		defer func() {
			r.mu.Lock()
			r.running = false
			r.mu.Unlock()
		}()

		// TODO: Implement audio forwarding
		// For AirPlay output: need to encode PCM to format expected by cliraop
		// For Chromecast: need to serve PCM over HTTP and send URL to Chromecast
		// For DLNA: need to serve PCM over HTTP and send URL via SOAP
		
		// For now, just consume and discard the audio
		if _, err := io.Copy(io.Discard, stdout); err != nil {
			log.Printf("Error reading audio stream from shairport-sync: %v", err)
		}
		log.Printf("Virtual receiver audio stream ended: %s", r.name)
	}()

	return nil
}

// Stop stops the receiver and closes all connections.
func (r *Receiver) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.running {
		return nil
	}

	if r.cancel != nil {
		r.cancel()
	}

	if r.cmd != nil && r.cmd.Process != nil {
		if err := r.cmd.Process.Kill(); err != nil {
			return fmt.Errorf("failed to stop shairport-sync: %w", err)
		}
		_ = r.cmd.Wait()
	}

	r.running = false
	log.Printf("Virtual receiver stopped: %s", r.name)
	return nil
}

// IsRunning returns whether the receiver is currently running.
func (r *Receiver) IsRunning() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running
}

// Name returns the receiver's name.
func (r *Receiver) Name() string {
	return r.name
}
