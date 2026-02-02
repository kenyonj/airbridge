package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"math"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/kenyonj/airbridge/internal/discovery"
	"github.com/kenyonj/airbridge/internal/httpserver"
	"github.com/kenyonj/airbridge/internal/player"
	"github.com/kenyonj/airbridge/internal/ssdp"
	"github.com/kenyonj/airbridge/internal/state"
	"github.com/kenyonj/airbridge/pkg/raop"
)

func main() {
	// Parse flags
	version := flag.Bool("version", false, "Print version")
	testStream := flag.String("test", "", "Test streaming to device (by name or IP:port)")
	serve := flag.Bool("serve", false, "Run as DLNA renderer server")
	target := flag.String("target", "", "Target AirPlay device name (for serve mode)")
	port := flag.Int("port", 8200, "HTTP port for DLNA server")
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

	// DLNA server mode
	if *serve {
		runDLNAServer(ctx, *target, *port)
		return
	}

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

// runDLNAServer starts the DLNA renderer server.
func runDLNAServer(ctx context.Context, targetDevice string, httpPort int) {
	fmt.Printf("Starting DLNA renderer server on port %d\n", httpPort)

	// Start device discovery
	disco := discovery.NewService()
	if err := disco.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start discovery: %v\n", err)
		os.Exit(1)
	}
	defer disco.Stop()

	// Wait for discovery
	fmt.Println("Waiting for AirPlay device discovery...")
	time.Sleep(5 * time.Second)

	// Find the target device
	var device *discovery.AirPlayDevice
	if targetDevice != "" {
		device = disco.GetDeviceByName(targetDevice)
		if device == nil {
			fmt.Fprintf(os.Stderr, "Target device not found: %s\n", targetDevice)
			fmt.Println("Available devices:")
			for _, d := range disco.GetDevices() {
				fmt.Printf("  - %s (%s:%d)\n", d.Name, d.Host, d.Port)
			}
			os.Exit(1)
		}
		fmt.Printf("Target device: %s at %s:%d\n", device.Name, device.Host, device.Port)
	}

	// Get local IP
	ip := getLocalIP()
	if ip == "" {
		fmt.Fprintln(os.Stderr, "Could not determine local IP address")
		os.Exit(1)
	}

	baseURL := fmt.Sprintf("http://%s:%d", ip, httpPort)
	deviceUUID := uuid.New().String()
	serverName := "Airbridge/1.0"
	friendlyName := "Airbridge"
	if device != nil {
		friendlyName = fmt.Sprintf("Airbridge (%s)", device.Name)
	}

	fmt.Printf("Device UUID: %s\n", deviceUUID)
	fmt.Printf("Base URL: %s\n", baseURL)
	fmt.Printf("Friendly Name: %s\n", friendlyName)
	fmt.Println()

	// Create state
	st := state.New(ctx)
	defer st.Stop()

	// Create player
	var audioPlayer interface {
		Play(ctx context.Context, uri string, volume int) error
		Pause(ctx context.Context) error
		Stop(ctx context.Context) error
		SetVolume(ctx context.Context, volume int) error
	}

	if device != nil {
		audioPlayer = player.NewRAOPPlayer(device)
	} else {
		fmt.Println("No target device specified, using NullPlayer")
		audioPlayer = player.NullPlayer{}
	}

	// Setup HTTP server
	mux := http.NewServeMux()
	httpserver.RegisterHTTP(mux, baseURL, deviceUUID, friendlyName, "Airbridge", st, audioPlayer)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", httpPort),
		Handler: httpserver.LogMiddleware(mux),
	}

	// Start SSDP
	go ssdp.Announce(ctx, baseURL, "uuid:"+deviceUUID, serverName)
	go ssdp.SearchResponder(ctx, baseURL, "uuid:"+deviceUUID, serverName)

	// Start HTTP server
	go func() {
		fmt.Printf("HTTP server listening on port %d\n", httpPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "HTTP server error: %v\n", err)
		}
	}()

	fmt.Println()
	fmt.Println("DLNA renderer is running!")
	fmt.Println("Cast audio to this device from any DLNA/UPnP controller.")
	fmt.Println("Press Ctrl+C to stop.")
	fmt.Println()

	// Wait for shutdown
	<-ctx.Done()
	srv.Shutdown(context.Background())
	fmt.Println("Goodbye!")
}

// getLocalIP returns the preferred local IP address.
func getLocalIP() string {
	// Try to find a local IP by connecting to a remote address
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}
