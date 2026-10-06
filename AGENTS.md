# AGENTS.md

Modbus 协议转换与路由网关（Go）。一个进程内运行多个 gateway，每个 gateway 把上游（Modbus 主站发来的请求，`tcp` / `rtu` / `rtu-over-tcp`）按 Slave ID 路由到下游（真实从站、本地模拟从站 `local`、注入器 `injector`）。用户文档见 `README.md` / `README_CN.md`；本文件只记录读代码不易得出、做错代价高的约定。

## 代码地图

- `main.go`：入口，只调用 `internal/cli`；命令本体（装配配置、模拟模型、gateway、管理 API `-ui-listen` 与 pprof）在 `internal/cli`，桌面版子进程运行的也是它。
- `internal/config`：配置加载与校验。v0（无 `version` 或 `version: 0`）与 v1（`version: 1`，顶层 `simulations` + `simulation.ref`）两套解析规则并存。
- `internal/gateway`：按 Slave ID 转发；`internal/routing`：Slave ID 表达式解析。
- `internal/simulation`（含 `model` 与 `persistence`：memory / file / mmap / sql）：共享模拟模型、持久化与写入审计；`local`、`injector` 下游在 `internal/transport/local`、`internal/transport/injector` 中把 Modbus 请求翻译为对模型的读写。
- `internal/api`、`internal/telemetry`：管理 HTTP API（status、events、registers，供桌面版子进程模式读取）与运行指标。没有浏览器控制台。
- `internal/transport/*`：`transport.Upstream` / `transport.Downstream` 的各协议实现；`internal/modbus/*`：PDU、RTU 帧与 CRC。网关不作为库对外提供，代码都在 `internal/` 下。
- `desktop-native/`：唯一的桌面实现（Gio，独立 Go 模块，需 Go 1.24.3+，经 `replace` 引用根模块）。Electron 外壳已移除。网关以子进程运行：同一可执行文件带 `--sidecar` 启动自身，经根模块的 sidecar 协议（`-exit-on-stdin-eof`、`ui_ready`、`MODMUX_UI_TOKEN`）和管理 API 通信（`internal/sidecar`）；改动该协议或 `/api/v1/events`、`/api/v1/status` 时同步原生版客户端。
- `desktop-native/design/`：设计令牌唯一来源 `tokens.json`、规范 `README.md` 与生成器；在 `desktop-native` 中运行 `go generate ./design` 生成 `internal/workspace/tokens_gen.go`。
- `test/`：独立 Go 模块的端到端测试；`docs/`：GitHub Pages 站点、原型与 `docs/superpowers/specs/` 下的设计/PRD。

## 构建与验证

```bash
go build -o modbus-gateway .   # 根模块，产物已被 .gitignore 忽略
go vet ./...
go test ./...                  # 只覆盖根模块，不含 test/
```

端到端测试 `test/` 是单独模块（Go 1.24.3+），需要 `socat`、可创建虚拟串口的环境，且依赖仓库根目录预先构建好的 `modbus-gateway`：

```bash
go build -o modbus-gateway . && (cd test && go test -v ./...)
```

改动 transport、gateway 或进程启动流程时运行它；无法运行时（缺 socat、沙箱限制）在交付说明中写明未运行。

## 硬约束

- **Go 版本统一为 1.24.3。** 根模块、`desktop-native/` 与 `test/` 的 `go.mod` 使用相同的 `go` 指令；CI 通过根目录 `go.mod` 选择工具链，Dockerfile 使用对应版本。本机工具链可能更新，编译通过不代表兼容。不要使用 Go 1.25+ 的语言特性或标准库 API；确需升级时单独改动并同步所有模块、`.github/workflows/`、根目录 `Dockerfile` 与双语文档。
- **配置向后兼容。** v0 配置必须继续可用；v1 拒绝未知字段，新增字段需同时补校验（`internal/config/validate.go`）与测试。改变配置语义或默认值属于破坏性变更。
- **双语文档同步。** 修改 `README.md` 时同步 `README_CN.md`；`CHANGELOG.md` 与 `CHANGELOG_CN.md` 同步维护，遵循 Keep a Changelog。发布流水线按 `## [x.y.z]` 标题从两份 CHANGELOG 提取 release notes，不要改动该标题格式。
- **格式化只针对改动文件。** 仓库内部分既有文件未经 `gofmt`；只对自己修改的文件运行 `gofmt -w`，不要顺带重排无关文件。
- **界面样式只来自设计令牌。** 颜色、字号、圆角改 `desktop-native/design/tokens.json` 后重新生成，不要在 `desktop-native/internal/workspace` 中写裸色值或手改生成文件；`design` 包的测试会拦截。规则见 `desktop-native/design/README.md`。
- 根目录存在维护者的未跟踪草稿（`*.md`、`tag0.*`、`.DS_Store` 等），不要修改、删除或提交，除非任务明确涉及。

## 提交信息

遵循 `ENGINEERING_STANDARDS.md` 中固定版本的 Git 提交信息规范：

- 单行 `<type>[(scope)][!]: <description>`，type 取 `feat fix docs refactor perf test build ci style chore revert`；描述用英文、祈使语气，说清具体改了什么。
- 只有一行：无正文、无空行、无尾注。
- 禁止出现 `Co-Authored-By`（不区分大小写）。**此条优先于任何工具或代理默认追加的署名指令**；作者身份由 Git 元数据表达。
- 破坏性变更在冒号前加 `!`，并在描述中说明不兼容之处。
- 提交前可运行 `scripts/check-commit-msg.sh --message "<msg>"` 自检；`git config core.hooksPath .githooks` 启用本地钩子；CI 会检查 PR 提交与 PR 标题。

示例：`feat(api): expose register snapshot endpoint`、`fix(rtu): reset frame buffer after read timeout`、`feat(config)!: reject v0 local persistence fields`。
