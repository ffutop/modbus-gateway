# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.6.2] - 2026-10-06

### Added

- Native desktop app (`desktop-native/`, Gio) with link topology, request/register monitoring, visual/YAML configuration editing and runtime logs. The gateway runs as a sidecar child of the same executable; closing the window stops it gracefully.
- Configuration workbench with searchable overviews, shared-model reference migration, advanced parameters, atomic bulk edits/copies/deletes and downstream moves, undo/redo, private draft recovery and external-file conflict handling. Draft, saved and running configurations are tracked independently; save-and-apply and recovery require restart confirmation, and the running baseline updates only after every listener succeeds. V0 configurations retain raw-text editing and CLI compatibility.
- Native configuration file open/save-as dialogs and keyboard shortcuts. Without `-config`, the app opens `config.yaml` beside the executable (beside `ModMux.app` on macOS). A missing file opens a blank v1 draft with the gateway stopped; first save refuses to overwrite a file created meanwhile. Relative persistence paths resolve beside the configuration file.
- Runtime log view with desktop/gateway records, run sessions, level/source/gateway/text filters, pause/follow controls, details, clipboard copy and confirmed snapshot export. Bounded log history remains available across gateway restarts and management connection loss.
- Read-only management API for status, request events and simulation registers, with token authentication in sidecar mode and loopback Host checks. Events include request/response PDUs in hex; status reports each upstream listener's startup, listening, failure and stopped states.
- Desktop release packages for macOS, Windows and Linux on amd64 and arm64, with embedded release versions, ModMux icons and bundled font licenses. Windows includes icon/version resources; Linux provides a user desktop-entry installer. Desktop logs use platform user log directories.
- CI validation for core and native desktop modules, commit-message checks, and an updated project website with a desktop application page.

### Changed

- Require Go 1.24.3 across all three modules, CI and Docker builds; the root gateway previously required Go 1.21.
- Move Modbus and transport packages under `internal/`; their previous import paths are no longer available to external consumers. Consolidate shared simulation models and persistence under `internal/simulation`, with local/injector protocol adapters under `internal/transport`.
- Move runtime assembly from `main.go` to `internal/app` and the command to `internal/cli`, shared by the CLI and desktop sidecar. Existing CLI startup and routing behavior remain compatible; the management listener is disabled by default.
- Use generated desktop design tokens, bundled Noto Sans SC UI fonts and Go Mono for values, grouped navigation, overview tables, two-column forms, native dropdowns and modal destructive-action confirmations.
- Keep telemetry reads and desktop request-history projections bounded and cached to avoid copying the complete history each frame.
- Move the Dockerfile to the repository root and restrict its build context to gateway sources.

### Fixed

- Close active management event streams promptly during shutdown.
- Ignore SIGPIPE in `-exit-on-stdin-eof` mode so parent termination or closed output pipes do not interrupt graceful shutdown and persistence flushing.
- Stream gateway logs to the desktop even when a log file is configured, without changing default CLI output. Report management connection loss and failed configuration application while retaining drafts and the last healthy session configuration.

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

[0.6.2]: https://github.com/ffutop/modbus-gateway/compare/v0.5.0...v0.6.2
