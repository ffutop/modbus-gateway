# ModMux 原生桌面版（预览）

基于 [Gio](https://gioui.org) 的即时模式原生界面，不需要 WebView，也不需要额外的运行时。网关运行在子进程中：应用以 `--sidecar` 参数再次启动自身，执行与命令行版完全相同的 `internal/cli`，管理 API 只监听回环地址并要求一次性令牌。界面渲染与 GC 不与转发共用运行时；网关崩溃时界面保留并显示原因，界面退出（包括被强制结束）时网关正常关闭并刷写持久化数据。仍只分发一个可执行文件，界面与网关版本始终一致。

## 构建与运行

本目录是独立的 Go 模块，需要 **Go 1.24+**（Gio v0.10 的要求）。根模块仍保持 Go 1.21，二者通过 `replace` 指向同一份源码。

```bash
cd desktop-native
go build -o modmux-desktop .
./modmux-desktop -config ../config.yaml
```

配置文件与命令行版完全相同。界面采用顶部菜单、链路树、单网关拓扑及请求／寄存器联动布局（选型记录见 [DEVELOPMENT.md](DEVELOPMENT.md)），只展示真实运行数据，不产生示例流量。

配置加载或网关启动失败时，窗口打开 YAML 编辑器，并显示错误原因及网关最后几行输出。使用 `-config` 指定现有文件，可以修正并保存非法 YAML。状态栏显示网关正在启动、运行中或已停止，以及各上游的监听状态（监听中、启动中、失败原因）；网关输出同时写入应用日志。

配置支持可视化和 YAML 两种编辑方式：YAML 保存原文，v1 可视化修改保留注释与未进入表单的高级字段（格式可能重排）；v0 仅支持原文编辑，不自动升级。保存前执行完整配置校验及文件版本冲突检测，通过临时文件原子替换并保留权限；外部修改、校验或写入失败均保留草稿。页面分别显示保存状态与运行状态，变更检查器独立比较草稿／文件和文件／运行配置。仅注释或格式变化无需运行变更。“仅保存”不改变运行链路；“保存并应用”或“应用已保存配置”在确认后重启整个子进程，全部网关转发中断、全部运行中的 memory 模型清空。全部监听成功后才更新运行基线；监听失败停止子进程并显示原因。存在未保存修改或网关正在启动时不可重复应用。

YAML 模式隐藏对象树，文本区填满剩余高度并内部滚动。树使用统一矢量角色图标，上游 →□—、下游 —□→，网关总览 → 网关 → 上下游为三级结构，每层缩进 20dp；模型总览 → 模型为两级结构。Cmd／Ctrl+S 仅保存，Cmd／Ctrl+F 聚焦搜索；无文本焦点时 Cmd／Ctrl+Z、Shift+Z／Ctrl+Y 操作结构事务，文本焦点下保留编辑器自身撤销。弹窗中 Tab 限于弹窗，Esc 取消。

### 可视化配置工作台

模拟模型、网关和上下游支持新增、编辑、查询、复制与删除。总览可按名称、协议、地址、Slave ID、模型引用和映射搜索，筛选有问题、未保存、待生效或无引用对象；引用、所属网关和变更摘要均可跳转，返回保留查看位置。默认进入网关总览。网关和模拟模型各在树分组与总览提供具名创建入口；新网关先命名，再分别添加上下游，不预设设备。新建并引用模型在确认后同时创建模型和绑定当前 local／injector 下游，取消不改草稿，撤销同时恢复两者。新建的不完整对象留在草稿中；YAML 保存与 CLI 沿用相同核心校验。

普通字段失焦或切换对象时形成编辑事务；映射、模型选择与改名使用“完成编辑／放弃本次编辑”，仅在活动事务中显示。保存先完成当前事务。模型名称完成后联动更新全部引用；模型选择器支持搜索与新建引用。RTU 基础帧格式、超时与间隔直接显示，波特率支持自定义正整数；RS485 开关展开专用参数。当前底层 RTU 驱动未消费请求间隔和 RS485／RTS 参数，页面明确标注它们仅保留在 YAML 中，不承诺硬件生效。管理 API 与 pprof 在高级运行设置，桌面连接由 sidecar 管理。协议切换保留已编辑的非活动字段，原有 YAML 注释与未展示字段保留。删除与批量删除使用模态弹窗，背景操作暂停；删除模型先显示引用影响，可统一或逐条迁移引用，或明确连同引用下游删除；删除网关连同上下游删除，共享模型保留。删除配置不删除持久化文件或数据库内容。

总览勾选对象后可批量修改、复制、删除或移动下游；所有操作先预览，再写入草稿。引用或路由冲突拒绝整次操作。复制默认共享模型，也可生成独立 memory 模型（不复制运行数据）；复制网关需重新补全监听地址。批量新增下游支持名称前缀和递增 Slave ID，默认路由需先配置明确 ID。撤销／重做最多保留 100 次编辑事务，覆盖字段、映射和结构操作；放弃全部修改会清空历史。

未保存草稿在停止输入后自动写入配置旁的私有恢复文件，重新打开应用可恢复或丢弃；若原文件已变化会进入三份文本冲突页，可复制草稿、查看基线／磁盘／草稿，以已审阅磁盘版本重新基准化后在 YAML 手动合并；再次外部修改仍拒绝覆盖。新恢复记录同时保存基线文本，旧记录只有版本摘要时不伪造历史基线。“保存并应用”校验并保存后打开重启确认弹窗，说明转发中断与 memory 数据清空的影响。新配置启动失败后可手动运行本次应用会话中最近一次监听成功的配置，不覆盖已保存文件或未保存草稿；重启应用后不保留该运行快照。

当前开发批次的边界：

- 真实流量来自子进程管理 API 的事件流（含请求与响应 PDU）；保留最近 5000 条请求、每个网关最多 64 个主站入口，报告遥测缓冲区遗漏。请求控件随历史淘汰，历史写入使用环形缓冲区。
- 下游状态点表示最近请求结果，灰色为未观测，不代表实时连接状态。未观测到的设备读值显示“未观测”；按网关／下游／Slave ID 保存最后成功读取值，写入回显不会覆盖读值。
- 模型四张表通过管理 API 读取运行中的共享 Simulation，仅刷新正在查看的窗口；本批寄存器窗口显示地址 0–119。网关未运行时显示错误，不以零值充当运行数据。
- 当前遥测仅提供网关请求 PDU、下游返回 PDU 和总耗时。ADU、事务号、CRC、单段耗时及上游异常回复尚未采集，界面不会重建它们。
- 重启后若新配置启动失败，网关保持停止并显示原因；可修正后重新应用，或手动运行本次会话中最近一次成功的配置。

开发进度与后续验收见 [DEVELOPMENT.md](DEVELOPMENT.md)。

平台依赖：

- **Windows**：不需要 CGO，渲染使用系统自带的 Direct3D 11。
- **Linux**：需要 CGO，以及 X11/Wayland、EGL/Vulkan、xkbcommon 的开发包（参见 [Gio 安装说明](https://gioui.org/doc/install/linux)）。中文字体已随应用打包，无需额外安装。
- **macOS**：需要 Xcode 命令行工具。

界面字体链为 `Go, Noto Sans SC`，等宽字体链为 `Go Mono, Noto Sans SC`。Noto Sans SC 常规与粗体通过 Go embed 打包，原生界面和原型共用，字号保持 13 / 11.5 / 12。字体来自 [Noto 官方仓库](https://github.com/notofonts/noto-cjk/tree/main/Sans/SubsetOTF/SC)，许可见 `internal/uifont/assets/OFL.txt`。

## 打包与发布

`scripts/` 下每个平台一个打包脚本，环境变量 `VERSION`（写入程序的版本，默认 `dev`）与 `GOARCH`（`amd64` 或 `arm64`，默认 Go 工具链的架构）选择版本与架构，参数为输出目录（默认 `dist`）。产物为 `dist/modmux-desktop-<goos>-<goarch>.<zip|tar.gz>`，解包内容留在 `dist/<goos>-<goarch>/`。

| 脚本 | 构建主机 | 产物 |
|---|---|---|
| `package-macos.sh` | macOS（Xcode 命令行工具）；同一台 Mac 构建两种架构 | `ModMux.app` 的 zip，ad-hoc 签名并校验 |
| `package-windows.sh` | 任意（不需要 CGO；首次运行需联网获取 go-winres） | 含 `modmux-desktop.exe` 的 zip，窗口程序，双击不弹出控制台；exe 内嵌图标与版本信息 |
| `package-linux.sh` | Ubuntu／Debian，先以 `sudo ./scripts/install-linux-deps.sh [arm64]` 安装开发包；目标架构与主机不同时自动使用多架构库与交叉编译器 | 含 `modmux-desktop`、`install.sh`、桌面入口与图标的 tar.gz |

```bash
cd desktop-native
VERSION=v0.6.1 GOARCH=arm64 ./scripts/package-macos.sh
```

推送 `v*.*.*` 标签时，发布流水线（`.github/workflows/release.yaml` 的 `build-desktop`）在 macOS 与 Ubuntu runner 上生成全部 6 个包，与命令行版一同上传到 GitHub Release：macOS 两种架构都在 Mac 上构建，Windows 与 Linux arm64 在 amd64 的 Ubuntu 22.04 上交叉编译。Linux 包运行时需要系统提供 EGL、Wayland（client、cursor、egl）、X11（含 x11-xcb）、xkbcommon（含 x11）、Xcursor 与 Xfixes 的运行库，桌面发行版通常已自带。macOS 包只有 ad-hoc 签名，尚未做 Developer ID 签名与公证，下载后首次打开会被 Gatekeeper 拦截；Windows 包未做 Authenticode 签名。

macOS 的 `ModMux.app` 可从 Finder 双击；将它拖入“应用程序”后，可从启动台（旧版 macOS）或 Spotlight 的“应用”视图（macOS 26）打开，也可固定到 Dock。运行不需要终端或 Go 工具链。应用图标采用开发工具风格的蓝色几何折带 M，以中央交汇表达汇聚与分流，外侧透明。源 PNG 与设计提示词见 [packaging/icon/ICON-v3.md](packaging/icon/ICON-v3.md)，三个平台的图标都由它派生，打包时自动装入。

Windows 的 exe 内嵌图标资源，资源管理器、任务栏与窗口标题栏直接显示。Linux 上 Gio 不设置窗口图标，桌面环境按窗口的应用 ID（`com.ffutop.modmux.native`，即 X11 的 `WM_CLASS` 与 Wayland 的 `app_id`）查找同名桌面入口再取图标；解压后运行 `./install.sh` 把桌面入口与图标装入当前用户的 `$XDG_DATA_HOME`（默认 `~/.local/share`），之后可从应用菜单启动，Dock／任务栏显示 ModMux 图标。入口指向解压目录中的可执行文件，移动目录后需重新运行；`./install.sh --uninstall` 移除。

### 首次启动

macOS 应用包，或旁边带有 `config.default.yaml` 的 Windows／Linux 可执行文件（即解压后的发布包），在未指定 `-config` 时按打包应用启动，首次启动以该示例创建用户配置：

| 平台 | 配置（同时是相对持久化路径的工作目录） | 日志 |
|---|---|---|
| macOS | `~/Library/Application Support/ModMux/config.yaml` | `~/Library/Logs/ModMux/desktop.log` |
| Windows | `%APPDATA%\ModMux\config.yaml` | `%LOCALAPPDATA%\ModMux\Logs\desktop.log` |
| Linux | `$XDG_CONFIG_HOME/modmux/config.yaml`（默认 `~/.config`） | `$XDG_STATE_HOME/modmux/desktop.log`（默认 `~/.local/state`） |

默认配置为 memory 模型，在 `127.0.0.1:15020` 提供 Slave ID 1。可在应用内编辑，或退出应用后把工程配置复制到上述用户配置文件；更新／替换应用不会覆盖已有用户配置。工程中的 `config.yaml` 不会被自动打包。显式使用 `-config` 的命令行启动，以及旁边没有 `config.default.yaml` 的可执行文件（如 `go build` 的产物），继续沿用原有路径语义。

打包结构及启动方式参见 [Apple Bundle 文档](https://developer.apple.com/library/archive/documentation/CoreFoundation/Conceptual/CFBundles/BundleTypes/BundleTypes.html)、[Spotlight 应用视图说明](https://support.apple.com/guide/mac-help/open-apps-in-spotlight-mh35840/mac)与 [XDG Base Directory 规范](https://specifications.freedesktop.org/basedir-spec/latest/)。

## 目录

| 路径 | 内容 |
|---|---|
| `main.go` | 带 `--sidecar` 时作为网关子进程运行 `internal/cli`；否则加载配置、启动网关子进程，驱动窗口事件循环，网关重启后按配置文件重建工作台 |
| `internal/sidecar` | 子进程启动／停止／崩溃检测（`Supervisor`）与管理 API 客户端（后台缓存事件、寄存器窗口与监听状态） |
| `internal/live` | 工作台读取的数据源（`Source`）与进程状态（`Runtime`）接口；`Local` 为测试用的进程内实现 |
| `design` | 设计令牌唯一来源 `tokens.json`、对比度规则与生成器；规范见 [design/README.md](design/README.md) |
| `internal/workspace` | 唯一界面：菜单、拓扑、链路／请求／模型联动、配置编辑；仅投影真实运行数据。`tokens_gen.go` 由 `go generate ./design` 生成 |
| `internal/configfile` | 原文配置读取、校验、版本冲突检测及原子保存 |
| `internal/launch` | 打包应用（macOS 应用包、Windows／Linux 发布包）启动时的用户配置、工作目录与日志路径 |
| `scripts/package-*.sh`、`packaging/` | 三个平台的打包脚本；`packaging/config.default.yaml` 为各平台共用的首次启动示例，`packaging/icon/` 为图标源图，`packaging/macos/`、`packaging/linux/` 为平台专属资源 |
| `internal/decode` | Modbus PDU 字段解码，每个字段对应它在 PDU 中的字节区间 |

## 测试

```bash
go test ./...
# 离屏渲染工作台的常规／最小窗口、菜单、配置、YAML、启动失败与重启确认截图：
WORKSPACE_SNAPSHOT_DIR=/tmp/modmux-live go test ./internal/workspace -run TestWorkspaceSnapshots -count=1
```
