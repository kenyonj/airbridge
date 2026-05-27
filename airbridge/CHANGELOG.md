# Changelog

## [0.2.3]

### Fixed
- Add-on now starts correctly on current Home Assistant OS versions. Added
  `init: false` to prevent the Supervisor from injecting Docker's tini init
  process as PID 1, which conflicted with s6-overlay from the base image and
  caused the "s6-overlay-suexec: fatal: can only run as pid 1" error on every
  startup.

## [1.0.0] - 2024

### Added
- Initial Home Assistant add-on release
- Web admin interface for device management
- Automatic AirPlay device discovery
- DLNA renderer bridge functionality
