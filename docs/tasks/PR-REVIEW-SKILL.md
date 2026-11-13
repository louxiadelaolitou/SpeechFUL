# PR-REVIEW-SKILL — 发布仓库 PR 审查技能

- 模式：常规
- 负责人／状态维护人：当前 Session
- 分支／Worktree／PR：`codex/docs/pr-review-skill`；独立 Worktree；用户要求进入 main；直接推送被仓库规则拒绝，改为通过 PR 集成。
- 实际基线：`9937638c077bf2da7d51daa5de13a83645dc4320`，`zewbby/SpeechFUL` 的 main。
- 授权依据与边界：用户要求“这个skill直接推到GitHub上的main分支对应位置”，并确认当前仓库为 SpeechFUL；仅发布本次技能与必要任务记录，不涉及产品部署。
- 需求确认：复用本会话已创建的 PR-review 技能，放在 `.agents/skills/pr-review/`；无未决取舍。
- 更新日期：2026-11-13
- 状态：实现 已完成；验证 PASS（本地格式与协作检查）；合并 未合并（待 PR）；发布 未发布（不涉及产品发布）。

## 目标与范围

将 PR-review 技能与 UI 元数据加入仓库，包含需求依据、源与目标版本固定、目标分支前进后的合并兼容性、增量复查、缺陷举证与验证记录要求。

## 验收标准

- 仓库技能与本次创建的个人技能内容一致，可使用 `$pr-review` 调用。
- Skill 格式、差异空白和仓库协作检查通过。
- 经普通快进推送进入 SpeechFUL main，不强推，不夹带其他工作。

## 当前进度与证据

| 项目 | 被测版本／环境 | 结果与证据 |
| --- | --- | --- |
| 实现 | 上述基线加本提交的两个技能文件 | 复制个人技能，内容一致；本提交正文引用本记录 |
| Skill 格式 | Windows / Python；`quick_validate.py .agents/skills/pr-review` | PASS |
| 差异与协作检查 | 同一基线与差异；`git diff --cached --check`；`python tools/check-collaboration.py --staged` | PASS |
| 产品构建／设备验收 | 纯技能文档变更 | SKIP，不涉及产品代码 |
| main 集成 | 普通推送与远端 SHA 核对 | 本提交及远端 main 为交付证据；最终推送结果见当前会话 |

## 阻碍与下一步

GitHub 以 GH013 拒绝直接推送：Changes must be made through a pull request。保留 main 保护规则，通过 PR 集成；合并前检查适用 CI 与审批条件。
