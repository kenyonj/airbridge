# Virtual AirPlay Receiver

This package provides a proof-of-concept implementation of virtual AirPlay receivers that can accept AirPlay audio and forward it to other output devices.

## Status: Proof of Concept

This is a **proof-of-concept implementation** demonstrating the technical feasibility of virtual AirPlay receivers. It is **not yet fully functional** for production use.

## Architecture

The implementation uses [shairport-sync](https://github.com/mikebrady/shairport-sync) as a subprocess to handle the AirPlay/RAOP protocol:

```
┌─────────────┐
│AirPlay Client│ (iPhone, Mac, etc.)
└──────┬──────┘
       │ AirPlay/RAOP
       ▼
┌─────────────┐
│shairport-   │ (Subprocess)
│   sync      │
└──────┬──────┘
       │ PCM audio (stdout)
       ▼
┌─────────────┐
│  Receiver   │ (This package)
└──────┬──────┘
       │ Forward to...
       ▼
┌─────────────┐
│   Player    │ (AirPlay, Chromecast, DLNA)
└─────────────┘
```

## Requirements

- **shairport-sync** version 3.3 or later must be installed and in PATH
  - Linux: `sudo apt-get install shairport-sync`
  - macOS: `brew install shairport-sync`
  - Build from source: https://github.com/mikebrady/shairport-sync

## Usage Example

```go
package main

import (
    "context"
    "log"
    
    "github.com/kenyonj/airbridge/internal/virtual_receiver"
    "github.com/kenyonj/airbridge/internal/player"
)

func main() {
    // Create output player (AirPlay, Chromecast, etc.)
    outputPlayer := player.NewNullPlayer() // Example: no-op player
    
    // Create virtual receiver
    receiver, err := virtual_receiver.NewReceiver(virtual_receiver.Config{
        Name:   "Kitchen Bridge",
        Player: outputPlayer,
    })
    if err != nil {
        log.Fatal(err)
    }
    
    // Start receiving
    if err := receiver.Start(); err != nil {
        log.Fatal(err)
    }
    
    log.Printf("Virtual receiver '%s' is running", receiver.Name())
    
    // ... run until shutdown ...
    
    receiver.Stop()
}
```

## Current Limitations

1. **Audio forwarding not implemented**: Audio is currently discarded rather than forwarded to the output player
2. **No format conversion**: Would need to convert PCM to the format expected by different output types
3. **No connection management**: Doesn't detect when audio starts/stops
4. **No error recovery**: Subprocess errors aren't handled gracefully
5. **No configuration persistence**: Settings aren't saved to database

## Future Work

To make this production-ready, the following would be needed:

### Audio Forwarding Pipeline
- Detect when audio streaming starts/stops
- Buffer audio appropriately to handle network jitter
- Convert PCM to format needed by output:
  - **AirPlay output**: Encode PCM with FFmpeg and pipe to cliraop
  - **Chromecast**: Serve PCM over HTTP, send URL to Chromecast
  - **DLNA**: Serve PCM over HTTP, send URL via SOAP

### Integration with Airbridge
- Add web UI for creating/managing virtual receivers
- Store configurations in database
- Support multiple simultaneous virtual receivers
- Add device discovery integration

### Error Handling
- Graceful subprocess failure recovery
- Automatic restart on crashes
- Proper cleanup of resources
- Connection error handling

### Testing
- Integration tests with real shairport-sync
- End-to-end tests with audio verification
- Performance benchmarks

## Alternative Approaches

See [docs/RAOP_RECEIVER_INVESTIGATION.md](../../docs/RAOP_RECEIVER_INVESTIGATION.md) for detailed analysis of alternative approaches, including:

- Native Go implementation using go.raopd (has build issues)
- AirPlay 2 support using goplay2 (requires external dependencies)
- Comparison of wrapper vs. native approaches

## References

- [shairport-sync](https://github.com/mikebrady/shairport-sync) - Mature AirPlay receiver
- [Issue #5](https://github.com/kenyonj/airbridge/issues/5) - Virtual AirPlay devices feature request
- [RAOP Receiver Investigation](../../docs/RAOP_RECEIVER_INVESTIGATION.md) - Research findings
