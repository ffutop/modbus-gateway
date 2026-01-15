# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
