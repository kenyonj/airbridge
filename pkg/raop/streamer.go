// Package raop provides RAOP/AirPlay streaming via cliraop subprocess.
package raop

import (
	"context"
	"fmt"
	"io"
	"log"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
)

// StreamConfig holds configuration for a RAOP stream.
type StreamConfig struct {
	Host           string // Target host IP
	Port           int    // Target port
	Volume         int    // Volume 0-100
	EncryptionType string // Encryption type from TXT record (e.g., "0,4")
	UseALAC        bool   // Use ALAC compression
}

// Streamer manages streaming audio to a RAOP device.
type Streamer struct {
	config      StreamConfig
	cliraopPath string
	cmd         *exec.Cmd
	stdin       io.WriteCloser
	mu          sync.Mutex
	running     bool
}

// NewStreamer creates a new RAOP streamer.
func NewStreamer(config StreamConfig) (*Streamer, error) {
	// Find cliraop binary
	cliraopPath, err := findCliraop()
	if err != nil {
		return nil, err
	}

	return &Streamer{
		config:      config,
		cliraopPath: cliraopPath,
	}, nil
}

// findCliraop locates the cliraop binary.
func findCliraop() (string, error) {
	// Check common locations
	searchPaths := []string{
		"./bin/cliraop",
		"./cliraop",
		"/usr/local/bin/cliraop",
	}

	// Add platform-specific paths
	var suffix string
	switch runtime.GOOS {
	case "darwin":
		if runtime.GOARCH == "arm64" {
			suffix = "-macos-arm64"
		} else {
			suffix = "-macos-x86_64"
		}
	case "linux":
		if runtime.GOARCH == "arm64" {
			suffix = "-linux-aarch64"
		} else {
			suffix = "-linux-x86_64"
		}
	}

	if suffix != "" {
		searchPaths = append([]string{
			"./bin/cliraop" + suffix,
			"./vendor/libraop/bin/cliraop" + suffix,
		}, searchPaths...)
	}

	for _, path := range searchPaths {
		absPath, err := filepath.Abs(path)
		if err != nil {
			continue
		}
		if _, err := exec.LookPath(absPath); err == nil {
			return absPath, nil
		}
	}

	// Try PATH
	if path, err := exec.LookPath("cliraop"); err == nil {
		return path, nil
	}

	return "", fmt.Errorf("cliraop binary not found. Please install libraop or set CLIRAOP_PATH")
}

// Start begins streaming audio from the given reader.
func (s *Streamer) Start(ctx context.Context, audio io.Reader) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("stream already running")
	}

	// Build command arguments
	args := []string{
		"-p", fmt.Sprintf("%d", s.config.Port),
		"-v", fmt.Sprintf("%d", s.config.Volume),
		"-d", "2", // Debug level
	}

	// NOTE: We intentionally do NOT pass -t (encryption type) flag
	// because the macOS cliraop binary crashes with auth-setup on Apple Silicon.
	// Testing shows Juke devices accept connections without auth-setup.

	// Add ALAC flag if requested
	if s.config.UseALAC {
		args = append(args, "-a")
	}

	// Add host and stdin indicator
	args = append(args, s.config.Host, "-")

	log.Printf("Starting cliraop: %s %v", s.cliraopPath, args)

	s.cmd = exec.CommandContext(ctx, s.cliraopPath, args...)

	// Set up stdin pipe
	stdin, err := s.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}
	s.stdin = stdin

	// Capture stderr for logging
	s.cmd.Stderr = &logWriter{prefix: "cliraop"}

	// Start the command
	if err := s.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start cliraop: %w", err)
	}

	s.running = true

	// Pipe audio to cliraop
	go func() {
		defer stdin.Close()
		io.Copy(stdin, audio)
	}()

	// Wait for command to finish in background
	go func() {
		err := s.cmd.Wait()
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
		if err != nil {
			log.Printf("cliraop exited with error: %v", err)
		} else {
			log.Printf("cliraop exited normally")
		}
	}()

	return nil
}

// Stop terminates the stream.
func (s *Streamer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running || s.cmd == nil || s.cmd.Process == nil {
		return nil
	}

	// Close stdin to signal EOF
	if s.stdin != nil {
		s.stdin.Close()
	}

	// Kill the process
	return s.cmd.Process.Kill()
}

// SetVolume adjusts the volume (note: requires restart with cliraop).
func (s *Streamer) SetVolume(volume int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config.Volume = volume
}

// IsRunning returns whether the stream is active.
func (s *Streamer) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// logWriter writes output with a prefix.
type logWriter struct {
	prefix string
}

func (w *logWriter) Write(p []byte) (n int, err error) {
	log.Printf("[%s] %s", w.prefix, string(p))
	return len(p), nil
}
