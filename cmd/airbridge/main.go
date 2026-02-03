package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"math"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kenyonj/airbridge/internal/bridge"
	"github.com/kenyonj/airbridge/internal/database"
	"github.com/kenyonj/airbridge/internal/discovery"
	"github.com/kenyonj/airbridge/internal/httpserver"
	"github.com/kenyonj/airbridge/internal/player"
	"github.com/kenyonj/airbridge/internal/renderer"
	"github.com/kenyonj/airbridge/internal/ssdp"
	"github.com/kenyonj/airbridge/internal/state"
	"github.com/kenyonj/airbridge/internal/upnp"
	"github.com/kenyonj/airbridge/internal/web"
	"github.com/kenyonj/airbridge/pkg/config"
	"github.com/kenyonj/airbridge/pkg/raop"
)

// generateDeterministicUUID creates a stable UUID from a device identifier.
// This ensures the same device always gets the same UUID across restarts.
func generateDeterministicUUID(identifier string) string {
	hash := sha256.Sum256([]byte("airbridge:" + identifier))
	// Format as UUID: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		hash[0:4], hash[4:6], hash[6:8], hash[8:10], hash[10:16])
}

func main() {
	// Parse flags
	version := flag.Bool("version", false, "Print version")
	testStream := flag.String("test", "", "Test streaming to device (by name or IP:port)")
	serve := flag.Bool("serve", false, "Run as DLNA renderer server (single device)")
	serveAll := flag.Bool("serve-all", false, "Run as DLNA renderer server (all devices)")
	webMode := flag.Bool("web", false, "Run with web admin interface")
	target := flag.String("target", "", "Target AirPlay device name (for serve mode)")
	port := flag.Int("port", 8200, "HTTP port for DLNA server")
	dbPath := flag.String("db", "./airbridge.db", "Path to SQLite database")
	configPath := flag.String("config", "", "Path to config file")
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

	// Multi-device server mode
	if *serveAll {
		runMultiDeviceServer(ctx, *configPath, *port)
		return
	}

	// Web admin mode
	if *webMode {
		runWebServer(ctx, *dbPath, *port)
		return
	}

	// Single device server mode
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
	// Use deterministic UUID based on device ID so it persists across restarts
	var deviceUUID string
	if device != nil {
		deviceUUID = generateDeterministicUUID(device.DeviceID)
	} else {
		deviceUUID = generateDeterministicUUID("default")
	}
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
	em := upnp.NewEventManager()
	mux := http.NewServeMux()
	httpserver.RegisterHTTP(mux, baseURL, deviceUUID, friendlyName, "Airbridge", st, audioPlayer, em)

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

// runMultiDeviceServer starts renderers for all discovered AirPlay devices.
func runMultiDeviceServer(ctx context.Context, configPath string, basePort int) {
	fmt.Println("Starting multi-device DLNA renderer server...")

	// Load config
	if configPath == "" {
		configPath = config.FindConfigFile()
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not load config: %v\n", err)
		cfg = config.DefaultConfig()
	}

	if basePort > 0 {
		cfg.HTTPPort = basePort
	}

	if configPath != "" {
		fmt.Printf("Loaded config from: %s\n", configPath)
	}

	// Start device discovery
	disco := discovery.NewService()
	if err := disco.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start discovery: %v\n", err)
		os.Exit(1)
	}
	defer disco.Stop()

	// Create renderer manager
	mgr := renderer.NewManager(cfg)
	if err := mgr.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start renderer manager: %v\n", err)
		os.Exit(1)
	}
	defer mgr.Stop()

	fmt.Println("Waiting for AirPlay device discovery...")
	fmt.Println()

	// Periodically update devices
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// Initial discovery wait
	time.Sleep(5 * time.Second)
	mgr.UpdateDevices(disco.GetDevices())

	// Print status
	printStatus(mgr)

	for {
		select {
		case <-ctx.Done():
			fmt.Println("Goodbye!")
			return

		case <-ticker.C:
			mgr.UpdateDevices(disco.GetDevices())
		}
	}
}

func printStatus(mgr *renderer.Manager) {
	instances := mgr.GetInstances()
	if len(instances) == 0 {
		fmt.Println("No devices found. Waiting for discovery...")
		return
	}

	fmt.Printf("=== Active Renderers (%d) ===\n", len(instances))
	for _, inst := range instances {
		fmt.Printf("  • %s\n", inst.FriendlyName)
		fmt.Printf("    Port: %d | Target: %s:%d\n",
			inst.Port, inst.Device.Host, inst.Device.Port)
	}
	fmt.Println()
	fmt.Println("Cast audio to any of these devices from Music Assistant or other DLNA controllers.")
	fmt.Println("Press Ctrl+C to stop.")
	fmt.Println()
}

// runWebServer starts the web admin interface.
func runWebServer(ctx context.Context, dbPath string, httpPort int) {
	fmt.Printf("Starting Airbridge with web admin on port %d\n", httpPort)
	fmt.Printf("Database: %s\n\n", dbPath)

	// Open database
	db, err := database.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// Start device discovery
	disco := discovery.NewService()
	if err := disco.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start discovery: %v\n", err)
		os.Exit(1)
	}
	defer disco.Stop()

	// Wait for initial discovery
	fmt.Println("Discovering AirPlay devices...")
	time.Sleep(3 * time.Second)

	// Create and start unified bridge (DLNA server with embedded devices)
	br := bridge.NewBridge(db, disco, httpPort+1)
	if err := br.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start bridge: %v\n", err)
		os.Exit(1)
	}
	defer br.Stop()

	// Create web server
	webServer, err := web.NewServer(db, disco, br, httpPort+1)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create web server: %v\n", err)
		os.Exit(1)
	}

	// Setup HTTP server for web admin
	mux := http.NewServeMux()
	webServer.RegisterRoutes(mux)

	// Root redirect to admin
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/admin", http.StatusFound)
			return
		}
		http.NotFound(w, r)
	})

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", httpPort),
		Handler: httpserver.LogMiddleware(mux),
	}

	go func() {
		fmt.Printf("\n🌐 Web admin: http://localhost:%d/admin\n", httpPort)
		fmt.Printf("📡 DLNA bridge: http://localhost:%d/device.xml\n", httpPort+1)
		fmt.Println("Press Ctrl+C to stop.\n")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "HTTP server error: %v\n", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(shutdownCtx)
}
