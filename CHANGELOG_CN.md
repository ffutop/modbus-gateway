# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- 本地从站 (Local Slave)：在网关内部实现了一个功能完整的本地 Modbus 从站。这使得网关可以作为独立服务器运行或进行数据缓存。
- 持久化存储引擎：为本地从站添加了可插拔的持久化层，支持多种后端实现：
    - 内存模式 (Memory)：高速易失性存储。
    - 文件模式 (File)：基于标准文件系统的持久化。
    - 内存映射 (Mmap)：利用 `mmap-go` 实现的高性能、跨平台（Linux, macOS, Windows）内存映射文件存储。
- 性能基准测试：添加了基准测试套件，用于验证和对比内存、文件及 Mmap 存储后端的性能表现。
- Modbus RTU over TCP：实现了完整的 Modbus RTU over TCP 支持（透传模式），支持通过 TCP 网络传输 RTU 数据帧。

### Changed

- 高级路由功能：增强了配置能力，支持复杂的路由规则，允许同时连接外部物理从站和内部本地从站。
- TCP 客户端增强：升级了 Modbus TCP 和 RTU-over-TCP 客户端，采用带锁的持久连接机制，支持自动断线重连和线程安全的并发访问。

## [0.2.0] - 2026-01-12

### Added

- 多网关实例支持：单进程内支持运行多个独立的网关转换逻辑。允许通过配置文件定义多个网关规则（Gateways），实现同时管理多个物理串口或 TCP 端口的隔离与转发，支持构建“多主多从”的复杂网络拓扑。
- Modbus 异常处理：当下游从站超时或失败时，网关现在会向上游主站返回标准的 Modbus 异常 PDU（如 `GatewayTargetDeviceFailedToRespond` 0x0B），提高了协议兼容性和可诊断性。

### Changed

- 多主一从架构升级：增强了 Modbus-Gateway 的适配能力，在原有基础上新增支持 Modbus RTU 主站接入。目前已全面支持 TCP/RTU 主站与 TCP/RTU 从站的灵活组合。


## [0.1.0] - 2025-11-19

### Added

- Modbus-Gateway 核心功能实现：首次发布支持“多主一从”架构的网关软件。
    - 支持多个 Modbus TCP 主站同时访问。
    - 支持后端连接 Modbus TCP 或 Modbus RTU 协议的从站设备。

[0.2.0]: https://github.com/ffutop/modbus-gateway/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/ffutop/modbus-gateway/releases/tag/v0.1.0
