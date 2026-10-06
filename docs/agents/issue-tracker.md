# Issue tracker: Local Markdown

用户于 2026-10-05 选择仓库内 Markdown。远程 GitHub 仓库不作为这些技能的默认发布目标。

- 一个功能一个目录：`.scratch/<feature-slug>/`。
- PRD：`.scratch/<feature-slug>/PRD.md`。
- 实现任务：`.scratch/<feature-slug>/issues/<NN>-<slug>.md`，从 01 开始。
- 状态：文件顶部 `Status:`，值见 triage-labels.md。
- 评论和后续讨论：追加到票据底部 `## Comments`。
- “发布到 issue tracker”即写入上述目录；“读取票据”即读取用户指定的本地文件。
- 外部 PR 不作为此本地 tracker 的需求入口。
