package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("Airbridge - DLNA to AirPlay Bridge")
	fmt.Println("===================================")
	fmt.Println()
	fmt.Println("Status: Under development")
	fmt.Println()
	fmt.Println("This application will:")
	fmt.Println("  1. Discover AirPlay devices on the network")
	fmt.Println("  2. Create virtual DLNA renderers for each configured target")
	fmt.Println("  3. Bridge audio streams from DLNA to AirPlay (RAOP)")
	fmt.Println()
	
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println("Version: 0.1.0-dev")
	}
}
