# Domain docs

用户于 2026-10-05 选择 single-context。约定领域上下文位于根目录 `CONTEXT.md`，已采纳架构决定位于 `docs/adr/`。本次只记录布局，不自动创建或采纳 ADR。

截至设置时，这两个领域文档入口尚不存在。读取相关任务时先检查它们是否已建立；缺失时使用 `AGENTS.md`、相关双语 README 与当前代码，不虚构历史决定。`docs/superpowers/specs/` 中的 PRD/设计提案可作背景，须核对状态与实现，不能自动当作已采纳 ADR。

统一词汇：

- gateway（网关）：一组上游入口与按 Slave ID 路由的下游；同一进程可有多个。
- upstream（上游）：接收 Modbus 主站请求的入口。
- downstream（下游）：真实设备或 local/injector 路由目标。
- Simulation（共享模拟模型）：命名的数据模型，可被多个下游引用。
- local：按标准 Modbus 语义访问共享模型的业务入口。
- injector：按映射将写入转入共享模型的注入入口。
- draft / saved / running：编辑草稿、磁盘配置、实际运行配置；保存不等于生效。
- sidecar：桌面应用启动的网关子进程；当前配置应用通过重启整个子进程实现。

根模块和桌面模块共享上述术语。根模块、桌面模块与端到端测试模块统一要求 Go 1.24.3+。
