# ModMux 设计规范

ModMux 是给现场工程师长时间盯着看的工业通信工具，不是营销页面。本规范约束三处界面，让它们看起来是同一个产品：

| 界面 | 位置 | 令牌文件（生成，勿手改） |
|---|---|---|
| 浏览器控制台 | `web/` | `web/tokens.css` |
| Electron 桌面版停机页 | `desktop/stopped.*` | `desktop/tokens.css` |
| Gio 原生桌面版 | `desktop-native/internal/ui` | `desktop-native/internal/ui/tokens_gen.go` |

**唯一来源是 `design/tokens.json`。** 修改有两种方式，结果相同，最后都要提交 `tokens.json` 和生成的文件：

- **可视化调配**：在仓库根目录运行 `go run ./design/cmd/tokenstudio`，打开 http://127.0.0.1:7790/ 。
- **手改 JSON**：编辑 `tokens.json`，然后运行 `go generate ./design`。

## 令牌调配台

调配台是一个本地页面，只监听回环地址。左侧编辑令牌，右侧实时预览：

- **组件预览**：使用控制台真实的 `web/app.css` 渲染工作台、徽标、拓扑、寄存器、字号与圆角，草稿改动立即生效。
- **语义颜色**：点击色块从色板中选色。如果这个色板颜色只被当前角色使用，可以直接跳到色板中调整；如果还被其他令牌共用，可以先复制出一个独立颜色，避免牵连其他角色。
- **色板**：编辑色值和名称，增删颜色。每个颜色都标出被哪些令牌引用；重命名时，引用会自动跟随。
- **字号与圆角**：调整设计尺寸和两端的缩放系数，并显示 Web 端的像素值和桌面端的 sp 值。
- **对比度**：底栏实时检查全部对比度规则（与 `TestContrastRules` 是同一份 `design.ContrastRules`）。点击某条规则，会跳到对应颜色。
- **保存并生成**（Ctrl+S）：服务端校验通过后，写入 `tokens.json`（固定格式，改一个令牌只产生一行 diff），并重新生成全部令牌文件。另外支持撤销、重做和放弃修改。

语义颜色、字号和圆角的**名称**会被代码引用，调配台中只能改值；如需增删，请直接编辑 `tokens.json` 并同步修改代码。

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

字号令牌是设计尺寸，各端渲染尺寸 = 设计尺寸 × 密度（`density`）。Web 的密度是 `--k = 1.12`（正文 14px），紧凑布局时调整 `--k`；Gio 端的密度是 1.04，结果四舍五入到 0.5sp。

| 令牌 | 设计尺寸 | 用途 |
|---|---|---|
| `micro` | 10.5 | 拓扑说明、键位提示 |
| `caption` | 11 | 表头、徽标、分组标题、状态栏 |
| `small` | 11.5 | 等宽数值、标签页、字段错误 |
| `body` | 12.5 | 默认文本 |
| `title` | 16 | 面板与对话框标题 |
| `display` | 18 | KPI 数字 |

CSS 中不要再写裸像素字号。裸像素字号不随 `--k` 缩放，紧凑布局时会失真。

### 圆角

`xs` 3（键位提示）· `sm` 6（按钮、输入框、横幅）· `md` 8（分组框、KPI 卡）· `lg` 10（拓扑节点、对话框）· `pill`（徽标、分段控件；Gio 的 `rounded` 会把它收敛为半高）。

### 阴影与遮罩

只有两级阴影：`shadow-sm`（凸起的分段按钮、拓扑节点）和 `shadow-lg`（对话框）。遮罩只有两种：`scrim` 用于对话框，`scrim-strong` 是窗口过小时的阻断层。

## 组件清单

新增组件前先确认下列组件无法满足需求：

| 组件 | Web 类名 | Gio | 需要覆盖的状态 |
|---|---|---|---|
| 按钮 | `.btn` / `.btn.pri` | `smallButton` | 悬停、禁用、焦点 |
| 徽标 | `.badge(.ok/.warn/.err)` | `badge` | — |
| 状态点 | `.dot(.warn/.err)` | — | — |
| 表单字段 | `.field` + `.field-err` | — | 焦点、无效、只读、禁用 |
| 表格 | `table.t`、`tr.bad` | `table.go` | 选中、失败行 |
| 分段控件 | `.seg` | — | 选中 |
| 分组框 / KPI | `.box` / `.kpi` | — | 数据过期 |
| 树节点 | `.node` | 网关侧栏 | 悬停、选中、焦点 |
| 标签栏 | `.dock-tabs` / `.tabs` | — | 选中 |

## 守卫

以下检查都在根模块中，CI 的 `go test ./...` 会运行：

- `TestGeneratedUpToDate`：生成文件与 `tokens.json` 不一致时失败。
- `TestNoRawColors`：`web/app.{css,js}`、`index.html`、`desktop/stopped.*` 和 `desktop-native/internal/ui` 中出现裸色值（`#rrggbb`、`rgba(`、`rgb(0x…)`）时失败。
- `TestContrastRules`：对比度规则未达标时失败。
- `TestFileRoundTrip`：`tokens.json` 不是固定格式时失败（运行 `go generate ./design` 即可修正）。
- `tokens.json` 本身会校验：命名必须是 kebab-case、不允许未知字段、色板条目必须被引用。

视觉回归方面，Web 端有 `e2e/` 中的布局溢出测试，Gio 端可运行 `UI_SNAPSHOT_DIR=… go test ./internal/ui -run Snapshot` 渲染截图。

## 尚未收敛

- **间距**：现有 CSS 的内边距不完全在 4px 网格上，暂未令牌化。新代码使用 4 的倍数（2px 仅用于细微对齐）。
- **布局尺寸**：控件高度和行高（26/24/38 × `--k`）仍写在 `app.css` 中，属于布局几何，不在令牌里。
- **Gio 端部分尺寸**：如品牌字样 14sp、按钮内圆角 5dp，仍是局部常量。
- **文档站**：是否并入本规范尚待决定，目前仍使用 Tailwind zinc 色板。
