package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"math"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kenyonj/airbridge/internal/discovery"
	"github.com/kenyonj/airbridge/pkg/raop"
)

func main() {
	// Parse flags
	version := flag.Bool("version", false, "Print version")
	testStream := flag.String("test", "", "Test streaming to device (by name or IP:port)")
	flag.Parse()

	fmt.Println("Airbridge - DLNA to AirPlay Bridge")
	fmt.Println("===================================")
	fmt.Println()

	if *version {
		fmt.Println("Version: 0.1.0-dev")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nShutting down...")
		cancel()
	}()

	// Start discovery
	disco := discovery.NewService()
	if err := disco.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start discovery: %v\n", err)
		os.Exit(1)
	}
	defer disco.Stop()

	// If test mode, wait for discovery then stream
	if *testStream != "" {
		fmt.Printf("Testing stream to: %s\n", *testStream)
		fmt.Println("Waiting for device discovery...")
		time.Sleep(6 * time.Second) // Wait for discovery

		// Find the device
		device := disco.GetDeviceByName(*testStream)
		if device == nil {
			// Try all devices for partial match
			for _, d := range disco.GetDevices() {
				if d.Name == *testStream || d.Host == *testStream {
					device = d
					break
				}
			}
		}

		if device == nil {
			fmt.Fprintf(os.Stderr, "Device not found: %s\n", *testStream)
			fmt.Println("Available devices:")
			for _, d := range disco.GetDevices() {
				fmt.Printf("  - %s (%s:%d)\n", d.Name, d.Host, d.Port)
			}
			os.Exit(1)
		}

		fmt.Printf("Found device: %s at %s:%d\n", device.Name, device.Host, device.Port)

		// Create streamer
		streamer, err := raop.NewStreamer(raop.StreamConfig{
			Host:           device.Host,
			Port:           device.Port,
			Volume:         80,
			EncryptionType: device.EncryptionTypes(),
			UseALAC:        false,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create streamer: %v\n", err)
			os.Exit(1)
		}

		// Generate test tone
		fmt.Println("Streaming 5 seconds of 440Hz test tone...")
		audio := generateTone(440, 5)

		if err := streamer.Start(ctx, bytes.NewReader(audio)); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to start stream: %v\n", err)
			os.Exit(1)
		}

		// Wait for stream to complete or interrupt
		select {
		case <-ctx.Done():
		case <-time.After(10 * time.Second):
		}

		streamer.Stop()
		fmt.Println("Test complete!")
		return
	}

	// Normal discovery mode
	fmt.Println("Starting AirPlay device discovery...")
	fmt.Println("Discovering AirPlay devices (Ctrl+C to stop)...")
	fmt.Println()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			devices := disco.GetDevices()
			if len(devices) == 0 {
				fmt.Println("No devices found yet...")
			} else {
				fmt.Printf("\n=== Found %d AirPlay devices ===\n", len(devices))
				for _, d := range devices {
					fmt.Printf("  - %s\n", d)
					fmt.Printf("      Encryption: et=%s, ALAC: %v\n", d.EncryptionTypes(), d.SupportsALAC())
				}
			}
		}
	}
}

// generateTone creates a 440Hz sine wave in 16-bit stereo PCM format.
func generateTone(frequency float64, durationSec int) []byte {
	sampleRate := 44100
	samples := sampleRate * durationSec
	amplitude := 0.5

	buf := make([]byte, samples*4) // 2 bytes per sample, 2 channels
	for i := 0; i < samples; i++ {
		t := float64(i) / float64(sampleRate)
		value := int16(amplitude * 32767 * math.Sin(2*math.Pi*frequency*t))
		
		// Little-endian 16-bit stereo
		offset := i * 4
		buf[offset] = byte(value)
		buf[offset+1] = byte(value >> 8)
		buf[offset+2] = byte(value)
		buf[offset+3] = byte(value >> 8)
	}
	return buf
}
