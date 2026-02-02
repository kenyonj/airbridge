package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kenyonj/airbridge/internal/discovery"
)

func main() {
	fmt.Println("Airbridge - DLNA to AirPlay Bridge")
	fmt.Println("===================================")
	fmt.Println()

	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println("Version: 0.1.0-dev")
		return
	}

	// Start discovery service
	fmt.Println("Starting AirPlay device discovery...")
	fmt.Println()

	disco := discovery.NewService()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := disco.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start discovery: %v\n", err)
		os.Exit(1)
	}

	// Handle signals for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Print discovered devices every 5 seconds
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	fmt.Println("Discovering AirPlay devices (Ctrl+C to stop)...")
	fmt.Println()

	for {
		select {
		case <-sigChan:
			fmt.Println("\nShutting down...")
			disco.Stop()
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
