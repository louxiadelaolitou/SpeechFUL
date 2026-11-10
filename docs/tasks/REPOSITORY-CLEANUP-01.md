# REPOSITORY-CLEANUP-01 — 文档入口与历史维护材料整理

- 模式：常规；负责人／状态维护人：当前 Session。
- 分支／Worktree／PR：`codex/docs/repository-entry-cleanup`；`D:/SpeechFUL`；未创建 PR。实际基线为本地 `origin/main` 与 `main` 一致的 `3db6fac2351d3fe4d3824aeca5aa0cbdb562693a`，开始时工作区干净。此前提交因缺少 Git 作者身份未生成；用户配置后，本轮已核实作者与提交者身份，按明确要求保存本地提交。
- 授权依据：用户要求“先处理文档入口和新旧冲突，再清理确认多余的文件，不要改变现有业务行为”；补充包含虽有引用但缺乏维护价值的内容。Q1 明确选择“按建议清理，历史通过 Git 追溯”。允许更新引用、删除被替代的历史材料及仅服务旧快照的检查；不删除当前安全、数据库升级和设备行为覆盖。随后明确要求本地提交、让整理版本成为 main，并追加“推送到 GitHub”。按连续指令同步本地 main 至 `origin/main`，只作正常快进推送，不强制覆盖，不扩大到正式部署。
- 需求确认：当前术语以已确认的校邮重置、暂停恢复码／Passkey 和“分类”规则为准，不重新定义产品行为。依据为 [AUTH-EMAIL-RESET-01](AUTH-EMAIL-RESET-01.md)、[PROFILE-SETTINGS-01](PROFILE-SETTINGS-01.md) 与现有 `GLOSSARY.md`。历史资料使用上述固定基线追溯，禁止改写旧 PASS、摘要和回执。无未决取舍。
- 更新日期：2026-11-10。
- 状态：实现 已完成；验证 PASS（本轮文档、依赖与协作检查；扩展工具全套有 1 项环境 BLOCKED，见下）；合并 main 已快进整合整理提交 `711f809` 及任务状态补充；GitHub 同步按本次授权执行，远端位置以 `origin/main` 核对；发布 未发布（仓库同步不等于部署）。

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
