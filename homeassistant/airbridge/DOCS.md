# Airbridge Home Assistant Add-on

Bridge DLNA/UPnP audio streams to AirPlay (RAOP) devices.

## About

Airbridge creates virtual DLNA renderers that forward audio to AirPlay speakers. This allows media servers like Music Assistant to stream to AirPlay-only devices through DLNA.

## Features

- Virtual DLNA renderer per AirPlay zone
- Automatic AirPlay device discovery (mDNS)
- Web admin interface for managing devices
- Volume control support

## Installation

1. Add this repository to your Home Assistant add-on store
2. Install the Airbridge add-on
3. Start the add-on
4. Open the Web UI to configure your devices

## Configuration

The add-on will automatically discover AirPlay devices on your network. Use the web interface to:

- View discovered AirPlay devices
- Enable/disable specific devices as DLNA renderers
- Configure device aliases

## Network Requirements

This add-on requires host network mode for:
- mDNS discovery of AirPlay devices (port 5353 UDP)
- SSDP announcements for DLNA renderers (port 1900 UDP)

## Support

For issues and feature requests, visit:
https://github.com/kenyonj/airbridge/issues
