# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Native desktop app preview (`desktop-native/`, Gio): a desktop menu, link topology, request/register monitoring and visual/YAML configuration editing. The gateway runs in a child process of the same executable (sidecar mode), so rendering never shares a runtime with forwarding and a crash on either side is reported rather than taking the other down; closing the window stops the gateway. Validated saves detect external changes and retain drafts on failure; the toolbar restarts the gateway, after a confirmation naming the interruption, to apply the saved configuration. The status bar shows whether the gateway is starting, running or stopped (with its last output lines) and each listener's state. A separate Go module needs Go 1.24+; the root module stays on Go 1.21.
- macOS application bundle packaging with a geometric ModMux icon, private first-launch configuration, user-directory logs and bundled Go/Noto Sans SC fonts.
- Telemetry events now carry the raw request and response PDUs.
- Management API: `/api/v1/events` includes each request's `request` and `response` PDUs in hex, and `/api/v1/status` lists every upstream listener's state (`starting`, `listening`, `failed` with its error, or `stopped`).

### Changed

- Gateway assembly moved from `main.go` to `internal/app`, and the command itself to `internal/cli`, so the desktop app runs exactly the CLI as its child process. Flags and startup behavior are unchanged.
- With `-exit-on-stdin-eof`, the gateway ignores SIGPIPE: when its parent exits and no one reads its output any more, it still completes the graceful shutdown and flushes persistence instead of being killed while logging.
- The native desktop app's colors, font sizes and radii are generated from `desktop-native/design/tokens.json` (see `desktop-native/design/README.md`). Status text uses darker green and red so badges and results meet WCAG AA contrast.

## [0.5.0] - 2026-09-19

### Added

- Shared simulations: Declare named models under top-level `simulations` with `version: 1`, and share their data and persistence across `local` and `injector` downstreams through `simulation.ref`, including across gateways.
- Injector downstreams: Map standard Modbus writes (FC05/FC06/FC15/FC16) from coils to discrete inputs or from holding registers to input registers without modifying the source tables. Reject reads, unmapped addresses, and writes crossing mapping boundaries.
- Configuration validation: Reject unknown v1 fields, invalid simulation references, invalid or overlapping mapping ranges, and duplicate upstream listen address/device strings. Each v1 `local`/`injector` entry must resolve to exactly one Slave ID in 1–247; target mapping overlaps are checked across all injectors sharing a model.
- Simulation write auditing and persistence health: Record write source, target range, commit version, and status. Runtime persistence failures mark the model as degraded while successful in-memory writes remain available and return normal Modbus responses.
- Unit and integration test coverage for shared simulations, injection mappings, configuration validation, persistence, and legacy compatibility.

### Changed

- Versioned configuration: Continue accepting omitted `version` or `version: 0` with an independent model per legacy `local` downstream. V1 moves persistence from the downstream `local` block to top-level simulations; v1-only fields are rejected in legacy configurations.
- Simulation startup: Open and restore all configured simulation stores before starting gateways. Failure to open any simulation's storage now aborts the entire process.
- English and Chinese READMEs: Prefer prebuilt Releases for macOS, Linux, and Windows on amd64/arm64, and provide self-contained installation, configuration, verification, migration, operations, troubleshooting, and development instructions.

### Fixed

- Docker startup: Replace the unsupported `-c` argument with `-config /etc/modbusgw/config.yaml`, separating the executable `ENTRYPOINT` from overridable default `CMD` arguments.
- Default mmap configuration: Use `/data/modbus-slave.bin` instead of the directory `/data/`, and document the need to create the parent directory and grant write access.

## [0.4.0] - 2026-05-13

### Added

- pprof Profiling Endpoint: Integrated Go's built-in `net/http/pprof` handler, exposing a dedicated HTTP server for runtime profiling. This enables CPU profiling, memory heap analysis, goroutine inspection, and other diagnostics via the standard `go tool pprof` toolchain.

## [0.3.0] - 2026-01-15

### Added

- Local Slave: Implemented a fully functional local Modbus slave that resides within the gateway. This enables the gateway to act as a standalone server or perform data caching.
- Persistence Storage Engine: Added a pluggable persistence layer for the local slave with multiple backend implementations:
    - Memory: High-speed volatile storage.
    - File: Standard OS file system persistence.
    - Mmap: High-performance, cross-platform (Linux, macOS, Windows) memory-mapped file storage using `mmap-go`.
- Performance Benchmarks: Added a benchmark suite to validate and compare the performance of Memory, File, and Mmap storage backends.
- Modbus RTU over TCP: Implemented fully functional RTU over TCP support, enabling transparent transmission of RTU frames via TCP networks.

### Changed

- Advanced Routing: Enhanced configuration capabilities to support complex routing rules, allowing simultaneous connections to external slaves and the internal local slave.
- TCP Client Enhancement: Upgraded Modbus TCP and RTU-over-TCP clients to use persistent connections with mutex locking, supporting automatic reconnection and thread-safe concurrent access.

## [0.2.0] - 2026-01-12

### Added

- Support for multiple gateway instances: Run multiple independent gateway conversion logics within a single process. Allows defining multiple gateway rules (Gateways) via configuration files to manage isolation and forwarding for multiple physical serial ports or TCP ports simultaneously, supporting the construction of complex "multi-master, multi-slave" network topologies.
- Modbus Exception Handling: The gateway now returns standard Modbus Exception PDU (e.g., `GatewayTargetDeviceFailedToRespond` 0x0B) to the upstream master when the downstream slave times out or fails, improving protocol compliance and diagnostics.

### Changed

- Multi-Master Architecture Upgrade: Enhanced Modbus-Gateway compatibility by adding support for Modbus RTU Master access. The gateway now fully supports flexible combinations of TCP/RTU Masters and TCP/RTU Slaves.


## [0.1.0] - 2025-11-19

### Added

- Modbus-Gateway Core Implementation: Initial release of the gateway software supporting a "Multi-Master, Single-Slave" architecture.
    - Enables simultaneous access for multiple Modbus TCP Masters.
    - Supports downstream connectivity to slave devices via Modbus TCP or Modbus RTU protocols.

### Changed

- Optimized SYSLOG connection capability to support using domain names.

[0.1.0]: https://github.com/ffutop/data-diode-connector/releases/tag/v0.1.0

[0.5.0]: https://github.com/ffutop/modbus-gateway/compare/v0.4.0...v0.5.0
