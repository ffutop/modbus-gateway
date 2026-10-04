# ModMux 设计规范

ModMux 是给现场工程师长时间盯着看的工业通信工具，不是营销页面。本规范约束 Gio 原生桌面版的界面：

| 界面 | 位置 | 令牌文件（生成，勿手改） |
|---|---|---|
| Gio 原生桌面版 | `desktop-native/internal/workspace` | `desktop-native/internal/workspace/tokens_gen.go` |

**唯一来源是 `design/tokens.json`。** 编辑 `tokens.json` 后运行 `go generate ./design`，提交 `tokens.json` 和生成的文件。效果用原生版的离屏截图测试查看（见“守卫”）。

语义颜色、字号和圆角的**名称**会被代码引用；增删名称时同步修改代码。

不在本规范范围内：GitHub Pages 站点（`docs/index.html`）、`docs/` 和 `desktop-native/prototype-ui` 下的原型。原型可以自由试验，验证通过的部分再回到令牌和组件清单。根目录的 `DESIGN.md` 是早期参考的 Cal.com 风格笔记，不是本产品规范。

## 原则

遇到风格争议，按以下顺序取舍：

1. **状态一眼可辨。** 正常、降级、故障必须同时用颜色、形状（圆点、徽标、边框）和文字表达，不能只靠颜色。色弱用户和黑白截图里也要能分清。
2. **数据优先于装饰。** 数值使用等宽字体或 `tabular-nums`。彩色只用来表达状态，不用来装饰或体现品牌。
3. **高密度但不拥挤。** 一屏装下网关、下游、日志和检查器，不能出现页面滚动；靠固定行高和细分隔线组织信息，不靠大留白。
4. **克制。** 主操作和选中态只用墨黑（`ink`），强调蓝（`accent`）只用于焦点和注入写入。新增颜色必须先说明现有令牌为什么不够用。

## 令牌

令牌分两层：

- **色板（palette）**：原始色值，只出现在 `tokens.json` 里，界面代码不引用。
- **语义令牌（color 等）**：按用途命名（`muted`、`err-bg`、`on-dark`），界面代码只引用这一层。

以后做暗色模式时，只需要为语义令牌换一套色板映射，组件代码不用动。

### 颜色角色

| 角色 | 令牌 | 规则 |
|---|---|---|
| 文本 | `ink` / `body` / `muted` | 在 `canvas` 和 `soft` 上对比度 ≥ 4.5 |
| 装饰文本 | `faint` | 仅用于零值、占位、键位提示，不能是信息的唯一载体 |
| 表面 | `canvas` / `soft` / `card` / `hover` / `backdrop` | `backdrop` 只用于应用框外 |
| 分隔 | `hair` / `hair-soft` / `line` / `line-strong` | 1px；`line*` 用于拓扑连线 |
| 深色区 | `dark` + `on-dark` / `on-dark-body` / `on-dark-muted` | 状态栏、网关节点、选中行 |
| 状态 | `ok` / `warn` / `err`（各带 `-bg` 与 `-solid`） | 见下文 |
| 强调 | `accent` / `accent-bg` / `accent-md` | 焦点环、注入写入、字段与字节联动高亮；不用于正文 |
| 变化 | `highlight` | 刚被写入的寄存器 |

每种状态有三个令牌，各有固定用途，不能混用：

- `ok` / `warn` / `err`：**文字**。在对应 `-bg` 和 `canvas` 上对比度 ≥ 4.5（700 色阶）。
- `-bg`：徽标、横幅的底色。
- `-solid`：圆点、描边、无效输入框边框（500/600 色阶）。`err-tint` 是无效字段和失败行的极淡底色。

`TestContrastRules` 会检查上述对比度，规则定义在 `design/contrast.go`。`warn-solid`（琥珀色）在白底上不足 3:1，因此琥珀圆点旁必须配文字。

### 状态词汇

同一状态在各端的表达必须一致：

| 状态 | 表达 | 例子 |
|---|---|---|
| 健康 | `ok-solid` 圆点 + 文字，或 `ok` 文字 | 管理连接正常；结果"正常" |
| 降级 / 重试中 | `warn-solid` 圆点 + 说明文字；徽标用 `warn` / `warn-bg` | 管理连接已断开，正在重试；请求过快未显示 |
| 故障 | `err` / `err-bg` 徽标或横幅；字段用 `err-solid` 边框 + `err-tint` 底色 + `err` 说明 | 启动失败；Slave ID 冲突；N 错误 |
| 数据过期 | `err` / `err-bg` 横幅，并把数值显示为"—" | 指标断流后，不把旧值或 0 当作健康值显示 |
| 未知 | `muted` 文字 | 运行数据未知 |
| 注入写入 | `accent` 虚线 | 拓扑图中的注入流 |
| 刚变化 | `highlight` 底色 | 寄存器视图 |

### 字号

字号令牌是设计尺寸，渲染尺寸 = 设计尺寸 × 密度（`density.desktop-native`，当前 1.04），结果四舍五入到 0.5sp。

| 令牌 | 设计尺寸 | 用途 |
|---|---|---|
| `micro` | 10.5 | 拓扑说明、键位提示 |
| `caption` | 11 | 表头、徽标、分组标题、状态栏 |
| `small` | 11.5 | 等宽数值、标签页、字段错误 |
| `body` | 12.5 | 默认文本 |
| `title` | 16 | 面板与对话框标题 |
| `display` | 18 | KPI 数字 |

界面代码不要写裸字号，统一引用字号令牌。

### 圆角

`xs` 3（键位提示）· `sm` 6（按钮、输入框、横幅）· `md` 8（分组框、KPI 卡）· `lg` 10（拓扑节点、对话框）· `pill`（徽标、分段控件；Gio 的 `rounded` 会把它收敛为半高）。

### 遮罩

遮罩只有两种（`alpha` 令牌）：`scrim` 用于对话框，`scrim-strong` 是窗口过小时的阻断层。界面不使用阴影。

## 组件清单

新增组件前先确认下列组件无法满足需求：

| 组件 | Gio | 需要覆盖的状态 |
|---|---|---|
| 按钮 | `button`（workspace）/ `smallButton`（ui） | 悬停、禁用、焦点 |
| 徽标 | `badge` | — |
| 状态点 | `dot` | — |
| 表格 | `table.go` | 选中、失败行 |
| 标签 | `tab` / `chip` | 选中 |
| 树节点 | 链路侧栏 | 悬停、选中 |

## 守卫

以下检查都在根模块中，CI 的 `go test ./...` 会运行：

- `TestGeneratedUpToDate`：生成文件与 `tokens.json` 不一致时失败。
- `TestNoRawColors`：`desktop-native/internal/workspace` 中出现裸色值（`rgb(0x…)`、`color.NRGBA{R: 0x…}`）时失败。
- `TestContrastRules`：对比度规则未达标时失败。
- `TestFileRoundTrip`：`tokens.json` 不是固定格式时失败（运行 `go generate ./design` 即可修正）。
- `tokens.json` 本身会校验：命名必须是 kebab-case、不允许未知字段、色板条目必须被引用。

视觉回归：在 `desktop-native` 中运行 `WORKSPACE_SNAPSHOT_DIR=… go test ./internal/workspace -run TestWorkspaceSnapshots` 渲染工作台截图（常规／最小窗口、菜单、配置、YAML、启动失败、重启确认）。

## 尚未收敛

- **间距与布局尺寸**：内边距、控件高度和行高仍是代码中的局部常量，暂未令牌化。新代码使用 4 的倍数（2 仅用于细微对齐）。
- **Gio 端部分尺寸**：如品牌字样 14sp、按钮内圆角 5dp，仍是局部常量。
- **文档站**：是否并入本规范尚待决定，目前仍使用 Tailwind zinc 色板。
