# Engineering Standards

本仓库采用的外部工程规范。规范正文以固定版本为准；升级版本须单独提交并同步更新本文件与检查入口。

## Git 提交信息

- 规范：[ffutop/engineering-handbook `standards/git/commit-messages.md`](https://github.com/ffutop/engineering-handbook/blob/5fb2cb512c242f9ac8197384d8053bb8250f7794/standards/git/commit-messages.md)
- 固定版本：`5fb2cb512c242f9ac8197384d8053bb8250f7794`
- 既有规则：仓库此前无书面提交规范；历史提交稳定使用 Conventional Commits 单行英文标题（如 `feat: add shared simulations and injector downstreams`）。
- 采用格式：规范第 4 节默认格式 `<type>[(scope)][!]: <description>`，描述使用英文、祈使语气、小写开头、句末无句号。`scope` 仅在有助于定位时使用，取值参考包路径（如 `api`、`config`、`rtu`、`simulation`）。
- 始终适用：单行，不含正文、空行或脚注；任何位置不得出现 `Co-Authored-By`（不区分大小写）。
- 豁免：`75dd282` 及其之前的提交不追溯、不改写。
- 检查入口：
  - 本地：`git config core.hooksPath .githooks` 启用 `.githooks/commit-msg`；也可手动运行 `scripts/check-commit-msg.sh`。
  - CI：`.github/workflows/commit-messages.yaml` 检查 PR 内全部非合并提交、PR 标题（squash 合并默认使用），以及推送到 `main` 的提交。
- 评审负责人：仓库维护者（@ffutop），负责确认描述与实际变更一致及破坏性变更 `!` 标记。
