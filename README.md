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
- 🔧 Web UI for target configuration
- 📡 Automatic AirPlay device discovery (mDNS)
- 🐳 Docker deployment ready
- 🏠 Home Assistant add-on compatible

## Status

⚠️ **Early Development** - Not yet functional

## Architecture

\`\`\`
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
                                  │                 │
                                  │ ┌─────────────┐ │
                                  │ │ Web UI      │ │
                                  │ │ Config      │ │
                                  │ └─────────────┘ │
                                  └─────────────────┘
\`\`\`

## Installation

_Coming soon_

## Configuration

_Coming soon_

## Development

### Prerequisites

- Go 1.22+
- Docker (for building libraop)
- Make

### Building

\`\`\`bash
make build
\`\`\`

### Running

\`\`\`bash
./airbridge --config config.yaml
\`\`\`

## License

MIT License - see [LICENSE](LICENSE)

## Credits

- [philippe44/libraop](https://github.com/philippe44/libraop) - RAOP client library
- [hzeller/gmrender-resurrect](https://github.com/hzeller/gmrender-resurrect) - DLNA renderer inspiration
