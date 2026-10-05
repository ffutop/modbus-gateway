# desktop-native 配置工作台实施与验收记录

日期：2026-10-05。基线：config-A-v1。生产实现已写入工作区，自动验证通过；尚未进行 Windows／Linux GUI、真实串口硬件或维护者最终验收。既有未提交工作保留，固化原型未修改。

## 已实现

- 默认网关总览；网关／模型总览与角色树，展开与选择分开。12dp 展开位、16dp 矢量图标、6dp 列间距、20dp 层级缩进。上游 →□—、下游 —□→，local／injector 按下游角色显示；网关总览 → 网关 → 上下游为三级结构；模型总览 → 模型为两级结构，全局设置使用滑杆图标。
- YAML 隐藏树，编辑器占满剩余区域并内部滚动。只查看 YAML 不生成默认字段、不制造未保存修改；不覆盖非法中间输入。真实文件名可查看完整路径，长中文与 IPv6 验收无操作遮挡。
- 分组与总览入口共用创建弹窗；新网关先命名、没有预设设备。模型可选持久化，“新建并引用”原子创建与绑定，取消不修改，撤销恢复两者。
- 三份配置独立比较与并列状态：A 运行、B 保存、C 未保存时，待保存为 B→C，待应用为 A→B。仅文本变化说明无需运行变更；已保存配置可直接确认应用，取消保留文件。全部监听成功才提交运行基线；监听失败停止进程并保留最近成功配置。恢复运行、停止、启动中、管理连接中断分别报告。
- 自定义波特率与常用值快捷选择；基础 RTU 帧格式直接显示。管理 API（CLI）与 pprof 为高级设置。协议切换先提交合法活动字段，再保留其非活动值和注释。命名习惯不阻止 CLI 支持的无名称对象保存，YAML 使用核心合法性校验。
- 规范化 ID 集合与覆盖数量、真实端点／模型摘要；映射源／目标区间及零基地址说明，目标表按源表约束。模型持久化说明使用当前工作目录和实际文件名语义。
- 无选择时隐藏批量动作；选中后报告数量和隐藏选择，只对纯下游选择提供移动。上游／下游总览作为查询入口，不重复树分支。无活动事务隐藏完成／放弃，无历史禁用撤销／重做／返回。
- 三份文本冲突页支持复制草稿、查看基线／磁盘／草稿，以精确审阅的磁盘版本重新基准化后手动合并；再次外部变更仍拒绝覆盖。恢复记录保存历史基线文本；旧记录不伪造历史文本。
- Cmd／Ctrl+S、Cmd／Ctrl+F、结构撤销重做与文本撤销优先；弹窗 Tab 限定与 Esc 取消；背景命令暂停。普通字段失焦／切换对象形成事务，模型与映射保留显式事务。问题摘要折叠，定位会展开高级字段及父网关。
- 全局差异、模型引用及恢复序列化按状态缓存；移除逐下游全量扫描差异键的平方复杂度。

## 验证

| 检查 | 结果 |
| --- | --- |
| 根模块 `go test ./...`、`go vet ./...` | 通过；根模块源码与 Go 1.21 指令未改 |
| desktop-native `go test ./...`、`go vet ./...` | 通过，包含设计令牌与对比度守卫 |
| `go test -race ./internal/sidecar ./internal/configfile` | 通过 |
| 独立 test 模块 `go test ./...` | 通过，27.628s；已构建根程序，socat 可用 |
| Gio 原生截图 | 25 个场景 × 1100×680／1440×900，50 张；人工检查树、YAML、创建、双差异、冲突及长值 |
| 原生桌面程序构建 | 通过；开发验收产物 `/private/tmp/modmux-desktop-config-A` |
| `git diff --check` | 通过 |

规模：100 个网关、1000 个下游、200 个模型、2000 条映射。环境：darwin／amd64，16 个逻辑 CPU，go1.24.0。40 次搜索到实际 Gio 布局完成：p95 **33.30ms**，最大 39.33ms，满足 200ms 目标；初次加载 835.58ms。该测量不包含 GPU 合成／屏幕呈现延迟，不替代目标机器上的人工体验验收。

- [规模原始记录](verification/scale.json)
- [总览（1100）](verification/config-overview-1100.png)
- [新建并引用（1100）](verification/config-create-reference-1100.png)
- [三份配置（1100）](verification/config-three-versions-1100.png)
- [YAML（1100）](verification/yaml-1100.png)
- [冲突（1100）](verification/config-conflict-1100.png)
- [长值（1100）](verification/config-long-values-1100.png)

## 实际能力边界

当前根模块 RTU client／server 只接入设备、波特率、数据位、停止位、校验位和超时；请求间隔、RS485／RTS 参数尚未传给底层驱动。页面明确标注仅保留配置，不假装硬件生效；本轮没有改变底层串口协议或默认值。SQLite3 仍是唯一 sql 持久化实现，没有增加服务器数据库驱动。

截图使用确定性 fixture；页面内重新基准化的真实磁盘边界由 configfile 测试验证。Windows／Linux GUI、真实串口和生产现场应用尚待验收，未声称通过。冻结浏览器原型保留为历史设计证据，生产实现不依赖其脚本或令牌快照。

## 复验

在 desktop-native 目录执行：

```sh
go test ./...
go vet ./...
go test -race ./internal/sidecar ./internal/configfile
WORKSPACE_SNAPSHOT_DIR=/private/tmp/modmux-config-review go test ./internal/workspace -run TestWorkspaceSnapshots -count=1
WORKSPACE_SCALE_REPORT=/private/tmp/modmux-config-scale.json go test ./internal/workspace -run TestConfigurationScaleInteraction -v -count=1
go build -o /private/tmp/modmux-desktop-config-A .
```

运行验收程序：`/private/tmp/modmux-desktop-config-A -config /绝对路径/现场配置.yaml`。此命令会实际启动网关子进程，应使用现场认可的测试配置。
