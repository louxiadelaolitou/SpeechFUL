# REQ-007-NOTIFICATION-READ-ACCEL — 通知列表与未读统计读取加速

- 模式：常规
- 负责人／状态维护人：当前 Session
- 执行 Session（指挥模式）：无
- 分支／Worktree／PR：`perf/notification-read-accel` @ `E:\speechful-notif-accel`；实现提交 `44243ca`；旧集成提交 `743cc4c`（按 #4 `a25aa8b` → #6 `44f07a4` → #5）；冲突修复合并提交 `633b287`（合并 #6 head `591b7ba`，内含 `main@3746d5a`）；PR [#5](https://github.com/zewbby/SpeechFUL-Develop/pull/5)（已由用户合入 base `perf/comment-load-accel`，合并提交 `6dbdfb0`；`main@bf5c0ae` 尚无 0049，待后续 `perf/comment-load-accel` → `main` PR 交付）
- 本轮规则版本（指挥模式）：不适用
- 授权依据与边界：用户 2026-11-16 指令“继续完成07优化”（需求行“通知列表与未读统计优化”）及“完成后向仓库提交pr”；#6 冲突修复后用户选择“接手修复 #5”，授权将最新 #6 合入本分支、解决冲突并把迁移重编号为 0049。授权范围为定位、实现、隔离库验证、本地提交、推送本任务分支并维护 PR；未授权合并、部署、发布或改动正式数据。
- 需求确认：用户已确认优化对象为通知列表与未读统计读取。读取口径沿用现有产品语义：互动通知按当前身份过滤，系统通知按账号共享；COMMENT 未读为 `read_version=0`，LIKE 未读为存在版本大于 `read_version` 的点赞事件；取消点赞不回退 `version`，已读历史由 `seen_like_count` 保留。无未决取舍。
- 更新日期：2026-11-16
- 状态：实现 已完成（含合并 #6 head `591b7ba`、冲突解决、迁移重编号 0049）；验证 PASS（迁移 49/49、通知行为、查询计划、等价性、合并后组合回归）；产品级 P95 NOT_RUN；合并 已合入 `perf/comment-load-accel`（合并提交 `6dbdfb0`），未进入 `main`；发布 未发布

## 目标与范围

加快通知列表首屏与未读角标统计的数据库读取，不改变接口字段、排序、分页游标或未读语义。

包含：
- 为 `c_inbox.records` 增加冗余列 `latest_like_version`，由点赞事件触发器在记录行锁内维护，把逐条通知的 `EXISTS c_inbox.like_events` 未读探测改为行内比较。
- 新增未读部分索引，同时服务未读统计聚合与“只看未读”列表排序。
- 未读统计由“选中身份一次列表聚合 + 账号级逐行探测 + 账号级逐身份计数”合并为一次账号级分组查询加一次系统通知计数。
- 列表 SQL 按“全部/只看未读”拆分，避免在 SQL 内对每行计算 `unread` 再过滤。

不包含：
- 移动端 UI、接口契约或响应结构变化。
- 系统通知表结构或已读逻辑调整。
- 正式库数据迁移执行、发布与线上指标采集。

## 接口与依赖

- 无 OpenAPI 变更；`ReadInbox`、`ReadInboxCounts` 的输入输出保持不变。
- 依赖迁移 `0049_notification_read_indexes.sql`。PR #4 已合入 `main@3746d5a`，`main` 占用 0043–0047；#6 的评论索引为 0048；本任务在 #6 之后使用 0049，账本 49/49。合并顺序：#4（已合并）→ #6 → #5（已合入 #6 分支 `perf/comment-load-accel`，合并提交 `6dbdfb0`）；因合入时 base 仍为 `perf/comment-load-accel`，`main@bf5c0ae` 尚未取得 0049，需后续以 `perf/comment-load-accel` → `main` 的 PR 完成交付。
- 迁移为前向修复：`Down` 与 0028/0041/0042 一样显式拒绝，因为回退函数会恢复逐行事件探测，而读取代码已依赖新列。

## 验收标准

- 通知列表“全部 / 回复 / 点赞 / 系统”的顺序、游标分页和未读标记与改动前一致。
- 未读统计在选中身份与未选身份时均与改动前一致；系统通知只计一次且按账号共享。
- 点赞、取消点赞、标记已读、关闭点赞者账号后，未读数与未读列表立即正确；全部取消后已读历史仍保留，不出现幽灵未读。
- 读取路径不再为每条 LIKE 通知探测 `like_events`；未读统计不再逐身份重复扫描。
- 运行角色仍不能写 `latest_like_version`，只能更新 `read_version`、`seen_like_count`。

## 当前进度与证据

| 项目 | 被测版本／环境 | 结果与证据 |
| --- | --- | --- |
| 实现 | `perf/notification-read-accel`，基线 `1d829b2`，实现提交 `44243ca` | 迁移 0049、读取 SQL、统计合并、迁移账本与行为断言 |
| 验证 | WSL2 Ubuntu-24.04 / PostgreSQL 16，隔离 socket fixture；Go 1.22.2 | PASS：`go build -mod=readonly ./cmd/authdev`、`go vet -mod=readonly ./internal/authprivacy ./internal/authprivacyhttp`；`TestNotificationInboxPostgres`、`TestNotificationInboxIdentityPaginationPostgres`、`TestNotificationInboxHTTPSPostgres`、`TestInboxRoutesAndDisabledMentionsBoundary`、`TestNotificationInboxGooseMigrationPostgres`（fresh/upgrade，含 40,000 行回填与索引断言）、`TestDMGooseFreshAndUpgradePostgres`（fresh/upgrade）、`TestFeedGooseFreshAndUpgradePostgres`（7 模式）、`TestIdentityAvatarGooseFreshAndUpgradePostgres`（独立 fixture）均 PASS |
| 查询计划 A/B | 同一隔离数据集副本：50,000 条互动记录、120,000 条点赞事件、5,000 条系统通知；账号 5 个身份；VACUUM ANALYZE 后热缓存 | 未读统计（选中身份 + 账号级逐身份）：160.5 ms → 7.4 ms（分组 6.9 ms + 系统 0.5 ms）；全部列表：50.7 ms → 16.9 ms；未读列表：75.1 ms → 6.7 ms。基线每条 LIKE 通知一次 `like_events` 探测（选中 24,000 次、账号级 40,000 次），最终统计走 `inbox_unread` Index Only Scan（324 buffers），未读列表同样命中该索引 |
| 等价性 | 同上 A/B 库 | 各身份未读数与系统计数逐项一致；全部列表与未读列表前 21 条 ID/时间逐行 `diff` 一致 |
| 集成验证（合并 #6 前） | 旧集成提交 `743cc4c`（#4 `a25aa8b` → #6 `44f07a4` → #5），WSL2 Ubuntu-24.04 / PostgreSQL 16，隔离 fixture；Go 1.22.2 | PASS：迁移账本 45/45（DM fresh/upgrade、feed 7 模式、avatar、通知 fresh/upgrade）；DM 目录重建 7 子测试、索引回滚/重放、迁移版本唯一性；评论/通知 CI 聚焦集（`TestCommentText*`、`TestCommentBusiness*`、`TestNotificationInbox*`、`TestInboxRoutesAndDisabledMentionsBoundary`、moderation 迁移集）全部 PASS；`go build ./...`、`go vet`、LF 副本 `gofmt -l` 无输出 |
| 合并 #6 与迁移重编号 | 合并提交 `633b287`，合并 #6 head `591b7ba`；WSL2 Ubuntu-24.04 / Go 1.22.2 | PASS：冲突 5 处（comments CI 路径、CHANGELOG、3 个迁移测试断言），已取 #6 的 0048 版本并改为 49；`0045_notification_read_indexes.sql` 重命名为 `0049_notification_read_indexes.sql`，通知迁移测试、inbox 注释与 CI 路径同步；相对 #6 的最终差异 10 个文件、无冲突标记 |
| 合并后静态检查 | 同上 | PASS：改动 Go 文件 `gofmt` 无输出；`go build ./...`；`go vet ./internal/authprivacy ./internal/authprivacyruntime ./internal/authprivacyhttp` |
| 合并后迁移链 | 同上，隔离 PostgreSQL 16 lab | PASS：`TestNotificationInboxGooseMigrationPostgres` fresh/upgrade 均为 49；`TestDMGooseFreshAndUpgradePostgres` fresh/upgrade 均为 49；`TestFeedGooseFreshAndUpgradePostgres` 七个环境均为 49；`TestIdentityAvatarGooseFreshAndUpgradePostgres` fresh/upgrade 均为 49；`TestMigrationVersionsAreUnique` PASS |
| 合并后通知回归 | 同上 | PASS：`TestNotificationInboxPostgres`、`TestNotificationInboxIdentityPaginationPostgres`、`TestNotificationInboxHTTPSPostgres`、`TestInboxRoutesAndDisabledMentionsBoundary` |
| 产品 CI（#5 最终 head） | PR #5 head `1437eb4`（仅任务记录与 CHANGELOG），WSL2／GitHub Actions | PASS：13 个 workflow 全部 success；23 项检查 22 success、1 skip（`actual-process-acceptance` 按设计跳过），无 failure |
| 合入 #5 | 合并提交 `6dbdfb0`（合并 `1437eb4` 到 `591b7ba`），2026-11-16T13:27:46Z | 用户操作合入 base `perf/comment-load-accel`；PR #5 状态 merged；`main@bf5c0ae` 不含 0049 |
| 产品 CI | PR #5 head `3b25645`（集成前） | PASS：21 项检查 20 success、1 skip（`actual-process-acceptance` 按设计跳过）；`go-postgres`、`feed-postgres`、`flutter`、`policy` 等均 success |
| 产品 CI | PR #5 集成 head `38f93f4` | PASS：21 项检查 20 success、1 skip（`actual-process-acceptance` 按设计跳过），无 failure；`go-postgres`、`feed-postgres`、`flutter`、`policy` 等均 success。另有 1 个重复 `policy` 运行被 concurrency 取消，不影响结果；PR 状态 clean |
| 产品级 P95 | 真实规模数据与客户端链路 | NOT_RUN |

## 阻碍与下一步

无实现阻碍。#6 冲突修复后已合并其最新 head `591b7ba`，本分支迁移由 0045 重编号为 0049，5 处冲突已解决并在隔离库复验 PASS；PR #5 最终 head `1437eb4` 产品 CI 全绿，并已由用户合入 `perf/comment-load-accel`（合并提交 `6dbdfb0`）。合入时 base 仍是 #6 分支而非 `main`，因此 `main@bf5c0ae` 只有 0048、没有 0049；交付 `main` 需另开 `perf/comment-load-accel` → `main` 的 PR 并获授权，未获授权前不创建、不合并。产品级 P95、真机联调与发布验证未执行；未部署。
