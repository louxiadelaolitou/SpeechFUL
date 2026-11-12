# release-notes-policy-20261112 — Release 正文仅写更新内容

- 模式：常规；负责人／状态维护人：当前 Session。
- 分支／Worktree：`codex/docs/release-notes-policy`；`E:/Hnuhole-env/source/SpeechFUL-release-notes-policy`；基线 `c3307e288f79c525af2ddbb5214dbd0b7a0c3e52`。
- 授权依据与边界：用户明确要求把已确认规则写进 CONTRIBUTING.md，然后提交、推送、合并，后续发布遵循该规则；本轮只改文档。
- 需求确认：Release 正文仅列用户可感知的新增、调整和修复，简洁中文、每项一句；安装、版本元数据、提交 SHA、构建测试及内部过程不进入正文，证据保留在任务记录与 CI。无未决取舍。
- 更新日期：2026-11-12。
- 状态：实现已完成；验证见下表；合并以本任务 PR 实时状态为准；产品发布不适用。

## 目标与范围

在 CONTRIBUTING.md 的交付章节增加“GitHub Release 说明”规则，供后续版本发布遵循。现有 v1.1.1 正文已按此精简，本轮不改发布附件或产品代码。

## 验收标准

- 规则明确限定正文的内容、语言和粒度，并说明工程证据去向。
- 规则通过 PR 合入 main。

## 当前进度与证据

| 项目 | 被测版本／环境 | 结果与证据 |
| --- | --- | --- |
| 文档核对 | 基线 c3307e2 + 本任务 CONTRIBUTING.md 新增段落；Windows | PASS，与用户确认的原文一致，无新增链接 |
| 差异与协作检查 | 同一文档差异；`git diff --check`；`python tools/check-collaboration.py --staged` | PASS，本地检查通过；远端结果以本任务 PR 当前 head 的 Collaboration CI 为准 |
| 产品测试 | 纯文档规则 | SKIP，不涉及产品代码 |

## 阻碍与下一步

提交并推送文档分支，创建 PR；适用 CI 通过后按用户授权合并。提交与最终合并回执由同一 PR 保留，不另建状态文档 PR。
