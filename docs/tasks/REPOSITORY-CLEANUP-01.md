# REPOSITORY-CLEANUP-01 — 文档入口与历史维护材料整理

- 模式：常规；负责人／状态维护人：当前 Session。
- 分支／Worktree／PR：本轮 `codex/docs/prune-historical-docs-20261110`；`E:/Hnuhole-env/source/SpeechFUL-docs-prune-20261110`；[PR #129](https://github.com/zewbby/SpeechFUL/pull/129)。初始基线为 `origin/main` 的 `fbf5e6be302432db8a187a6293403b88be454bc7`，独立 Worktree 开始时干净；期间 main 合入 runner／tools 清理，已整合 `50baa3c2768b69188f2d6562cdb8b6c51e9fa674`，最终差异以该 main 为准。前轮分支为 `codex/docs/repository-entry-cleanup`，基线 `3db6fac2351d3fe4d3824aeca5aa0cbdb562693a`，已同步 GitHub。
- 本轮授权与需求确认：用户询问 main 的 docs 为何有大量 Markdown、哪些不需要，随后明确“弄好以后直接清了合并上去就行”。本轮据此清理过程文档、修复历史引用、提交、推送、创建 PR 并在检查通过后合并；不涉及正式部署。现行业务规则和验收门槛不变，原文通过固定 Git 提交保留，无新增产品取舍。
- 授权依据：用户要求“先处理文档入口和新旧冲突，再清理确认多余的文件，不要改变现有业务行为”；补充包含虽有引用但缺乏维护价值的内容。Q1 明确选择“按建议清理，历史通过 Git 追溯”。允许更新引用、删除被替代的历史材料及仅服务旧快照的检查；不删除当前安全、数据库升级和设备行为覆盖。随后明确要求本地提交、让整理版本成为 main，并追加“推送到 GitHub”。按连续指令同步本地 main 至 `origin/main`，只作正常快进推送，不强制覆盖，不扩大到正式部署。
- 需求确认：当前术语以已确认的校邮重置、暂停恢复码／Passkey 和“分类”规则为准，不重新定义产品行为。依据为 [AUTH-EMAIL-RESET-01](AUTH-EMAIL-RESET-01.md)、[PROFILE-SETTINGS-01](PROFILE-SETTINGS-01.md) 与现有 `GLOSSARY.md`。历史资料使用上述固定基线追溯，禁止改写旧 PASS、摘要和回执。无未决取舍。
- 更新日期：2026-11-10。
- 状态：本轮实现 已完成；本地验证 PASS，最终 head 远端结果见 [PR Checks](https://github.com/zewbby/SpeechFUL/pull/129/checks)；合并 [PR #129 实时状态](https://github.com/zewbby/SpeechFUL/pull/129)；发布 未发布。前轮已完成并同步 main，原扩展工具缺依赖记录保留在下方。

## 本轮：精简 docs 过程文档

初查 main 共有 404 个 docs Markdown，期间 runner／tools 清理新增 2 份任务记录；整合后的 main 基线为 406 个。本轮删除 267 个，最终保留 139 个：旧分支记录 77、早期讨论 45、逐轮 UI 评审 36、阶段执行／交接／专项报告 108、最初协作规范落地过程 1。删除项逐文件定位到基线原件，源码、脚本及 CI 无路径或文件名调用；不能仅以“有文档引用”作为继续保留过程副本的理由。

保留现行模块规则、协议契约、架构／ADR、威胁模型、运行与回退手册、任务记录；特别保留评论快照一致性设计和 main 默认配置说明。文字帖 `original/` 的 19 份固定字节原件及 manifest 有现行契约校验，不删。其余原始 JSON、截图、源码、测试、迁移及 workflow 也不改动。

剩余 Markdown 中指向删除文档的链接改为 `50baa3c` 完整 SHA 下的原文，保留 main 新增的工具退役注释；分支和讨论 README 改为简短历史入口，修正已失效的“README 分支一览”维护规则。CONTRIBUTING 明确同一任务集中记录，不按分支、问答轮次或修复尝试另建过程文档。删除清单以本轮 Git diff 为准，统一历史入口见 [archive](../archive/README.md)。

本轮还标明未随当前仓库交付的旧图／prompt 文件，移除失效链接，修正编辑迁移的当前链接（0030，原历史编号叙述不改）。UI 评审工作流及项目技能的复审模板改为复用任务记录，避免继续按页按轮生成过程 Markdown；技能名称、发现方式、产品规则和其他资源不变。

初次实现提交为 `dd2ed0649094d8d23a85515c6478b0b49b3776f6`。整合后的本轮验证绑定 `50baa3c2768b69188f2d6562cdb8b6c51e9fa674`＋本轮完整 Markdown 差异（由本分支整合提交承载），环境为 Windows PowerShell／Python 3.9.13。验证之后仅更新任务证据不使原检查失效；提交前仍核对暂存差异。

| 本轮验证 | 命令与范围 | 结果 |
| --- | --- | --- |
| 文档与历史链接 | 一次性 Python 路径扫描／Git `cat-file --batch-check`；检查器保存在任务自有 `E:/Hnuhole-env/tmp/docs-prune-verify-20261110.py` | PASS：1034 个本地链接无缺失；169 个固定提交原文／目录对象可读取；267 个删除项均属于固定基线 |
| 删除调用与行为边界 | 扫描已跟踪源码、脚本和 workflow 的删除路径及 basename；`git diff --name-only` | PASS：无调用；全部变更均为 Markdown，源码、现行测试、迁移、CI、原始 JSON、图片和 `original/` 字节不变 |
| 协作回归 | `python -B -X utf8 -m unittest discover -s tools/tests -p test_collaboration.py -v` | PASS：19 项 |
| 契约与原始归档 | `python -B -X utf8 -m unittest discover -s tools/tests -p test_community_text_contract.py -v` | PASS：10 项，含原始 Git blob 字节与 manifest 完整性 |
| 项目 UI 技能 | `quick_validate.py .agents/skills/hnuhole-ui-craft`；人工核对复审模板与 CONTRIBUTING 一致 | PASS；仅修改复审记录位置 |
| 差异与暂存 | `git diff --check`；`python -B -X utf8 tools/check-collaboration.py --staged` | PASS；最终整合提交前复核暂存范围，远端结果见 PR policy |
| 产品运行验收 | 业务及运行文件未改变 | NOT_RUN：不启动数据库、服务或设备，不重新宣称 SQL、真机、性能或生产验收通过 |

本轮 PR 的最终 head、CI 和合并 SHA 由关联 PR 实时记录，不为回写自身合并再创建文档 PR。

## 前轮记录（保持原验证范围）

## 目标与范围

建立从根 README 到代码、开发、测试、运行和历史资料的清楚入口；消除当前说明与旧分支快照争夺权威的问题。清理重复术语定义、被任务记录替代的巨型进度台账及其专用元数据检查、已退役的 F1/F2 设计锁和 SQL 快照、明确废弃或重复的设计资源。业务源码、正式迁移、协议向量、当前行为测试和产品 CI 保持不变。

## 验收标准

- 新读者从 `docs/README.md` 能找到前后端入口、运行方式、验证分层、当前需求来源与历史追溯。
- 当前术语与认证说明不再把已暂停的恢复码／Passkey 当作开放流程，不再声称校邮不能登录或重置旧号。
- 删除项均已确认用途及调用闭包；剩余 Markdown 不含指向删除项的本地链接，旧证据原始摘要不改写。
- 当前业务源码、SQL、锁文件、行为测试及 CI 相对基线无差异；执行文档链接、协作检查和适用工具回归。

## 当前进度与证据

| 项目 | 被测版本／环境 | 结果与证据 |
| --- | --- | --- |
| 调查 | Windows PowerShell；基线 `3db6fac` | 已核对分支、远端、Worktree、现有改动和用户确认来源；仅本地整理 |
| 实现 | `3db6fac`＋本任务承载的最终差异 | 完成文档导航、认证／术语冲突修正、历史依赖退役与链接替换；29 个删除项原 Git blob 共 14,627,658 字节（13.95 MiB），不是清理 Git 历史后的仓库体积 |
| 文档与历史定位 | 本轮最终 Markdown；一次性 Python 路径检查及 `git cat-file -e 3db6fac:<path>` | PASS：806 个本地链接、20 个固定历史路径；新入口的“开发与验证”锚点已核对；不以联网检查代替本地对象验证 |
| 删除依赖闭包 | 对全部剩余 `.py/.sh/.ps1/.go/.dart/.yml/.yaml` 扫描删除路径与 basename | PASS：无运行源码／CI 对删除文件的调用；保留 JSON 的历史路径与 SHA 不改写 |
| 行为不变 | `git diff --name-only 3db6fac` 核对 `apps/services/packages/infra/.github` 非 Markdown 文件及原始归档 | PASS：业务源码、当前行为测试、正式迁移、权限 SQL、协议向量、锁文件、CI 和固定字节归档无差异；唯一删除 SQL 是已退役的文档设计快照 |
| 协作与空白 | `python -B -X utf8 -m unittest discover -s tools/tests -p test_collaboration.py -v`；`git diff --check`；`python tools/check-collaboration.py --staged` | PASS：19 项协作测试；差异空白检查；暂存规则检查在提交前执行 |
| 扩展工具回归 | Windows／Python 3.9.13；`python -B -X utf8 -m unittest discover -s tools/tests -v` | 197 PASS、3 SKIP（POSIX 权限要求）；1 BLOCKED：`test_nullable_fields_match_actual_openapi30_validator` 缺少 `openapi_schema_validator`，命令 exit 1。不修改测试或全局 Python 来绕过缺依赖，不记录为全套 PASS |
| 产品运行验收 | 未改变产品实现 | NOT_RUN：本轮不启动服务、数据库或设备，不重新宣称历史 SQL／真机结果通过 |

## 删除依据与保留边界

| 删除组 | 文件与依据 |
| --- | --- |
| 重复当前术语 | `CONTEXT.md`；仍适用的私有用户名、资格、隐私和治理术语并入 `GLOSSARY.md`，已变更规则按已确认任务修正 |
| 被任务记录替代的进度 | `HANDOFF.md`、`progress.md`、`module-acceptance-handoff.md`、`module-acceptance-ledger.json`；共四份多代快照，保留固定 Git 入口，不维护第二份当前状态 |
| 旧台账专用工具 | `check-identity-handoff.py`、`check-privacy-closure-handoff.py`、`check-non-ios-closure.py`、两个 `check-android-b5*-evidence.py`、`collect-android-b3-b4-acceptance.py`、`post-image-media/finish_handoff.py` 及 `test_non_ios_closure.py`；仅维护或校验旧元数据，调用闭包内一起退役，无现行 CI 引用 |
| 早期设计阶段锁 | F1 页面计划与出图清单；F2 设计、lock、verification、`check-community-flutter-f2.py`、`schemas/community-posts-local-v1.sql`；固定早期 14 接口与三表快照，不是当前运行或升级输入，现行 CI／源码无调用 |
| 明确废弃图稿 | 分类列表 v7／v8 六图、v8 评审、`post-detail-deleted-v1.png`；依据原 `modules/navigation.md` 明确废弃 v7／v8，以及 `ui-reviews/post-detail-states-v1.md` 的 WITHDRAWN／NO_SEPARATE_PAGE |
| 精确重复图片 | `post-publish-return-channel-v2.png` 与 `channel-feed-populated-v9.png` 字节相同；三个文档引用改用后者 |

共删除 29 个文件。原始内容从 `3db6fac:<原路径>` 可查；保留的历史 JSON 摘要和结果不改写，旧 Markdown 链接改为固定提交。未将“有引用”作为保留旧工具的理由，而是核对其是否验证当前行为。

仍保留两份字节相同的 B5 设备 fixture、两个三行 driver 和注册 v2/v3 图片：它们属于仍保留的设备 runner／版本化证据输入，不能仅凭重复判定覆盖等价。当前 Go／Flutter／原生行为测试、真实 SQL runner、C/V 正式迁移和权限审计不删。

## 阻碍与下一步

本轮必要文档、依赖与协作验证完成。扩展工具回归的缺依赖单列，不影响已确认未变化的业务差异结论；完整工具全绿需在具备原规范校验依赖的环境复验。

用户随后要求让整理版本成为 main，核对工作区干净、原 main `3db6fac` 是 `711f809` 的祖先后已完成本地快进整合。用户再明确要求推送到 GitHub；实际 fetch 后确认 `origin/main` 仍为 `3db6fac`，本地无分叉。按授权使用 `git push origin main:main` 同步，并核对远端 main 与本地 HEAD 一致，不作强制推送。任务状态补充均在任务分支提交后快进 main；没有正式部署。后续代码职责拆分与 runner／CI 重构另按具体范围实施。
