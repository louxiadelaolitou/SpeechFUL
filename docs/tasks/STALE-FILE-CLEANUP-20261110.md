# STALE-FILE-CLEANUP-20261110 — 过期文件与历史远端分支退役

- 模式：常规；负责人／状态维护人：当前 Session；无委派。
- 分支／Worktree／PR：`codex/chore/prune-stale-repository-files`；独立 Worktree `SpeechFUL-stale-cleanup-20261110`；实际基线为 fetch 后的 `origin/main`：`e26d45375d6526bed72d00fd5fe42c26cc6d60ec`；[PR #130](https://github.com/zewbby/SpeechFUL/pull/130) 已合并，远端工作分支已退役，本机分支／Worktree 保留。
- 授权依据与边界：用户在全仓核查后明确“清了，直接合并推送就行”，并追加“把 GitHub 上的分支，已经完成的属于历史分支的全退役了，同步文档也要更新了”。允许确认过期文件退役、必要入口／文档修复、验证、推送合并和已完成历史远端分支删除。不部署，不操作业务数据、数据库或设备，不删除本机分支／Worktree，不混入另一项未提交的移动端重构。
- 需求确认：正常功能测试、正式 SQL 迁移／权限审计、现行协议与固定原始字节归档保留。历史回执和旧设计以固定 Git 提交追溯；不能仅凭日期或无字面引用删除，执行依赖闭包仍须复核。沿用当前 pgx 手写查询，退役无生成调用的 sqlc 配置及重复查询源；不变更运行 SQL。分支以 main 包含的提交或已合并 PR 精确 head 为依据；额外文档回执先核对／归集，未完成工作保留。
- 更新日期：2026-11-10。
- 状态：文件实现 已完成；本地及远端适用验证 PASS（Windows 工具 3 SKIP；远端 dispatch 专用长测 1 SKIP）；PR130 已合并；历史远端分支退役 已完成，共 64 条；发布 未发布（本任务无需部署）。

## 范围与依赖

全仓核查基线包含 2108 个文件、57 个 SQL、383 个常规命名测试和 139 个 docs Markdown。退役旧机器清理／旧图稿渲染器、旧私信交接、无执行依赖的阶段 JSON 及已被当前流程替代的认证设计资产；修正 API／近期工作说明和 F4 镜像缺少 native_media 的问题。保留 45 个正式迁移和 11 个现行授权／审计 SQL，所有正常 Go／Flutter／原生测试及协议向量。SQLC 残留仅有配置到查询源的内部引用，当前查询常量和 pgx 调用不改。

分支清理初始快照为 GitHub 的 65 条远端分支和 129 个 PR。59 条分支头与已合并 main PR 的 head 精确一致；4 条需核对额外提交，1 条有未合并 PR，main 保留。删除前重新核对远端 SHA，使用期望 SHA 限定的引用删除，拒绝分支移动；不删除未合并产品改动。最终名单和结果在本记录追加。

## 验收标准

- 删除项不再被保留的可执行代码、CI 或工具导入；Markdown 历史引用可解析到固定基线，保留 JSON 的原始内容不改写。
- 正式迁移、权限 SQL、产品运行源码、协议／锁文件与正常功能测试保持原行为；两个重复 B5 场景不按字节相同直接删除。
- F4 镜像包含 mobile pubspec 的全部本地包，现有合成工具隔离回归通过；不冒充真实 Flutter／SQL／设备验收。
- 完成适用工具、契约、链接、空白和协作检查；远端当前 head 的适用 CI 通过后合并。
- 远端历史分支逐条有完成依据，额外文档成果不丢失；main、活跃任务和未合并工作保留，GitHub 结果复核后更新文档。

## 当前进度与证据

| 项目 | 被测版本／环境 | 结果与证据 |
| --- | --- | --- |
| 基线与隔离 | fetch 后 `e26d453`；Windows PowerShell | PASS：`D:/SpeechFUL` 有另一条重构的未提交文件，本任务使用独立 Worktree，未切换／清理它。 |
| 文件退役 | `e26d453`＋本任务最终差异 | 126 个文件：94 份阶段 JSON、26 个旧认证设计文件（含另 3 份 JSON）、4 个旧脚本／交接、sqlc 配置与重复查询源共 2 个。保留的历史 JSON 不改字节，旧设计保留简短 Git 入口，重写 51 个 Markdown 引用。 |
| 工具回归 | 同一实现差异；任务独立 Python 3.12 环境；`python -B -X utf8 -m unittest discover -s tools/tests -v` | PASS：201 项，198 PASS／3 SKIP（Windows 无 POSIX 权限语义），exit 0；全部正常测试保留。 |
| F4 runner | WSL2／hnuhole；`bash tools/tests/test-community-f4-runner-isolation.sh` | PASS：六类编排阶段、四次下层私有 socket／端口调用、非法端口／无界 timeout 拒绝、锁文件保留、HOME／SDK／遍历／符号链接／owner 隔离。使用合成工具，不启动 Go、PG、Flutter SDK 或设备。镜像与测试夹具补齐 native_media，CI 新增运行同一回归的 job。 |
| API 契约 | 独立验证环境；`python -B -X utf8 packages/openapi/check-specs.py` | PASS：13 份规范／契约；规范源和固定向量没有改动。 |
| 依赖与行为边界 | 一次性全树引用／AST／Git diff 验证；`e26d453`＋最终差异 | PASS：删除项无保留可执行引用；146 份 Python 可解析；383 个正常命名测试、45 个迁移及 11 个授权／审计 SQL、产品 Go／Dart／原生源码、向量和锁文件均保留原字节／Git 内容。两个 B5 场景因不同验收边界保留，没有按重复直接删测试。 |
| 文档与历史 | 全部非 original Markdown 本地链接及新增固定 Git 目标 | PASS：1093 个本地链接、32 个本轮历史目标，无新增断链；原始字节归档未更改。 |
| 分支调查 | GitHub 65 条分支及 129 个 PR 的快照 | PASS：59 条精确匹配已合并 PR head，另外 4 条只有额外／被 main 覆盖的文档；main 和 PR75 分支保留。删除前仍须重新核对 SHA，结果见最终回执。 |
| 远端适用 CI | PR130 head `b9d851a5640921233d5d1659ab311a4cb422f233`；[Checks](https://github.com/zewbby/SpeechFUL/pull/130/checks) | PASS：11 个 workflow 均 success；21 个检查中 20 success、actual-process-acceptance 按 workflow_dispatch 条件 SKIP。包含真实隔离 SQL、Dart→HTTPS、Flutter、Android 候选及迁移／恢复演练；新增 F4 隔离 job 通过。不冒充真机或生产验收。 |
| 合并 | [0d26f0b](https://github.com/zewbby/SpeechFUL/commit/0d26f0b4187985781e7e063db828e6f55d2f01fc) | PASS：核对 head 未移动、base 仍为 e26d453、所有检查完成、mergeable=true/clean 后按用户授权 Squash Merge。fetch 后核对合并 tree 与已测 b9d851a 完全相同；不自批或冒充 GitHub 成员独立批准。 |
| 远端分支退役 | 下表 63 条历史分支＋本次 PR130 工作分支 | PASS：删除前再次 ls-remote 校对；旧 63 条使用原子 push 和逐引用期望 SHA 删除，拒绝分支移动。PR130 合并后，按原 b9d851a head 删除其远端工作分支。Git ls-remote 与 GitHub API 均复核仅剩 main、docs/shared-rate-limit-deployment。未删除本机分支／Worktree。 |
| 交付文档／部署 | 本记录、branches 索引及 RECENT_WORK | 仅在已合并 main 上补充上述实际结果，继承同一被测实现，不重跑未改变的产品矩阵；最终提交的协作检查另核。部署 NOT_RUN，没有操作业务数据库或手机。 |

## 阻碍与下一步

本轮实现、验证、合并及分支退役已完成。PR75 的未合入 operator ACL 变更保留待其原任务处理；不推进部署或删除未完成工作。后续新分支按实际任务维护，本记录中的数量仅为本轮完成快照。


## 历史远端分支核对清单

下表 63 条历史分支均已按所列期望 SHA 删除；此外，本任务远端分支在 PR130 合并后按 head `b9d851a5640921233d5d1659ab311a4cb422f233` 删除，共 64 条。`docs/shared-rate-limit-deployment`／PR #75 保留：main 的 operator ACL 样例尚不含该 PR 的 `+setnx` 修正，本任务不替用户合入这项权限变更。`main` 和本机活动 Worktree 保留。

合并后回执：标签分支的原 `961020c` 文档已补入 `CUSTOM-TAGS-01`；Android `c1dd912` 的 PR113 最终回执已存在于 main；个人页 `ca0580b` 仅补记 PR68 已 squash 为 `ce140d57e0bf6feb27165352394c9946a1b72aef`、原 head `22498e8` 的 12 个 workflow 及原本地 20 项测试通过，无新增产品改动或部署。本段只归集原记录，不声称本轮重跑。官网 README 链接已在 main，早期 WEBSITE-01 状态已标明由 PR127 正式发布任务接替。

| 历史分支 | 删除前 head | 完成依据 |
| --- | --- | --- |
| `codex/chore/android-beta-launch-20261109` | `99fa83364574e47ee46a9a5dd2d09631b69e0ceb` | PR #126 已合并，head 精确相同 |
| `codex/chore/public-trial-access-01` | `ef23b87c9e1496f94dd297af58fdd3da77fe1d4f` | PR #116 已合并，head 精确相同 |
| `codex/chore/speechful-display-name` | `8e4e260418495e549a1dc2086707757bf9eac8ec` | PR #98 已合并，head 精确相同 |
| `codex/chore/tools-cleanup-20261110` | `6a0820692efc422ce803117219b9800f2a4f30a4` | PR #128 已合并，head 精确相同 |
| `codex/chore/v1-runtime-passkey-align` | `c4210ce44cf3a21c9e026a510878b0ad34fd8d50` | PR #119 已合并，head 精确相同 |
| `codex/docs/ai-coding-workflow` | `9cea9ced4a20732d1f535476b64852944699d87b` | PR #90 已合并，head 精确相同 |
| `codex/docs/feed-refresh-merge-receipt` | `5e6e07e984fb767cf38669a9c366bc9f35d96e17` | PR #70 已合并，head 精确相同 |
| `codex/docs/feed-scroll-merge-receipt` | `06bd55c6c7fbc11c334c048c2bc88eaf61b37b8b` | PR #67 已合并，head 精确相同 |
| `codex/docs/media-renewal-merge-receipt` | `9c300b35d2f603e8aec903cb2b7d266568405c50` | PR #78 已合并，head 精确相同 |
| `codex/docs/post-detail-comments-merge-receipt` | `ad8fae5fa98d105073b7776d39781bddd2d83ae2` | PR #72 已合并，head 精确相同 |
| `codex/docs/prune-historical-docs-20261110` | `7dbd053f5c5bd2c2dc504ee6051e2272b337512d` | PR #129 已合并，head 精确相同 |
| `codex/docs/recent-work` | `da73289f0954e2a930ad6f351d089fe9d94bccb3` | PR #110 已合并，head 精确相同 |
| `codex/docs/requirements-grilling` | `c9de6a4df575d9c89d9b15b556289c39548bf25d` | PR #93 已合并，head 精确相同 |
| `codex/docs/startup-flash-delivery` | `7831bed2f9420c357973258d16669f39c560fa33` | PR #58 已合并，head 精确相同 |
| `codex/docs/startup-wordmark-merge-receipt` | `2482bd6d7c21ebce3e1b0ff82b9c2a6eb72a4e88` | PR #87 已合并，head 精确相同 |
| `codex/docs/website-readme-link` | `e75f1cc30da96aa3c483274188c0a844ba4887b1` | 官网链接已由 PR #127 和 main README 覆盖，仅旧阶段文档差异 |
| `codex/docs/workflow-efficiency` | `c5b600d798fb1e374f3ca1490621592514d4a02b` | PR #96 已合并，head 精确相同 |
| `codex/feat/channel-rankings` | `d449c9439830d2a4e4187287030bd61bd1f5b17d` | PR #106 已合并，head 精确相同 |
| `codex/feat/custom-tags` | `961020cc241c31d653a0bd6dfde869062d9896a8` | PR #109 后仅两次文档回执，已归集至 CUSTOM-TAGS-01 |
| `codex/feat/email-password-reset` | `5b10fc927a7f154766e8c9b5e06322d881d05869` | PR #120 已合并，head 精确相同 |
| `codex/feat/feed-scroll-smile-refresh` | `fad2a5cf73ea6b908c89ab134ca21d6ad285b40d` | PR #65 已合并，head 精确相同 |
| `codex/feat/feed-vector-cards` | `630341269b67045b66e5738533a06edffe00f9f8` | PR #121 已合并，head 精确相同 |
| `codex/feat/ios-v1-parity` | `b956cb0cc293763b366ad206839458b16d34c4dd` | PR #117 已合并，head 精确相同 |
| `codex/feat/passkey-native-domestic` | `73c29fa434e54298f7a40405b2a3cffb0fd30520` | PR #118 已合并，head 精确相同 |
| `codex/feat/post-detail-comments` | `aef761806d110f98f4c21ab95dba4369f076267d` | PR #71 已合并，head 精确相同 |
| `codex/feat/post-search` | `13160f0f6e26e1c7df9d2c76d81263418ca2c25a` | PR #101 已合并，head 精确相同 |
| `codex/feat/post-sharing` | `be5f06decb04e15753741513bd9a498101715e50` | PR #108 已合并，head 精确相同 |
| `codex/feat/post-sharing-mobile` | `c3b1fa56975299b30118bdf62fa44c4894761e2a` | PR #111 已合并，head 精确相同 |
| `codex/feat/profile-settings-delivery` | `3e9555df1c034ab729bb75df3d2236ab683a7b2c` | PR #99 已合并，head 精确相同 |
| `codex/feat/telegram-motion` | `7efe83509f21981753a8e7d36ed53b8210b04850` | PR #107 已合并，head 精确相同 |
| `codex/feat/website-android-download-20261109` | `d89773533ab6a8b80364d38b983cbb891346ee73` | PR #127 已合并，head 精确相同 |
| `codex/feat/welcome-mascot-logo` | `ff633ff11ecd534685b7eaaf2feb51dc5d1718c6` | PR #88 已合并，head 精确相同 |
| `codex/fix/auth-login-startup` | `635a2ff0eefab4a60736e4cfd67edebb32b12b53` | PR #56 已合并，head 精确相同 |
| `codex/fix/dm-resume-notifications` | `3bdade9fb4a24919c159b4aac66ef610385e12aa` | PR #105 已合并，head 精确相同 |
| `codex/fix/feed-action-spacing` | `858dd580626dbb0762c711fe19e8f13344ba134f` | PR #122 已合并，head 精确相同 |
| `codex/fix/feed-refresh-cancel` | `217bd4e0812ff31620eef3db315ce63df3a3af79` | PR #69 已合并，head 精确相同 |
| `codex/fix/media-seamless-renewal` | `06448b36e7abd38622e0dc3eaaff7ae58dd0b59c` | PR #76 已合并，head 精确相同 |
| `codex/fix/post-search-as-you-type` | `4494d3f687d8bf76ab164288ba71a0d14a426d97` | PR #104 已合并，head 精确相同 |
| `codex/fix/review-remediation-20261108` | `7ecd44df0b8fe54ac57274ef3f262dca19e33ac7` | PR #123 已合并，head 精确相同 |
| `codex/fix/startup-media-profile` | `97a2f9d2e4e3f9d212b604fbb30337e34c1465a1` | PR #66 已合并，head 精确相同 |
| `codex/fix/theme-blue-avatar` | `70a1525dcce5b4e132781e145535a8007822ee24` | PR #97 已合并，head 精确相同 |
| `codex/perf/global-hotlist-search-isolation` | `3ace5c2ca0dbe7371cc2adcbf0036095bea707a0` | PR #115 已合并，head 精确相同 |
| `codex/refactor/categories-global-hotlist` | `084bd3e8536634922dfb2776181a212005a2017a` | PR #112 已合并，head 精确相同 |
| `codex/test/android-v1-acceptance-01` | `c1dd9124fb8a441c4234868efa393a11bc8e8df0` | PR #113 后仅回执，main 已含相同最终回执 |
| `docs/identity-avatar-delivery` | `f7a41f93edd7a5f24a9c9568ffe842d3b3b7b303` | PR #64 已合并，head 精确相同 |
| `docs/image-viewer-gestures-merge-receipt` | `a36588883fa41c785e454f1f6013e20731b48296` | PR #79 已合并，head 精确相同 |
| `docs/mobile-navigation-merge-receipt` | `a3cb6dbea0c22b32cd8e322368ac0bfc1773abb7` | PR #59 已合并，head 精确相同 |
| `docs/notification-inbox-delivery` | `825513a1d6c61aa6f9dfaa0f0210246ded84b813` | PR #82 已合并，head 精确相同 |
| `docs/shared-rate-limit-merge-receipt` | `992126f4ee4e40a6fe28f0401d9e00f916a1154f` | PR #74 已合并，head 精确相同 |
| `feat/direct-message-chat-completion` | `796c54f49403404d8a4c9561966c868cc9ac0a51` | PR #51 已合并，head 精确相同 |
| `feat/direct-message-governance` | `483fe9a237c38f03768f859935b9020b9520267a` | PR #52 已合并，head 精确相同 |
| `feat/email-auth-flow` | `bd5943122958a056edd463f9223ec6e86a558f41` | PR #45 已合并，head 精确相同 |
| `feat/identity-avatar-onboarding` | `7a3b40286cc309b9ca2295dcac7d0bc0c5488108` | PR #63 已合并，head 精确相同 |
| `feat/notification-inbox` | `560818e89fc89e1cdd260507c97c8e0d90dd0e30` | PR #81 已合并，head 精确相同 |
| `feat/personal-profile-mood` | `ca0580be25a072d3a85dec14094eddcde9d9e8e5` | PR #68 后仅合并回执，本任务已保存其原版本摘要 |
| `feat/shared-rate-limit-abuse-control` | `5ef371db493869bf0ab7679c6b874e4fcb144d30` | PR #73 已合并，head 精确相同 |
| `feat/startup-wordmark` | `f29ed9fefb67155507e0887e1d7c9376b6f21d87` | PR #86 已合并，head 精确相同 |
| `fix/direct-message-header-density` | `a0bc27b4fe75fa9ac3e275ad083a672eec3c66d4` | PR #62 已合并，head 精确相同 |
| `fix/direct-message-identity-loading` | `8da6048b64d1d33a03ee41647c89f00253d441b9` | PR #60 已合并，head 精确相同 |
| `fix/image-viewer-gestures` | `7abfe169d9f671e7717b133022754feaef710453` | PR #77 已合并，head 精确相同 |
| `fix/mobile-navigation-refresh` | `f1e85fc2b51050170eb7e7955638be6b8d80a585` | PR #57 已合并，head 精确相同 |
| `test/direct-message-device-recovery` | `488d8d6461a8c90a468fff0ee37ca33e5fc41d73` | PR #55 已合并，head 精确相同 |
| `test/realme-large-photo-memory` | `77ed6f49aebd47d43b72a2d1eef1dd6c8c0cb21e` | PR #61 已合并，head 精确相同 |
