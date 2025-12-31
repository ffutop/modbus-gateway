# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added


### Changed

- 多主一从架构升级：增强了 Modbus-Gateway 的适配能力，在原有基础上新增支持 Modbus RTU 主站接入。目前已全面支持 TCP/RTU 主站与 TCP/RTU 从站的灵活组合。


## [0.1.0] - 2025-11-19

### Added

- Modbus-Gateway 核心功能实现：首次发布支持“多主一从”架构的网关软件。
    - 支持多个 Modbus TCP 主站同时访问。
    - 支持后端连接 Modbus TCP 或 Modbus RTU 协议的从站设备。

[0.1.0]: https://github.com/ffutop/data-diode-connector/releases/tag/v0.1.0
