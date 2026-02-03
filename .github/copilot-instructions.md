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

### Version Management
Version is tracked in two places that must stay in sync:
- `cmd/airbridge/main.go:77` - `fmt.Println("Version: X.X.X")`
- `airbridge/config.yaml:2` - `version: "X.X.X"`

Use `./scripts/release.sh bump [major|minor|patch]` to update versions, commit, tag, and push.

### Error Handling Style
Use `_, _ =` pattern for intentionally ignored error returns (e.g., `_, _ = w.Write(...)`) to satisfy golangci-lint.

## Testing

### Test Patterns
- Table-driven tests throughout
- `httptest` package for HTTP handler tests
- In-memory SQLite (`:memory:`) for database tests
- Use `zeroconf.NewServiceEntry()` constructor for mDNS test fixtures (not struct literals)

### Running Tests
```bash
go test -race ./...           # All tests with race detection
go test -v ./internal/upnp    # Single package verbose
make test                     # Via Makefile
```

## CI/CD

### GitHub Actions Workflows
- **test.yml** - Runs on push to main and PRs; tests Go 1.22/1.23/1.24 + golangci-lint
- **docker.yml** - Builds Docker images; triggers only on version tags (`v*`)
- **ha-addon.yml** - Builds Home Assistant addon; triggers only on version tags and releases

## Known Limitations

### Chromecast Receiver (Issue #1)
Implementing Chromecast receiver support is blocked by Google's device authentication requirements. CASTV2 protocol requires signed certificates from real Chromecast hardware. Consider Chromecast as *output target* (issue #2) instead, which uses client-side casting without auth issues.
