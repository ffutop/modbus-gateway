# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.6.1] - 2026-10-06

### Added

- 原生桌面版新增运行日志页：汇集桌面／网关日志，支持运行会话、级别／来源／网关／关键词筛选、暂停与跟随、详情、复制、确认后导出快照及跨网关重启的有界历史缓存。子进程配置日志文件时仍向桌面发送日志，命令行版默认输出不变。
- 原生桌面版配置文件可选：未指定 `-config` 时打开应用所在目录（可执行文件旁，macOS 为 `ModMux.app` 旁）的 `config.yaml`；文件不存在时编辑器以空白 v1 草稿开始、网关不启动，首次保存才创建文件，且不覆盖期间被其他程序创建的同名文件。“打开…”（Ctrl/⌘+O）与“另存为…”（Ctrl/⌘+Shift+S）使用系统自带的文件选择器（macOS 为 NSOpenPanel／NSSavePanel，Windows 为通用文件对话框，Linux 为 XDG 桌面门户，其次 zenity 或 kdialog；均不可用时退回应用内对话框）选择 `.yaml`／`.yml` 文件并把编辑器切换到所选文件；运行中的网关在重启前保持原配置。应用及网关子进程的工作目录为配置文件所在目录，相对持久化路径相对配置文件解析。

### Changed

- 打包的桌面应用不再附带 `config.default.yaml`，首次启动也不再创建私有配置，改为按上文打开应用所在目录的 `config.yaml`；日志仍写入平台用户日志目录。

## [0.6.0] - 2026-10-05

### Added

- 原生删除、批量删除、网关重启与运行恢复改用模态确认弹窗；模型引用迁移在弹窗内完成，确认期间禁用背景操作。

- 原生配置工作台新增搜索总览与模型选择器、完整高级参数、原子批量修改／复制／删除及下游移动、多步撤销重做、私有草稿恢复、保存并应用确认，以及本次应用会话中最近成功配置的手动运行恢复。

- 原生可视化配置支持模拟模型、网关及上下游增减；模型改名与引用迁移原子联动，显式预览级联删除，保留 YAML 高级字段，支持多步撤销与重做。桌面完整性检查阻止保存不完整的 v1 网关，不改变命令行配置兼容性。
- 原生桌面版预览（`desktop-native/`，基于 Gio）：提供桌面菜单、链路拓扑、请求／寄存器监视与可视化／YAML 配置编辑。网关在同一可执行文件启动的子进程中运行（子进程模式），界面渲染不与转发共用运行时，任一侧崩溃都会被报告而不会拖垮另一侧；关闭窗口即停止网关。保存执行校验与外部修改冲突检测，失败保留草稿；工具栏可在确认中断影响后重启网关，使保存的配置生效。状态栏显示网关正在启动、运行中或已停止（附最后几行输出），以及各上游监听状态。它是独立的 Go 模块，与根模块和端到端测试模块统一要求 Go 1.24.3 及以上。
- macOS 应用包打包，包含几何风格 ModMux 图标、首次启动的私有配置、用户目录日志和打包的 Go／Noto Sans SC 字体。
- 桌面版发布覆盖 macOS、Windows、Linux 的 amd64 与 arm64：打标签发布时，`modmux-desktop-<os>-<arch>` 压缩包与命令行版一同上传，由 `desktop-native/scripts/` 下各平台的打包脚本构建并写入发布版本号。解压后的 Windows 与 Linux 包在首次启动时于平台用户目录创建配置与日志（`%APPDATA%`／`%LOCALAPPDATA%`，或 XDG 配置与状态目录）。Windows 可执行文件内嵌 ModMux 图标与版本信息；Linux 包附带 `install.sh`，为当前用户安装桌面入口与图标，应用菜单与 Dock 显示 ModMux 图标。
- 遥测事件新增原始请求与响应 PDU。
- 管理 API：`/api/v1/events` 以十六进制提供每个请求的 `request` 与 `response` PDU；`/api/v1/status` 列出每个上游监听的状态（`starting`、`listening`、附错误的 `failed` 或 `stopped`）。

### Changed

- 所有 Go 模块、CI 与 Docker 构建统一使用 Go 1.24.3；根网关的最低构建版本从 Go 1.21 提升至 Go 1.24.3。

- 按冻结的 A 原型重建原生配置页展示层，恢复分组导航、对齐的总览表格、双列表单和原生下拉控件。

- 按确认的原型优化原生配置检查器：矢量角色图标与统一树形缩进、铺满剩余高度的 YAML、明确且原子的新建流程、独立的草稿／文件／运行差异、自定义波特率、页面内冲突重新基准化、键盘导航及大工程查询缓存。全部监听成功后才更新运行基线；管理连接中断明确提示，应用失败保留最近成功配置。

- 网关装配逻辑从 `main.go` 移至 `internal/app`，命令本身移至 `internal/cli`，桌面版的子进程运行的正是命令行版；参数与启动行为不变。
- 使用 `-exit-on-stdin-eof` 时网关忽略 SIGPIPE：父进程退出、输出无人读取后，仍会完成正常关闭并刷写持久化数据，而不会在写日志时被终止。
- 原生桌面版的颜色、字号与圆角由 `desktop-native/design/tokens.json` 生成（见 `desktop-native/design/README.md`）。状态文字使用更深的绿色和红色，徽标与结果文字达到 WCAG AA 对比度。
- 原生桌面版改为专业工具（IDE）风格：主操作、选中项和勾选框统一使用一种蓝色；按钮分为主要、默认、危险（红色描边）和链接四级；互斥选项由筛选胶囊改为分段控件，布尔筛选改为复选框；控件圆角改为 4dp；界面中的拉丁字母改用与中文相同的 Noto Sans SC，不再使用 Go 字体（等宽数值仍用 Go Mono）。

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
[0.6.0]: https://github.com/ffutop/modbus-gateway/compare/v0.5.0...v0.6.0
[0.6.1]: https://github.com/ffutop/modbus-gateway/compare/v0.6.0...v0.6.1
