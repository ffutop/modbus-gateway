# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.5.0] - 2026-09-19

### Added

- 共享仿真模型：通过 `version: 1` 在顶层 `simulations` 声明具名模型，使用 `simulation.ref` 在多个 `local`、`injector` 下游间共享数据与持久化，支持跨网关引用。
- 注入下游（Injector）：将标准 Modbus 写请求（FC05/FC06/FC15/FC16）从线圈映射到离散输入，或从保持寄存器映射到输入寄存器，不修改源表。拒绝读取、未映射地址和跨映射边界的写入。
- 配置校验：拒绝 v1 未知字段、无效模型引用、非法或重叠映射范围，以及重复的上游监听地址/设备路径字符串。每个 v1 `local`/`injector` 入口必须恰好解析为一个 1–247 的 Slave ID；目标映射重叠检查覆盖引用同一模型的全部注入入口。
- 仿真写入审计与持久化健康状态：记录写入来源、目标范围、提交版本及状态。运行中持久化失败会将模型标记为降级，已成功写入的内存数据仍可访问，并返回正常 Modbus 响应。
- 新增共享仿真、注入映射、配置校验、持久化及旧配置兼容性的单元测试与集成测试覆盖。

### Changed

- 配置版本化：继续接受未声明 `version` 或 `version: 0` 的旧配置，每个旧版 `local` 下游保持独立模型。v1 将持久化从下游 `local` 块移至顶层仿真配置；旧配置中出现 v1 专属字段时会被拒绝。
- 仿真启动流程：启动网关前统一打开并恢复所有已配置模型的存储；任一模型存储打开失败，现在会导致整个进程退出。
- 中英文 README：优先推荐 macOS、Linux、Windows 的 amd64/arm64 预构建 Releases，提供可独立使用的安装、配置、验证、迁移、运维、排障和开发说明。

### Fixed

- Docker 启动：将不支持的 `-c` 参数改为 `-config /etc/modbusgw/config.yaml`，分离可执行文件 `ENTRYPOINT` 与可覆盖的默认 `CMD` 参数。
- 默认 mmap 配置：将目录路径 `/data/` 改为具体文件 `/data/modbus-slave.bin`，并注明需创建父目录及授予写权限。

## [0.4.0] - 2026-05-13

### Added

- pprof 性能剖析端点：集成了 Go 内置的 `net/http/pprof` 处理器，暴露独立的 HTTP 服务用于运行时性能分析。支持通过标准 `go tool pprof` 工具链进行 CPU 剖析、内存堆分析、Goroutine 检查等诊断操作。

## [0.3.0] - 2026-01-15

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

[0.5.0]: https://github.com/ffutop/modbus-gateway/compare/v0.4.0...v0.5.0
