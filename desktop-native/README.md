# ModMux 原生桌面版（预览）

基于 [Gio](https://gioui.org) 的即时模式原生界面，不需要 WebView，也不需要额外的运行时。网关运行在子进程中：应用以 `--sidecar` 参数再次启动自身，执行与命令行版完全相同的 `internal/cli`，管理 API 只监听回环地址并要求一次性令牌。界面渲染与 GC 不与转发共用运行时；网关崩溃时界面保留并显示原因，界面退出（包括被强制结束）时网关正常关闭并刷写持久化数据。仍只分发一个可执行文件，界面与网关版本始终一致。

## 构建与运行

本目录是独立的 Go 模块，需要 **Go 1.24+**（Gio v0.10 的要求）。根模块仍保持 Go 1.21，二者通过 `replace` 指向同一份源码。

```bash
cd desktop-native
go build -o modmux-desktop .
./modmux-desktop -config ../config.yaml
```

配置文件与命令行版完全相同。正式入口已采用选定原型的顶部菜单、链路树、单网关拓扑及请求／寄存器联动布局。`prototype-ui` 仍是独立的模拟原型，正式入口不产生示例流量。

配置加载或网关启动失败时，窗口打开 YAML 编辑器，并显示错误原因及网关最后几行输出。使用 `-config` 指定现有文件，可以修正并保存非法 YAML。状态栏显示网关正在启动、运行中或已停止，以及各上游的监听状态（监听中、启动中、失败原因）；网关输出同时写入应用日志。

配置支持可视化和 YAML 两种编辑方式：YAML 保存原文，v1 可视化修改保留注释与未进入表单的高级字段（格式可能重排）；v0 仅支持原文编辑，不自动升级。保存前执行完整配置校验及文件版本冲突检测，通过临时文件原子替换并保留权限；外部修改、校验或写入失败均保留草稿。保存后的配置标为“待重启”，不会改变运行中的链路；工具栏“重启网关”在确认后重启子进程使其生效（确认提示说明转发中断，以及 memory 模型数据将清空）。存在未保存修改或网关正在启动时不可重启。

当前开发批次的边界：

- 真实流量来自子进程管理 API 的事件流（含请求与响应 PDU）；保留最近 5000 条请求、每个网关最多 64 个主站入口，报告遥测缓冲区遗漏。请求控件随历史淘汰，历史写入使用环形缓冲区。
- 下游状态点表示最近请求结果，灰色为未观测，不代表实时连接状态。未观测到的设备读值显示“未观测”；按网关／下游／Slave ID 保存最后成功读取值，写入回显不会覆盖读值。
- 模型四张表通过管理 API 读取运行中的共享 Simulation，仅刷新正在查看的窗口；本批寄存器窗口显示地址 0–119。网关未运行时显示错误，不以零值充当运行数据。
- 当前遥测仅提供网关请求 PDU、下游返回 PDU 和总耗时。ADU、事务号、CRC、单段耗时及上游异常回复尚未采集，界面不会重建它们。
- 重启后若新配置启动失败，网关保持停止并显示原因，不会自动回退到旧配置；修正并保存后可再次重启。

开发进度与后续验收见 [DEVELOPMENT.md](DEVELOPMENT.md)。

平台依赖：

- **Windows**：不需要 CGO，渲染使用系统自带的 Direct3D 11。
- **Linux**：需要 CGO，以及 X11/Wayland、EGL/Vulkan、xkbcommon 的开发包（参见 [Gio 安装说明](https://gioui.org/doc/install/linux)）。中文字体已随应用打包，无需额外安装。
- **macOS**：需要 Xcode 命令行工具。

界面字体链为 `Go, Noto Sans SC`，等宽字体链为 `Go Mono, Noto Sans SC`。Noto Sans SC 常规与粗体通过 Go embed 打包，原生界面和原型共用，字号保持 13 / 11.5 / 12。字体来自 [Noto 官方仓库](https://github.com/notofonts/noto-cjk/tree/main/Sans/SubsetOTF/SC)，许可见 `internal/uifont/assets/OFL.txt`。

## macOS 应用打包

在 macOS 上执行一次构建：

```bash
cd desktop-native
./scripts/package-macos.sh
```

生成 `dist/ModMux.app`，可从 Finder 双击；将它拖入“应用程序”后，可从启动台（旧版 macOS）或 Spotlight 的“应用”视图（macOS 26）打开，也可固定到 Dock。运行不需要终端或 Go 工具链。脚本只构建当前 Mac 的架构，并执行本地 ad-hoc 签名与签名校验；尚未做 Developer ID 签名、公证或跨架构发布。

应用图标采用开发工具风格的蓝色几何折带 M，以中央交汇表达汇聚与分流，外侧透明。源 PNG、macOS `.icns` 及设计提示词见 [packaging/macos/ICON.md](packaging/macos/ICON.md)，打包时自动装入应用资源。

首次从应用包启动时会创建：

- 配置：`~/Library/Application Support/ModMux/config.yaml`
- 日志：`~/Library/Logs/ModMux/desktop.log`
- 相对持久化路径的工作目录：`~/Library/Application Support/ModMux/`

默认配置为 memory 模型，在 `127.0.0.1:15020` 提供 Slave ID 1。可在应用内编辑，或退出应用后把工程配置复制到上述用户配置文件；更新／替换应用包不会覆盖已有用户配置。工程中的 `config.yaml` 不会被自动打包。显式使用 `-config` 的命令行启动继续沿用原有路径语义。

打包结构及启动方式参见 [Apple Bundle 文档](https://developer.apple.com/library/archive/documentation/CoreFoundation/Conceptual/CFBundles/BundleTypes/BundleTypes.html)与 [Spotlight 应用视图说明](https://support.apple.com/guide/mac-help/open-apps-in-spotlight-mh35840/mac)。

## 目录

| 路径 | 内容 |
|---|---|
| `main.go` | 带 `--sidecar` 时作为网关子进程运行 `internal/cli`；否则加载配置、启动网关子进程，驱动窗口事件循环，网关重启后按配置文件重建工作台 |
| `internal/sidecar` | 子进程启动／停止／崩溃检测（`Supervisor`）与管理 API 客户端（后台缓存事件、寄存器窗口与监听状态） |
| `internal/live` | 工作台读取的数据源（`Source`）与进程状态（`Runtime`）接口；`Local` 为测试用的进程内实现 |
| `internal/workspace` | 唯一界面：菜单、拓扑、链路／请求／模型联动、配置编辑；仅投影真实运行数据。`tokens_gen.go` 由 `go generate ./design` 生成 |
| `internal/configfile` | 原文配置读取、校验、版本冲突检测及原子保存 |
| `internal/launch` | 应用包启动的用户配置、工作目录与日志路径 |
| `scripts/package-macos.sh` | 构建并校验 macOS `.app` |
| `internal/decode` | Modbus PDU 字段解码，每个字段对应它在 PDU 中的字节区间 |

## 测试

```bash
go test ./...
# 离屏渲染工作台的常规／最小窗口、菜单、配置、YAML、启动失败与重启确认截图：
WORKSPACE_SNAPSHOT_DIR=/tmp/modmux-live go test ./internal/workspace -run TestWorkspaceSnapshots -count=1
```
