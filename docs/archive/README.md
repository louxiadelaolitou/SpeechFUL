# 历史资料追溯

当前开发从[文档入口](../README.md)开始；当前工作状态由[具体任务](../tasks/)维护。

## 后续全仓核对的退役范围（2026-11-10）

固定旧机器的两个迁移清理器、旧图稿渲染器、旧私信交接，以及确认无现行执行依赖的阶段 JSON 和旧认证交互稿，已从当前树退役。它们从[本轮清理前的 e26d453](https://github.com/zewbby/SpeechFUL/tree/e26d45375d6526bed72d00fd5fe42c26cc6d60ec)追溯，精确清单以[清理任务](../tasks/STALE-FILE-CLEANUP-20261110.md)及其提交差异为准；保留的 JSON 不改写其原字节或历史 PASS。

本轮同时退役未进入运行链路的 sqlc 配置和重复查询源；实际 pgx 查询、45 个正式迁移、11 个授权／审计 SQL、正常功能测试和有校验依赖的 `original/` 字节归档保留。下方“原始 JSON 保留”描述前一轮范围，不表示阶段回执须永久留在当前树。旧认证设计统一从[历史入口](../design/auth-flow-final/README.md)阅读。

## 前一轮退役的过程文档（2026-11-10）

清理前的完整文档固定在
[`50baa3c2768b69188f2d6562cdb8b6c51e9fa674`](https://github.com/zewbby/SpeechFUL/tree/50baa3c2768b69188f2d6562cdb8b6c51e9fa674/docs)。
本轮删除 267 个 Markdown 文件，使 `docs` 从 406 个减至 139 个；保留的引用直接指向该提交中的原文，历史无需在当前目录再复制一份。

| 退役内容 | 数量 | 历史入口与保留边界 |
| --- | ---: | --- |
| 旧分支说明、交付和合并回执 | 77 | [原 branches](https://github.com/zewbby/SpeechFUL/tree/50baa3c2768b69188f2d6562cdb8b6c51e9fa674/docs/branches)；当前状态由任务维护 |
| 早期讨论、逐轮问答和图稿说明 | 45 | [原 discussions](https://github.com/zewbby/SpeechFUL/tree/50baa3c2768b69188f2d6562cdb8b6c51e9fa674/docs/discussions)；当前模块规则与来源链接保留 |
| 逐轮 UI 评审与出图提示 | 36 | [原 ui-reviews](https://github.com/zewbby/SpeechFUL/tree/50baa3c2768b69188f2d6562cdb8b6c51e9fa674/docs/design/ui-reviews)；图片、实际组件和 UI 工作流保留 |
| 阶段执行、换机交接、历史验证与专项报告 | 108 | [原 design](https://github.com/zewbby/SpeechFUL/tree/50baa3c2768b69188f2d6562cdb8b6c51e9fa674/docs/design)；现行契约、架构、威胁模型、回退手册和原始 JSON 保留 |
| 协作规范最初落地过程 | 1 | [原落地记录](https://github.com/zewbby/SpeechFUL/blob/50baa3c2768b69188f2d6562cdb8b6c51e9fa674/docs/engineering/collaboration-rollout.md)；当前规则见 AGENTS 与 CONTRIBUTING |

本地可用 `git show 50baa3c2768b69188f2d6562cdb8b6c51e9fa674:<原路径>` 查看任一原文。
删除清单以本轮提交的差异为准，验证和交付见[整理任务](../tasks/REPOSITORY-CLEANUP-01.md)。这次清理不删除 Git 历史，不重写原始证据中的版本、路径、摘要或 PASS／FAIL。

## 前一轮已退役的材料

前一轮整理前的完整仓库固定在
[`3db6fac2351d3fe4d3824aeca5aa0cbdb562693a`](https://github.com/zewbby/SpeechFUL/tree/3db6fac2351d3fe4d3824aeca5aa0cbdb562693a)。
在任意本地完整克隆中可用 `git show 3db6fac2351d3fe4d3824aeca5aa0cbdb562693a:<原路径>` 读取；不要在当前运行目录恢复旧部署或执行旧清理脚本。

- [旧术语 CONTEXT](https://github.com/zewbby/SpeechFUL/blob/3db6fac2351d3fe4d3824aeca5aa0cbdb562693a/CONTEXT.md)：当前定义已集中到根 `GLOSSARY.md`。
- [HANDOFF](https://github.com/zewbby/SpeechFUL/blob/3db6fac2351d3fe4d3824aeca5aa0cbdb562693a/docs/design/HANDOFF.md)、[progress](https://github.com/zewbby/SpeechFUL/blob/3db6fac2351d3fe4d3824aeca5aa0cbdb562693a/docs/design/progress.md)：旧多轮进度，不再作为新会话指令。
- [模块交接](https://github.com/zewbby/SpeechFUL/blob/3db6fac2351d3fe4d3824aeca5aa0cbdb562693a/docs/design/module-acceptance-handoff.md)、[验收台账](https://github.com/zewbby/SpeechFUL/blob/3db6fac2351d3fe4d3824aeca5aa0cbdb562693a/docs/design/module-acceptance-ledger.json)：旧提交与旧设备的验收边界，不是当前通过清单。
- 同一提交中的 `tools/check-identity-handoff.py`、`check-privacy-closure-handoff.py`、`check-non-ios-closure.py`、两个 `check-android-b5*-evidence.py` 及配套收集／写台账脚本只校验或维护上述旧快照，随之退役。它们没有替代当前 Go、Flutter、SQL 或设备行为测试。
- [F1 页面计划](https://github.com/zewbby/SpeechFUL/blob/3db6fac2351d3fe4d3824aeca5aa0cbdb562693a/docs/design/community-flutter-pages-plan.md)、[出图清单](https://github.com/zewbby/SpeechFUL/blob/3db6fac2351d3fe4d3824aeca5aa0cbdb562693a/docs/design/ui-reviews/f1-completion-audit.md)以及 F2 的设计、接口／依赖锁、验证记录、专用检查和本地 SQL 快照：只描述早期文字切片，不再约束后续功能或当前存储。实际行为继续由现行源码、正式迁移、契约和行为测试验证。
- 导航设计明确废弃的 v7／v8 六张分类列表图、v8 评审以及取消的“删除后独立页面”图：原始资产仍在上述固定提交中；当前文档引用改为历史链接。保留的 v9 列表图同时供“发布返回”流程引用，不再复制同一图片。

旧报告和 JSON 中记录的路径、命令、SHA 与 PASS／FAIL 保留原意，应结合其被测提交解读。退役清单与依赖核对见[整理任务](../tasks/REPOSITORY-CLEANUP-01.md)，不再维护新的逐文件摘要台账。

## 已退役的一次性验收脚本

以下脚本从当前树删除，原始字节保留在固定提交 `fbf5e6be302432db8a187a6293403b88be454bc7`：

- [2026-10-15 Passkey 实验执行文件](https://github.com/zewbby/SpeechFUL/tree/fbf5e6be302432db8a187a6293403b88be454bc7/tools/community-android-stage3/execution-20261015)：固定当次路径和一次性夹具的 8 个脚本。
- [P5-D1 观测目录](https://github.com/zewbby/SpeechFUL/tree/fbf5e6be302432db8a187a6293403b88be454bc7/tools/comment-snapshot-process)：五个 `p5d1_*.py` 单次实验脚本；当前 campaign 和功能／性能 runner 保留。
- [图片媒体旧任务工具](https://github.com/zewbby/SpeechFUL/tree/fbf5e6be302432db8a187a6293403b88be454bc7/tools/post-image-media)：仅退役写死旧结果的 `record_evidence.py` 和旧任务清理器 `cleanup_owned.py`，保留协议生成工具。
- [旧 F3 wrapper](https://github.com/zewbby/SpeechFUL/blob/fbf5e6be302432db8a187a6293403b88be454bc7/tools/run-community-flutter-f3-linux.sh)：已由当前 Flutter 直接命令和 F4 集成 runner 替代。

此次没有删除正常功能测试或现行 CI，也没有执行旧脚本。范围与验证见[清理任务](../tasks/TEST-RUNNER-CLEANUP-20261110.md)。旧 JSON 的路径和摘要描述其原版本，保持原字节。

## 仍保留的历史来源

- [早期讨论](../discussions/README.md)：产品决定来源，不是当前状态。
- [旧分支记录](../branches/README.md)：对应任务与交付时点，不代表现在仍未合并。
- [文字帖原始归档](community-text-posting-20261008/README.md)：有独立原始字节与契约校验，保持其格式。
- 本轮退役的报告与评审通过上方固定提交阅读；当前目录仍保留的 JSON、截图和有校验依赖的原始资料按各自版本解释。当前规则入口见[设计索引](../design/README.md)。
