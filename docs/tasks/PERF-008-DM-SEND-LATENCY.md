# PERF-008 — 私信发送响应速度优化

- 模式：常规
- 负责人／状态维护人：当前 Session
- 执行 Session（指挥模式）：无
- 分支／PR：`perf/dm-send-latency` → `main`，[#24](https://github.com/zewbby/SpeechFUL-Develop/pull/24)。本轮基于 PR head `2fc0590` 修复，核对目标 `f1a79b6`，merge-base `213b8b0`。
- 本轮规则版本（指挥模式）：不适用
- 授权依据与边界：用户 2026-11-19 指令“按照顺序修复”，对应本 Session 已报告的 P0/P1/P2；2026-11-20 指令“提pr”，授权 push 任务分支与创建 PR；2026-11-20 指令“修复这鞋问题”，授权修复 PR #24/#25 的审查问题；2026-11-21 指令“推送最新pr”“修复ci”，授权推送最新提交并重跑失败的 CI。仅修改私信发送调度、私信轮询去重、私信 PUT 连接复用与 DM 未 ACK 部分索引；不改变 `PENDING_LIMIT` 4096 语义、不改变全局授权 Gate 架构；未授权合并与部署。
- 需求确认：已确认诊断与修复顺序。P0：点击即发独立通道、PUT 保持连接、`send()` 不阻塞目录刷新。P1：去掉每轮重复目录请求与重复目录刷新。P2：为发送事务的 outstanding 计数增加部分索引，并把计数移到最终事务的 prepare 阶段；同一账号的发送在最终事务中已先锁账号行，计数仍与提交串行，4096 上限不变。实施中依据测试回退了“`queued` 跳过预检 GET”：离线首发必须保持 `queued`（未发出、可取消），不能为省一个 RTT 把未发送消息标成 `unknown`。未决取舍：无。
- 更新日期：2026-11-21
- 状态：实现 完成（含审查修复 `4483410`、`69044bb`）；验证 PASS（Flutter analyze、DM/HTTP 测试、Go build/vet、隔离 PostgreSQL lab、真机功能、真机发送时延基线/修复对比）；代码 head `87205c2` 及后续文档提交的 CI PASS（重跑后 25 success、1 skip）；审查修复后的真机复测 NOT_RUN；PR [#24](https://github.com/zewbby/SpeechFUL-Develop/pull/24) 已创建；合并 未合并；发布 未发布

## 目标与范围

缩短用户点击发送到消息进入“发送中/已发送”的等待，并降低私信轮询对发送链路的争用。

包含：

- 移动端发送独立于 4 秒轮询：`send()` 入队后立即触发发送通道；发送通道与同步通道共用同一互斥，避免并发重复 PUT。
- 发送前的 `GET commands/{id}` 回执核对保留：`queued` 表示从未发出、可取消；只有 `unknown` 才可能已到达服务端。该契约由 `dm_reliability_test.dart` 覆盖，本次不改变。
- 私信 PUT/POST 复用 HTTPS 连接；其他请求的连接策略不变。
- `send()` 不再等待目录刷新，入队提交后立即通知 UI；目录刷新转入后台并只通知一次。
- 每轮同步只拉一次 `chat/conversations`；去掉 `finally` 中重复的目录刷新。
- 迁移新增 `c_dm.messages(sender) WHERE NOT acknowledged` 部分索引。
- 发送事务的 outstanding 计数移到 `withPostSessionAtPrepared` 的 prepare 阶段，不再持有全局 Gate 行锁。

不包含：

- 不改变消息发送/ACK/已读/撤回/拉黑/限流语义，不改变 HTTP 契约。
- 不改变 4 秒轮询周期与前台接收延迟。
- 不改变全局授权 Gate 对私信读请求的最终事务校验；轮询读的 Gate 评估结论记录在“阻碍与下一步”。
- 不做目录刷新的批量 SQL 重构；本次只去掉重复调用，保留现有逐会话读取。

## 接口与依赖

- 移动端：`DmController.send` / `_kickSend` / `_queueRefresh` / `_synchronize` / `_drainSends`；`DmChatController._chatBeforeSync`；`PostHttpTransport.send` 新增可选连接复用参数；`HttpPostApi.dmRequest` 传入私信专用值。
- 服务端：`Community.sendDMTextOnce` 改用 `withDMSessionPrepared`；新增 `withDMSessionPrepared` 与 `dmSenderPendingSQL`；迁移 `0054_dm_sender_pending_index.sql`；新增 `TestDMSenderPendingIndexPostgres`。
- 上游：无新增依赖。

## 验收标准

- 点击发送后 UI 立即出现本地消息，不再等待 4 秒轮询或目录刷新完成；发送通道完成后状态从 `queued` 变为 `sent`。
- `queued`/`unknown` 行仍先 `GET` 回执再决定 `PUT`；离线首发重启后仍为 `queued`；重启/崩溃恢复路径不变。
- 私信 PUT 复用连接；发帖、评论、上传等非私信请求仍按原策略关闭连接。
- 每轮同步每个身份只请求一次 `chat/conversations`；未读数、置顶、删除/撤回的可见状态不回退。
- 发送事务不再在持有全局 Gate 行锁时执行 outstanding 全表计数；4096 上限仍严格。
- 迁移在 fresh/upgrade 库通过，发送计数查询计划命中新索引。

## 当前进度与证据

### PR #24 目录刷新回归修复（2026-11-21）

- 用户授权修复审查 P1/P2、在验证通过后推送原 PR。本轮新增回归测试仅保留在本地。
- 收信完成后先刷新会话目录，再按当前会话的免打扰状态决定是否提醒。未读数可以在发送 PUT 等待期间更新。发送回执落库后刷新列表中的消息状态和正式会话 ID。
- Windows、Flutter 3.47.5，基线 `2fc0590` 加本提交的两个私信控制器差异。首条通知、慢发送期间未读更新、首发回执更新目录三项本地用例，修复前 3 FAIL，修复后 3 PASS。
- `flutter test --no-pub` 运行全部 `dm_*_test.dart`、`post_http_connections_test.dart` 和上述本地用例，115 PASS、3 SKIP。跳过项需要外部 fixture。`flutter analyze --no-pub lib/src/messaging` PASS。本轮 Android 真机 NOT_RUN。

### 前期实现与验证

| 项目 | 被测版本／环境 | 结果与证据 |
| --- | --- | --- |
| 实现 | `perf/dm-send-latency` 提交 `16eccb6`，基线 `213b8b0` | 完成：P0/P1/P2 代码与迁移 0054 |
| 审查问题修复（P2 回执通知） | 提交 `78f85fa`（基于 `8e0526f`，目标 `develop/main@dbbc249`） | 完成：`_kickSend` 在 `_drainSends` 之后校验 epoch、发布 `_operationError` 并 `_notify()`；`send()` 仍只等待持久化入队，回执不再依赖下一轮 4 秒同步 |
| 审查问题修复（P3 测试可重跑与 keepAlive 覆盖） | 提交 `4483410` | 完成：DM 真机测试改用每次运行唯一的 `runTag` 文本，避免同 origin 重跑时 `waitForDmState` 匹配多行；`post_http_connections_test.dart` 增加 POST/PUT `keepAlive` 后连接仍可复用的断言 |
| 审查后续问题修复（同步成功清除即时错误） | 提交 `69044bb`（基于 `988102c`） | 完成：`_synchronize` 开始时记录 `_immediateErrorRevision`；发送通道发布并清除 `_operationError` 时递增修订号；接收通道收尾只在没有新 `_operationError` 且修订号未变时清除可见错误。此前若发送通道在同步进行中发布了“待核对”类提示，接收通道随后用已被发送通道清空的 `_operationError` 覆盖 `error = null`，用户看不到回执；修复后该提示保留到下一次成功同步。新增“同步中的发送回执不能擦除待 ACK 提示”用例，临时回退控制器修复时按预期失败（`Expected: contains '核对'; Actual: <null>`），恢复后通过 |
| Flutter 静态检查（审查修复后） | `69044bb` | `flutter analyze --no-pub` → `No issues found!`；`dart format` 2 个改动文件 0 变化；`git diff --check` → PASS |
| Flutter 定向测试（审查修复后） | `69044bb` | `flutter test --no-pub test/dm_reliability_test.dart test/dm_chat_races_test.dart test/dm_lifecycle_test.dart test/post_http_connections_test.dart` → 53 passed / 0 failed；新增“即时发送通道无需显式 sync 即可发布 `sent`”与“离线失败即时可见”回归，后者在临时回退控制器修复时按预期失败（通知数停留 2）；PASS。错误竞态修复后复跑全部 14 个 `test/dm_*.dart` → 104 passed / 3 skipped / 0 failed；PASS |
| Flutter 完整套件（错误竞态修复后） | `69044bb`；Windows 11 25H2 x64 / Flutter 3.47.5 / Dart 3.13.4 | `flutter test --no-pub --concurrency=2 -r compact` → 1364 passed / 4 skipped / 0 failed，exit 0；PASS |
| 真机复测（审查修复后） | 未运行 | NOT_RUN：控制器改动仅增加一次通知，未改变 PUT/Gate/存储路径；真机行为证据仍为修复前 `16eccb6` 构建，需在下一轮真机窗口复验 |
| Flutter 静态检查 | 同上 | `flutter analyze lib/src/messaging lib/src/posts/post_http_transport.dart lib/src/posts/http_post_api.dart` → `No issues found!`；PASS |
| Flutter DM 测试 | `16eccb6` | 14 个 `test/dm_*.dart`：101 passed / 3 skipped / 0 failed，覆盖 `dm_reliability_test.dart`、`dm_chat_races_test.dart`、`dm_chat_test.dart`、`dm_http_fixture_test.dart` 等；`post_http_connections_test.dart`、`post_api_test.dart`、`comment_http_test.dart`、`governance_http_test.dart`：61 passed；PASS |
| Go 静态检查 | 同上 | WSL Ubuntu-2404：`gofmt` 后 `go build ./...`、`go vet ./internal/authprivacy/...`；PASS |
| 服务端隔离 lab | `services/api/authlab/run-dm-chat-isolated.sh`，临时 PostgreSQL 16，`Ubuntu-2404` | `TestDMPostCard*`、`TestDMChat*`、`TestDMFirstContact*`、`TestDMReliableText*`、`TestDMGoose*`、`TestDMSenderPendingIndexPostgres` 全部 PASS；`TestDMChatDevicePostgres` SKIP（未请求独占 Android fixture）；PASS |
| 迁移链 | 隔离 lab fresh/upgrade 库 | `TestDMGooseFreshAndUpgradePostgres` fresh/upgrade PASS；ledger=54；`down-to 49` 被拒绝且 ledger 保持 54；PASS |
| 查询计划 | 隔离 lab | `TestDMSenderPendingIndexPostgres` PASS：`EXPLAIN`（禁用 seqscan/bitmapscan）命中 `dm_sender_pending` |
| 真机功能验收 | Xiaomi 15（24129PN74C，`dada`，Android 17/API 37，序列号 `4e56f5`），APK `2026111904`，隔离 C/V/PostgreSQL HTTPS fixture（WSL Ubuntu-2404，`DM_DEVICE_FIXTURE`），2026-11-19 18:25:51–18:25:56 | `flutter drive --use-existing-app` 输出 `All tests passed.` EXIT=0，测试端 `00:04 +2: All tests passed!`；覆盖首条 PUT 响应丢失→`unknown`、重开→`sent`、接收方 inbox/ACK/未读=1、回复发送、草稿与历史持久化；证据 `D:/dm-device-verify/drive_result.log`；PASS |
| 真机发送时延（基线 `213b8b0`） | Xiaomi 15（24129PN74C，`dada`，Android 17/API 37，序列号 `4e56f5`），基线 worktree `E:/speechful-dm-baseline`，APK `2026111911`（`HNUHOLE_DM_TEXT=1`，`org.hnuhole.hnuhole_mobile.dmtext`），隔离 fixture 就绪 12:31:52，`HOST_PORT=33439` | `flutter drive --use-existing-app` 输出 `All tests passed.`；`DM_SEND_LATENCY`：稳态 5 条 3016/3982/4029/3965/4006 ms，中位 3982 ms、均值 3799.6 ms；回复 597 ms；首条（PUT 响应丢弃）533 ms→`unknown`；证据 `D:/dm-device-verify/baseline-latency.txt`；PASS |
| 真机发送时延（修复后） | 同上设备，`perf/dm-send-latency` 提交 `16eccb6`，APK `2026111912`（`org.hnuhole.hnuhole_mobile.dmtext`），隔离 fixture 就绪 12:33:47，`HOST_PORT=40623` | `All tests passed.`；稳态 5 条 517/450/474/433/618 ms，中位 474 ms、均值 498.4 ms；回复 519 ms；首条 428 ms→`unknown`；证据 `D:/dm-device-verify/fixed-latency.txt`；PASS |
| 时延对比 | 同一设备、同一隔离 C/V/PostgreSQL HTTPS fixture 模式，基线/修复各 1 轮、每轮 5 条稳态样本 | 稳态中位 3982→474 ms（8.4×，-88.1%）；均值 3799.6→498.4 ms（7.6×，-86.9%）；最差 4029→618 ms（6.5×）；PASS |
| PR CI（首次运行） | PR [#24](https://github.com/zewbby/SpeechFUL-Develop/pull/24)，head `87205c2`；run [44866994550](https://github.com/zewbby/SpeechFUL-Develop/actions/runs/44866994550) | FAIL：`flutter-and-android` 的 `Build isolated Android chat integration` 在 18s 失败，Gradle 无法从 `plugins.gradle.org` 解析 Kotlin 2.2.21（`Could not find org.jetbrains.kotlin:kotlin-stdlib:2.2.21`、`Could not parse POM .../kotlin-gradle-plugin-2.2.21.pom`）。同一 head 的合并树 `c5d94fd` 在本地按相同命令构建通过，APK 包名正确（`org.hnuhole.hnuhole_mobile.dmchat`）；失败时段 `plugins.gradle.org` 的 Kotlin 2.2.21 POM 经重定向后由 Maven Central/插件 CDN 返回 200，判定为上游插件仓库瞬时不可用，非代码或配置缺陷。 |
| PR CI（重跑后） | 同上 head `87205c2` | 重跑失败 job 后 26 项检查：25 success、1 skip、0 failure（含 policy、scope、compile、go-postgres、gate-postgres、flutter、mobile-recovery、reliable-chat-sql、search-postgres-100k 等）；PASS。PR [#25](https://github.com/zewbby/SpeechFUL-Develop/pull/25) head `fab02f7` 同期重跑后 24 success、1 skip、0 failure；PASS |

## 阻碍与下一步

- 实施中依据测试回退了“`queued` 跳过预检 GET”：`dm_reliability_test.dart` 要求离线首发重启后仍为 `queued`（可取消、未发出），`dm_chat_races_test.dart` 以该 GET 作为删除竞态的同步点。保留该 GET；P0 的连接复用、点击即发与 P1 的目录去重仍然生效。
- 真机 fixture 的授权证据 `ValidUntil=now+5m`（`e2eAuthorizationGateControlFor`），而 fixture 自身等待 25 分钟；超过 5 分钟后 Gate 以 `EVIDENCE_EXPIRED` 冻结，后续请求均为 503 `AUTHORIZATION_UNAVAILABLE`。真机复测须在 fixture 写文件后 5 分钟内启动，否则重启 fixture。
- 真机功能 PASS 的构建已包含 P0/P1/P2 代码；插桩测试新增 `waitForDmState` 与 `DM_SEND_LATENCY` 输出（仅 `integration_test/dm_text_device_test.dart`），用于证明 `send()` 不依赖显式 `sync()`。临时 `dm_controller.dart` 诊断输出已移除，静态检查与 DM 测试复验通过（101 passed / 3 skipped）。
- 时延口径：从 `send()` 调用到本地行进入 `sent`（含预检 `GET commands/{id}`、`PUT`、本地 vault 提交）；每条使用唯一文本，10 ms 轮询。基线稳态样本被 4 秒轮询约束（3016–4029 ms），修复后为 433–618 ms，差异来自调度结构而非网络抖动。修复版 APK 构建于提交 `16eccb6` 的同一工作树（构建后仅更新本记录）。证据为同设备同网络、每构建 1 轮 5 样本，不构成统计基准；如需更强置信度可重复多轮。
- 真机测试包必须设置 `HNUHOLE_DM_TEXT=1`（`android/app/build.gradle.kts` 读取该环境变量决定 `.dmtext` 后缀）。本次基线构建曾漏设，生成的 APK 实际是主包 `org.hnuhole.hnuhole_mobile`，`pm install -r` 覆盖了设备上的主应用（应用数据保留）；已从 `E:/speechful`（`fix/avatar-feed-projection@ea62171`，versionCode `2026111701`）重建主应用并 `pm install -r -d` 恢复，启动验证 `topResumedActivity=org.hnuhole.hnuhole_mobile/.MainActivity`、截图 `D:/dm-device-verify/main-restored.png`。该误装不属于产品变更。
- 测试工具新增 `integration_test/dm_latency_device_test.dart`，并在 `dm_text_device_test.dart` 增加 `dmTextOf`/`waitForDmState` 与 `DM_SEND_LATENCY` 输出；仅隔离测试包使用。
- 复测命令（fixture 写文件后 5 分钟内完成）：`HNUHOLE_DM_TEXT=1 flutter build apk --debug --target=integration_test/dm_latency_device_test.dart --dart-define-from-file=D:/dm-device-verify/fixture.json --build-number=<n>`；`adb push` + `pm install -r`；`adb reverse tcp:58443 tcp:<HOST_PORT>`；`flutter drive --use-existing-app=<vm-service-uri> --driver=test_driver/auth_live_device_driver.dart --target=integration_test/dm_latency_device_test.dart`。
- 轮询读是否必须持有全局 Gate：属于授权证据最终校验的安全边界，本次不调整；先通过减少每轮请求数降低争用，若仍需进一步优化，另开安全评审。
- 已 push `develop/perf/dm-send-latency` 并创建 PR #24；最新 head `87205c2` 的 26 项 CI 在重跑失败 job 后为 25 success、1 skip、0 failure。首次失败来自 `plugins.gradle.org` 无法提供 Kotlin 2.2.21，本地同命令构建通过，按上游瞬时故障处理，未修改构建配置。审查修复后的真机复测仍为 NOT_RUN；未合并、未部署，需用户另行授权。
- 本轮“还有问题么”复查结论：PR #25 重新核对时间线排序与置顶字段后未发现新的正确性问题；PR #24 发现并修复了上述同步期间错误提示被擦除的竞态。两分支均未新增未决取舍。
