# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added


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
