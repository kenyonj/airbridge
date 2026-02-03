# Airbridge

> Bridge DLNA/UPnP audio streams to AirPlay (RAOP) devices

Airbridge creates virtual DLNA renderers that forward audio to AirPlay speakers. This allows media servers like [Music Assistant](https://music-assistant.io/) to stream to AirPlay-only devices through DLNA.

## Why?

Some AirPlay devices (like [Juke Audio](https://jukeaudio.com/) multi-zone speakers) have compatibility issues with certain AirPlay implementations. Airbridge works around this by:

1. Presenting itself as standard DLNA/UPnP renderers
2. Receiving audio streams via HTTP
3. Forwarding to AirPlay devices using [libraop](https://github.com/philippe44/libraop)

## Features

- 🎵 Virtual DLNA renderer per AirPlay zone
- 📡 Automatic AirPlay device discovery (mDNS)
- ⚙️ YAML configuration for device filtering and aliases
- 🔊 Volume control support
- 🌐 Web admin interface for managing devices
- 🐳 Docker deployment with Unraid support

## Status

⚠️ **Early Development (v0.x)** - Core functionality works but expect breaking changes, bugs, and missing features. Not yet recommended for production use.

### What works:
- DLNA renderer creation and discovery
- Audio streaming to AirPlay devices
- Web admin interface
- Docker/Unraid/Home Assistant deployment

### Planned:
- Chromecast support ([#1](https://github.com/kenyonj/airbridge/issues/1), [#2](https://github.com/kenyonj/airbridge/issues/2))
- Spotify Connect support ([#3](https://github.com/kenyonj/airbridge/issues/3))
- Tests and CI ([#4](https://github.com/kenyonj/airbridge/issues/4))

## Quick Start

### Option 1: Docker (Recommended)

```bash
docker run -d --network=host -v airbridge-data:/data ghcr.io/kenyonj/airbridge:latest
```

Access the web admin at `http://localhost:8200/admin`

### Option 2: Build from Source

#### 1. Download cliraop binary

Get the pre-built `cliraop` binary for your platform from [libraop releases](https://github.com/philippe44/libraop/releases) and place it in the `bin/` directory:

```bash
mkdir -p bin
# Download appropriate binary for your platform
# Rename to: bin/cliraop-macos-arm64, bin/cliraop-linux-amd64, etc.
chmod +x bin/cliraop-*
```

#### 2. Build airbridge

```bash
make build
```

#### 3. Run

**Bridge all discovered AirPlay devices:**
```bash
./bin/airbridge --serve-all
```

**Bridge a single device:**
```bash
./bin/airbridge --serve --target "Guest Bedroom"
```

**Test streaming to a device:**
```bash
./bin/airbridge --test "Guest Bedroom"
```

## Usage

```
Usage of ./bin/airbridge:
  -config string
      Path to config file
  -db string
      Path to SQLite database (default "./airbridge.db")
  -port int
      HTTP port for DLNA server (default 8200)
  -serve
      Run as DLNA renderer server (single device)
  -serve-all
      Run as DLNA renderer server (all devices)
  -target string
      Target AirPlay device name (for serve mode)
  -test string
      Test streaming to device (by name)
  -version
      Print version
  -web
      Run with web admin interface
```

## Configuration

Create a `config.yaml` file (or place at `~/.config/airbridge/config.yaml`):

```yaml
# HTTP port for the first renderer (subsequent use incremental ports)
http_port: 8200

# Enable automatic discovery of AirPlay devices
auto_discover: true

# Prefix for DLNA friendly names
name_prefix: "Airbridge"

# Optional: Filter to only include specific devices
device_filter:
  - "Kitchen"
  - "Living Room"
  - "Guest Bedroom"

# Optional: Device-specific configuration
devices:
  - name: "Kitchen"
    alias: "Kitchen Bridge"
    port: 8201
  
  - name: "Office"
    enabled: false  # Exclude this device
```

## Architecture

```
┌─────────────┐     DLNA/UPnP     ┌─────────────────┐     RAOP      ┌──────────────┐
│ Media       │ ────────────────▶ │   Airbridge     │ ────────────▶ │ AirPlay      │
│ Server      │                   │                 │               │ Speakers     │
│ (MA, Plex)  │                   │ ┌─────────────┐ │               │              │
└─────────────┘                   │ │ Virtual     │ │               │ ┌──────────┐ │
                                  │ │ DLNA        │ │               │ │ Zone 1   │ │
                                  │ │ Renderers   │ │               │ └──────────┘ │
                                  │ └──────┬──────┘ │               │ ┌──────────┐ │
                                  │        │        │               │ │ Zone 2   │ │
                                  │ ┌──────▼──────┐ │               │ └──────────┘ │
                                  │ │ libraop     │ │               │      ⋮       │
                                  │ │ RAOP client │ │               └──────────────┘
                                  │ └─────────────┘ │
                                  └─────────────────┘
```

## How It Works

1. **Discovery**: Airbridge uses mDNS to discover AirPlay/RAOP devices on your network
2. **SSDP**: For each device, it advertises a virtual DLNA MediaRenderer
3. **SOAP**: When a media server (like Music Assistant) connects, it handles AVTransport and RenderingControl SOAP requests
4. **Streaming**: Audio is received via HTTP and piped to the `cliraop` binary
5. **RAOP**: The cliraop process streams audio to the AirPlay device using RAOP protocol

## Use with Music Assistant

1. Start airbridge: `./bin/airbridge --serve-all` or `./bin/airbridge --web`
2. In Music Assistant, go to Settings → Players
3. You should see "Airbridge (Device Name)" devices appear as DLNA players
4. Select a device and start playing music!

## Docker

### Run with Docker

```bash
# Run with web admin interface
docker run -d --network=host -v airbridge-data:/data ghcr.io/kenyonj/airbridge:latest

# Or run in serve-all mode (no web UI)
docker run -d --network=host ghcr.io/kenyonj/airbridge:latest --serve-all
```

**Note:** Host network mode is required for mDNS device discovery and SSDP announcements.

### Unraid

Airbridge is available in the Unraid Community Applications store. Search for "Airbridge" or manually install using the template at `unraid/airbridge.xml`.

### Home Assistant

Airbridge is available as a Home Assistant add-on:

1. Go to **Settings → Add-ons → Add-on Store**
2. Click the ⋮ menu → **Repositories**
3. Add: `https://github.com/kenyonj/airbridge`
4. Click **Close** and refresh the page
5. Find "Airbridge" in the add-on list and click **Install**
6. Start the add-on and open the **Web UI** to manage devices

### Build Docker Image Locally

```bash
make docker-build
make docker-run
```

## Development

### Prerequisites

- Go 1.22+
- Docker (for building libraop, optional)
- Make

### Building

```bash
make build      # Build airbridge
make clean      # Clean build artifacts
```

### Project Structure

```
.
├── cmd/airbridge/       # Main application entry point
├── internal/
│   ├── bridge/          # Unified DLNA bridge with embedded devices
│   ├── database/        # SQLite database for web mode
│   ├── discovery/       # mDNS-based AirPlay discovery
│   ├── httpserver/      # HTTP server for UPnP endpoints
│   ├── player/          # RAOP player implementation
│   ├── renderer/        # Multi-device renderer manager
│   ├── ssdp/            # SSDP announcement/discovery
│   ├── state/           # Playback state management
│   ├── upnp/            # UPnP/DLNA SOAP handlers
│   └── web/             # Web admin interface
├── pkg/
│   ├── config/          # YAML configuration
│   └── raop/            # cliraop subprocess wrapper
└── unraid/              # Unraid Community Apps template
```

## Tested With

- ✅ Juke Audio 8-zone multi-room speakers
- ✅ HomePod mini (as AirPlay target)
- ✅ Apple TV (as AirPlay target)
- ✅ Sonos (AirPlay mode)

## Troubleshooting

### Device not discovered
- Ensure your AirPlay device is on the same network
- Try: `dns-sd -B _raop._tcp` to verify mDNS discovery works
- Check firewall settings allow mDNS (port 5353 UDP)

### Audio doesn't play
- Verify cliraop binary is in `bin/` directory
- Test directly: `./bin/airbridge --test "Device Name"`
- Check logs for connection errors

### DLNA renderer not visible
- Ensure port 8200+ is not blocked by firewall
- Try: `curl http://localhost:8200/device.xml` to verify HTTP server

## License

MIT License - see [LICENSE](LICENSE)

## Support

If you find Airbridge useful, consider supporting development:

[![GitHub Sponsors](https://img.shields.io/github/sponsors/kenyonj?style=for-the-badge&logo=github&label=Sponsor)](https://github.com/sponsors/kenyonj)
[![Ko-fi](https://img.shields.io/badge/Ko--fi-Support-ff5e5b?style=for-the-badge&logo=ko-fi)](https://ko-fi.com/kenyonj)

## Credits

- [philippe44/libraop](https://github.com/philippe44/libraop) - RAOP client library
- [tr1v3r/rcast](https://github.com/tr1v3r/rcast) - DLNA renderer reference
- [grandcat/zeroconf](https://github.com/grandcat/zeroconf) - mDNS library
