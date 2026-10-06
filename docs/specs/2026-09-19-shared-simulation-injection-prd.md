# 共享模拟模型与标准 Modbus 数据注入 — 产品需求文档

**日期：** 2026-09-19 起草；2026-10-06 按 v0.6.2 实现倒推重写
**状态：** 已实现（v0.5.0 引入，v0.6.x 增加只读管理 API）
**范围：** `simulations` 共享模型、`local` 业务入口、`injector` 注入入口及其配置、校验、持久化与可观测性
**依据：** 本文描述代码的实际行为，不描述未实现的设想；差异以代码为准。代码位置见第 10 节。

本文合并并取代此前的《本地模拟从站数据填充 PRD》与《共享模拟数据模型配置契约 PRD》。

---

## 1. 问题

一台真实 Modbus 从站有四张数据表：

| 数据表 | 标准读 | 标准写 | 真实设备中的数据来源 |
|---|---|---|---|
| Coils（0x） | FC01 | FC05、FC15 | 主站写入 |
| Discrete Inputs（1x） | FC02 | 无 | 现场开关量采集 |
| Holding Registers（4x） | FC03 | FC06、FC16 | 主站写入 |
| Input Registers（3x） | FC04 | 无 | 现场模拟量采集 |

网关内的模拟从站没有现场 I/O，所以 1x、3x 没有数据来源。而 Modbus 协议本身不允许写这两张表，被测主站也不应获得写入它们的能力。

需要解决的是：**让外部数据源填充模拟从站的 1x/3x，同时让被测主站看到的仍是一台行为标准的从站。**

## 2. 从问题推导出的设计决定

| # | 第一性约束 | 推导出的决定 |
|---|---|---|
| D1 | 填充端与被测主站必须看到**同一份**数据 | 数据模型从路由中独立出来，成为进程级具名资源 `simulations[]`；入口用 `simulation.ref` 引用，而不是各自创建模型 |
| D2 | 现场填充工具（PLC、SCADA、测试脚本）大多只会标准 Modbus | 注入使用标准写功能码 FC05/06/15/16，由网关把写入重映射到 1x/3x；不引入私有功能码，也不提供 HTTP 写入 |
| D3 | Slave ID 是某条 Modbus 网络上的地址，不是数据本身的属性 | Slave ID 写在路由项（下游）上；同一模型可在不同网关以不同 Slave ID 暴露 |
| D4 | 被测主站不能借注入入口改写 1x/3x | 注入入口只接受写请求、只写 1x/3x、只在显式映射范围内生效；业务与注入的访问隔离依靠不同监听端口或网络，不依靠 Slave ID |
| D5 | 既有部署不能因升级而失效 | 不声明 `version` 的配置按 v0 解析，每个 `local` 仍是独立模型；只有 `version: 1` 才启用共享模型与注入 |
| D6 | 模拟从站优先保证可用，被测主站不应因磁盘问题看到异常 | 存储在启动时必须可用，否则进程退出；运行期写盘失败不影响 Modbus 响应，模型标记为 `degraded` 并记录日志 |

## 3. 目标与非目标

**目标**

- 四张表都可由受控来源填充；被测主站按标准功能码访问，行为不变。
- 一个具名模型可被多个业务入口和多个注入入口共享，也可跨网关共享。
- 写入可审计，模型状态与寄存器值可通过只读接口观察。
- 已有 v0 配置无需修改即可继续运行。

**非目标**

- 模拟 PLC 逻辑、变化率、故障或物理过程；数据保持到下一次写入为止，不自动过期或回零。
- 浮点、32 位整数、缩放、字序等数据类型语义；映射只做 bit→bit 和 `uint16`→`uint16`。
- 私有功能码、HTTP 写入或注入、跨多个 Modbus 请求的事务。
- 从真实设备自动同步数据到模拟模型。
- 认证与加密；Modbus 入口本身没有访问控制。

## 4. 配置契约

### 4.1 示例

```yaml
version: 1

simulations:
  - name: sim-device
    persistence:
      type: mmap
      path: ./data/sim-device.bin

gateways:
  - name: business-modbus          # 被测主站访问
    upstreams:
      - type: tcp
        tcp: { address: "0.0.0.0:502" }
    downstreams:
      - name: sim-business
        type: local
        slave_ids: "100"
        simulation: { ref: sim-device }

  - name: simulation-injection     # 仅受信任填充端可达
    upstreams:
      - type: tcp
        tcp: { address: "127.0.0.1:1502" }
    downstreams:
      - name: sim-injector
        type: injector
        slave_ids: "247"
        simulation:
          ref: sim-device
          mappings:
            - source: { table: coils, start_address: 0, count: 256 }
              target: { table: discrete_inputs, start_address: 0 }
            - source: { table: holding_registers, start_address: 0, count: 1024 }
              target: { table: input_registers, start_address: 0 }
```

### 4.2 字段

| 字段 | 版本 | 说明 |
|---|---|---|
| `version` | — | 缺省或 `0` 为 v0；`1` 为 v1；其他值拒绝加载 |
| `simulations[].name` | v1 | 非空且进程内唯一；是模型、版本号、状态与持久化的归属 |
| `simulations[].persistence.type` | v1 | `memory`、`file`、`mmap`、`sql`；空值或未知值按 `memory` 处理 |
| `simulations[].persistence.path` | v1 | `file`/`mmap` 为文件路径；`sql` 为 DSN |
| `downstreams[].type: local` | v0/v1 | 业务入口：在引用的模型上执行标准读写 |
| `downstreams[].type: injector` | 仅 v1 | 注入入口：把标准写请求映射到引用模型的 1x/3x |
| `downstreams[].slave_ids` | v0/v1 | v1 的 `local`/`injector` 必须恰好解析为一个 1–247 的 ID |
| `downstreams[].simulation.ref` | 仅 v1 | `local`/`injector` 必填，必须引用已声明的模型 |
| `downstreams[].simulation.mappings[]` | 仅 v1 | `injector` 必填且至少一项；`local` 不得声明 |
| `mappings[].source` | 仅 v1 | `table` 为 `coils` 或 `holding_registers`；`start_address` 为 0 基 PDU 地址；`count` 为映射容量（>0） |
| `mappings[].target` | 仅 v1 | `table` 必须与源表配对（`coils→discrete_inputs`、`holding_registers→input_registers`）；`start_address` 为 0 基地址，长度等于源 `count` |
| `downstreams[].local.persistence` | 仅 v0 | v0 每个 `local` 的独立存储；v1 不接受该字段 |

### 4.3 v0 兼容

- 每个 v0 `local` 下游在内部生成一个独立模型，命名为 `__v0:<网关名>#<下游序号>`，持久化取自它的 `local.persistence`。两个 v0 `local` 永不共享数据。
- v0 未知字段继续被忽略；但出现 `simulations`、下游 `simulation` 或 `type: injector` 时拒绝加载，避免漏写 `version: 1` 后这些配置被静默丢弃。
- v0 `local` 的 `slave_ids` 不受单 ID 限制。
- 迁移到 v1 时：把存储移到顶层 `simulations`，`local` 改用 `simulation.ref`；原先一个 `local` 绑定多个 ID 的，拆成多个 `local` 引用同一模型。

## 5. 运行行为

### 5.1 数据模型

- 每个模型保存四张表的完整 0–65535 地址空间，初值为 0（或从存储恢复）。
- 线圈与离散输入按 bit 存储，响应按 Modbus 规则打包：范围内第一个 bit 位于第一个字节的最低位。寄存器为 `uint16`，网络字节序为大端。
- 所有地址均为 0 基 PDU 地址，不使用 `00001`、`10001`、`30001`、`40001` 表示法。

### 5.2 `local` 业务入口

| 功能码 | 行为 |
|---|---|
| FC01/02/03/04 | 读取对应表；数量上限 2000 bit 或 125 个寄存器 |
| FC05 | 写线圈：`0xFF00` 为 ON，`0x0000` 为 OFF；**其他值被忽略但仍回显成功** |
| FC06 | 写保持寄存器 |
| FC15/FC16 | 批量写线圈或保持寄存器；数量上限 1968 bit 或 123 个寄存器；字节数必须与数据长度一致 |
| 其他 | `Illegal Function` |

数量越界或报文长度不符返回 `Illegal Data Value`；地址范围越界返回 `Illegal Data Address`。`local` 无法写 1x/3x。

### 5.3 `injector` 注入入口

| 功能码 | 源表 | 写入目标 |
|---|---|---|
| FC05 | Coils | Discrete Inputs；**任意非零值为 ON**，零为 OFF |
| FC15 | Coils | Discrete Inputs |
| FC06 | Holding Registers | Input Registers |
| FC16 | Holding Registers | Input Registers |
| 其他（含所有读请求） | — | `Illegal Function`，模型不变 |

- **映射解析：** 请求范围 `[address, address+quantity)` 必须完整落在同一源表的某一个映射内；目标地址为 `target.start_address + (address − source.start_address)`。未映射、部分越出或跨两个映射时返回 `Illegal Data Address`，模型不变。
- **只写目标：** 注入从不修改模型的 Coils 和 Holding Registers；源表只是注入端的“地址窗口”。
- **响应：** 与标准写响应相同，回显的是请求中的源地址；不向注入端暴露目标地址、版本号或状态。
- **容量与报文：** `count` 是映射窗口容量，单个请求仍受 FC15 1968 bit、FC16 123 个寄存器的协议上限约束；填满大窗口需要多个请求。

### 5.4 一致性与版本

- 每个读或写请求在模型锁内完成：单个 FC15/FC16 对读取者整体可见，读取者不会看到同一请求的部分新值。
- 不提供跨请求的批次原子性；多个请求之间可被其他读写穿插，同一地址以后提交者为准。
- 每次成功写入使模型版本号加一，与写盘是否成功无关。版本号只出现在审计日志与管理 API 中，不进入 Modbus 响应，**不持久化**，进程重启后从 0 开始。

### 5.5 持久化

| 类型 | 行为 |
|---|---|
| `memory` | 不持久化，重启后清零 |
| `file` | 启动时读入；每次写入后把整个 393216 字节数据区写回文件并 fsync |
| `mmap` | 启动时映射文件；每次写入后 flush |
| `sql` | 当前构建未注册 SQLite 驱动，配置后启动失败 |

- `file`/`mmap` 文件布局固定为 393216 字节（四表依次排列）；大小不符时会被调整，较大的文件会被截断。寄存器按主机字节序存储，不保证跨字节序架构迁移。
- 不保证意外断电时零丢失。

### 5.6 启动、状态与关闭

- 启动时先逐个打开全部模型（包括没有被任何入口引用的模型），再创建网关。任一模型存储打开或恢复失败，整个进程退出。
- 模型状态只有两种：`ready`，以及运行期写盘失败后的 `degraded`。`degraded` 不会自动恢复；内存写入仍然生效，Modbus 照常返回成功，首次降级时记录一条错误日志。恢复需要排除存储问题后重启进程。
- 配置错误不会形成运行期状态：加载或路由校验失败时，进程不启动。
- 收到 SIGINT/SIGTERM（桌面版子进程模式下 stdin 关闭）时，先停止网关，再关闭各模型的存储。

## 6. 配置校验

以下任一规则不满足，进程拒绝启动；桌面版配置编辑器通过同一套规则逐项列出问题。

| 规则 | 适用 |
|---|---|
| `version` 只能是 0 或 1，且为整数 | 全部 |
| v1 出现未知字段 | v1 |
| v0 出现 `simulations`、`simulation` 或 `injector` | v0 |
| 模型名为空或重复 | v1 |
| `local`/`injector` 缺少 `simulation.ref` 或引用了不存在的模型 | v1 |
| `local`/`injector` 的 `slave_ids` 不是恰好一个 1–247 的 ID | v1 |
| `local` 声明了 `mappings`；`injector` 没有映射 | v1 |
| 映射表配对非法、`count` 为 0、源或目标范围超出 0–65535 | v1 |
| 同一 `injector` 内同一源表的映射范围重叠 | v1 |
| 引用同一模型的所有 `injector`（可跨网关）中，同一目标表的范围重叠 | v1 |
| 两个上游的监听地址或串口设备字符串完全相同 | v1 |
| 同一网关内同一 Slave ID 路由到多个下游，或 `slave_ids` 无法解析 | 全部 |
| 启用 `ui` 时，管理 API 地址与任一 Modbus TCP 上游端口冲突 | 全部 |

## 7. 可观测性

- **审计日志：** `local` 与 `injector` 的每次写入（含被拒绝的注入）记录一条 `simulation write` 日志，字段包括模型名、入口类型（`local`/`injector`）、来源连接地址、目标表、地址、数量、当前版本、状态与结果。被拒绝的注入记录的是请求中的源地址。日志不记录写入的值。
- **管理 API**（默认关闭，`ui.enabled` / `ui.listen` 或 `-ui-listen` 启用，只接受 GET）：
  - `/api/v1/status`：各模型的名称、状态与版本号（v0 模型以 `__v0:` 名称出现），以及上游监听状态；
  - `/api/v1/simulations/<name>/registers?table=&start=&count=`：读取任一表的一段值，单次最多 2048 个；
  - `/api/v1/events`：SSE 推送每个 Modbus 请求的网关、下游、来源、Slave ID、功能码、PDU 与耗时，注入请求同样包含在内。

## 8. 安全边界

- 业务入口与注入入口的隔离**完全依赖部署**：放在不同网关、使用不同监听地址或串口，并由网络限制只有受信任的填充端能到达注入端口。配置校验**不强制**这一点；把 `injector` 和 `local` 放进同一个网关也能通过校验，此时被测主站同样可以注入。
- 独立的注入 Slave ID 只是逻辑区分，不是访问控制。
- Modbus 入口没有认证与加密。管理 API 只读；监听回环地址时校验 Host 头以防 DNS 重绑定，设置 `MODMUX_UI_TOKEN` 后要求 Bearer token。

## 9. 已知限制

| 限制 | 影响 |
|---|---|
| 隔离不由配置强制（第 8 节） | 误配置会让被测主站获得注入能力 |
| 监听冲突只比较字符串，`0.0.0.0:502` 与 `127.0.0.1:502` 不算冲突 | 这类冲突要到绑定端口时才暴露 |
| `degraded` 不自动恢复，版本号不持久化 | 需要重启恢复；重启后版本号不连续 |
| `local` FC05 非标准值被忽略却返回成功；`injector` FC05 把任意非零值当作 ON | 与 Modbus 规范对非法值返回 `Illegal Data Value` 的要求不一致 |
| 审计日志不记录值、不可单独开关或摘要化 | 大批量注入时日志量随请求数增长 |
| `file` 每次写入都写回整个数据区 | 高频写入时磁盘开销较大 |
| 无跨请求事务、无元数据（源时间、质量码） | 需要这些语义时须另行设计 |

## 10. 验收依据

| 行为 | 自动化测试 |
|---|---|
| 注入后业务端经 FC02/FC04 读到相同值 | `test/shared_simulation_test.go`：`TestSharedSimulation_InjectionFillsDiscreteAndInputRegisters` |
| 注入端拒绝读请求与未映射地址，模型不变 | `TestSharedSimulation_InjectorRejectsUnmappedAndReadRequests`；`internal/transport/injector` 中的单元测试 |
| 业务端与注入端路由互相隔离 | `TestSharedSimulation_RouteIsolationBetweenBusinessAndInjection` |
| 注入数据经持久化在重启后恢复 | `TestSharedSimulation_PersistenceRestartRecoversInjectedData`；`test/persistence_test.go` |
| 写盘失败仍返回成功并进入 `degraded`；版本号递增 | `internal/simulation/simulation_test.go` |
| v0 兼容与 v1 各项拒绝规则 | `internal/config/config_test.go`：`TestV0_*`、`TestConfig_RejectionCases` 等 |
| 管理 API 默认关闭、token 与 Host 校验 | `test/management_api_test.go` |

**代码位置：** 配置与校验 `internal/config/`；模型、版本、状态与审计 `internal/simulation/`；持久化 `internal/simulation/persistence/`；业务入口 `internal/transport/local/`；注入入口 `internal/transport/injector/`；启动装配 `internal/app/`、`internal/cli/`；管理 API `internal/api/`。
