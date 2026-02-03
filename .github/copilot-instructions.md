# Copilot Instructions for Airbridge

Airbridge bridges DLNA/UPnP audio streams to AirPlay (RAOP) devices. It creates virtual DLNA renderers that forward audio to AirPlay speakers.

## Build, Test, Lint

```bash
make build       # Build binary to bin/airbridge
make test        # Run all tests
go test ./internal/discovery -v  # Run tests for a single package
make lint        # Run golangci-lint (requires golangci-lint installed)
make fmt         # Format code with gofmt
```

## Architecture

### Data Flow
Media servers (Music Assistant, Plex) → DLNA/UPnP → Airbridge → RAOP → AirPlay speakers

### Key Components

- **`cmd/airbridge/main.go`** - Entry point with three modes: `--serve` (single device), `--serve-all` (multi-device), `--web` (admin UI)
- **`internal/bridge/`** - Unified DLNA bridge with embedded renderers for web mode; manages HTTP server with `/device.xml` root and `/renderer/{uuid}/` routes
- **`internal/renderer/`** - Manager for multi-device mode (`--serve-all`); creates one HTTP server per AirPlay device with deterministic ports/UUIDs
- **`internal/discovery/`** - mDNS service discovery for `_raop._tcp` (AirPlay devices)
- **`internal/upnp/`** - SOAP handlers for AVTransport, RenderingControl, ConnectionManager services
- **`internal/ssdp/`** - SSDP announce/search responder for DLNA device advertisement
- **`pkg/raop/`** - Subprocess wrapper for `cliraop` binary that handles RAOP streaming

### Two Rendering Architectures

1. **`--serve-all` mode** (`internal/renderer/Manager`): One HTTP server per device on different ports
2. **`--web` mode** (`internal/bridge/Bridge`): Single HTTP server with embedded devices using path-based routing (`/renderer/{uuid}/...`)

## Conventions

### UUIDs
Device UUIDs are deterministic (SHA256 of `airbridge:<deviceID>`) so DLNA controllers recognize the same device across restarts.

### cliraop Dependency
Audio streaming requires the external `cliraop` binary from [libraop](https://github.com/philippe44/libraop). Expected paths: `./bin/cliraop`, `./bin/cliraop-macos-arm64`, etc.

### Configuration
YAML config loaded from `--config` flag, `./config.yaml`, or `~/.config/airbridge/config.yaml`. See `config.example.yaml`.
