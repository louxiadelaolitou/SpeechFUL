# REQ-006 — 私信会话目录查询优化

- 模式：常规
- 负责人／状态维护人：当前 Session
- 执行 Session（指挥模式）：无
- 分支／Worktree／PR：`perf/dm-directory-reads`，基线 `main@1d829b2`；未创建 PR
- 本轮规则版本（指挥模式）：不适用
- 授权依据与边界：用户 2026-11-16 指令“按方案A执行，此会话执行06优化”。仅实现需求 06；不开始 03/07；不合并、不部署、不打 tag、不发布、不 force push。迁移号 0043 由本任务占用，03 使用 0044 及以后；0043 必须先合入，goose 只应用高于当前版本的新迁移。
- 需求确认：需求 06（私信会话目录查询优化，用户提供文档）。保留会话 ID 游标分页、100 条上限、置顶/备注/静音、隐藏/删除消息边界、撤回、acknowledged/read 语义、来源帖与双向拉黑规则；新增 `(low_identity, conversation_id)`、`(high_identity, conversation_id)` 索引；候选页只做索引分支 `UNION ALL`，选中页再批量关联；未读与 acknowledged-unread 一次条件聚合；接入既有 P0 两阶段读取并在最终 C Gate 事务内重验，变化时有界重算一次。
  - 已确认偏差：需求文档要求 `CREATE INDEX CONCURRENTLY`；仓库测试 lab 将全部 Up SQL 放在单一事务执行，且 0041/0042 同样要求写入静默，因此 0043 采用 `LOCK TABLE ... SHARE ROW EXCLUSIVE` + 普通 `CREATE INDEX`。按文档“先上线索引，再灰度读取”的独立回退边界执行，记录中不声称并发建索引。
  - 已确认偏差：0043 Down 与 0041/0042 一致为 forward-only（抛错）。迁移测试 `TestDMGooseFreshAndUpgradePostgres`/`TestIdentityAvatarGooseFreshAndUpgradePostgres` 把“最近迁移可回滚”视为破坏性回滚并失败；索引又属派生访问路径。放弃优化时先关闭 `preparedDmDirectoryReads`，再执行运维脚本 `services/api/ops/0043_dm_directory_cursor_indexes_rollback.sql`；账本保持 0043，恢复用同目录 `_reapply.sql`。`TestDMDirectoryIndexRollbackPostgres` 验证回退后 prepared 启动被拒、旧路径仍可读，恢复后 prepared 重新可用。
  - 已确认偏差：goose 只应用高于账本的新版本，0043 必须先于 03 的 0044+ 合入。新增 `TestMigrationVersionsAreUnique` 在合并/CI 阶段拒绝重复数字前缀，避免并行分支同号导致其中一个迁移在已升级库上被静默跳过；历史空号（0032 故意缺失）仍允许。
  - 未决取舍：无。
- 更新日期：2026-11-16
- 状态：实现 完成；验证 PASS（功能/安全/分页边界/迁移/查询计划/固定次数/1M 合成规模 p95），合成 lab 性能证据不等同生产验收；合并 未合并；发布 未发布

## 目标与范围

缩短私信会话目录在单身份大量会话下的打开等待，并避免目录读取占用账号/Gate 写锁。

包含：

- 迁移 0043：为 `c_dm.conversations` 增加两个游标索引；Down 按仓库策略 forward-only，索引回退由 `services/api/ops` 脚本显式执行。
- 目录候选改为按身份低/高两侧分别走索引分页，同游标同上限 `UNION ALL` 合并取前 100。
- 仅对选中页批量关联对方身份、来源帖与设置；未读/acknowledged-unread 用一次 `LEFT JOIN LATERAL` 条件聚合。
- 两阶段读取：独立 10s 只读 RepeatableRead 快照准备，最终 C Safety Gate 事务内比较会话 `(id,revision)`、对方/观看者身份与账号版本、来源帖版本、双向账号拉黑与 DM 拉黑、设置版本；新增会话落入已返回区间时重算一次。连续冲突返回 `SERVICE_UNAVAILABLE`，授权/会话失败不得由准备结果泄漏。
- 配置开关 `preparedDmDirectoryReads`（默认关闭）；启动时校验两个索引有效且列序正确、版本表存在。关闭时保留原查询与回退能力。
- 运维回退/恢复脚本 `services/api/ops/0043_dm_directory_cursor_indexes_{rollback,reapply}.sql`：不改变 goose 账本，供放弃优化或重建索引时显式执行。
- 1M 会话合成压测 `TestDMDirectoryScalePostgres`：默认跳过，仅在隔离 lab 显式开启；对比改前基线（旧查询、无 0043 索引）、有索引旧路径与新路径。
- 等价性、安全、SQL 次数、索引可用性、回退脚本与迁移版本唯一性测试。

不包含：

- 不修改移动端、OpenAPI 或 HTTP 状态码/响应结构。
- 不改变未读数、acknowledged/read、隐藏、删除、撤回、静音、置顶、备注、来源帖或拉黑业务语义。
- 不实现 03/07 的评论与通知批量投影。

## 接口与依赖

- 上游基座：需求 01 的两阶段读取（`withPreparedReadPlan`）与 0041/0042 版本表，均已合入 `main@1d829b2`。
- 迁移顺序：本任务 0043；03 因 goose 版本单调性必须使用 0044+，且 0043 先合入。
- 运行时配置：`Config.PreparedDMDirectoryReads` → `authprivacy.DirectMessageConfig.PreparedDirectoryReads`；仅当私信功能开启、C Gate 为原生 `c_auth`、Posts 开启时生效。
- 版本依赖：`c_dm.conversations.revision`（消息/设置触发器推进）、`c_posts.read_identity_versions`、`c_posts.read_account_versions`、`c_posts.read_feed_post_versions`、`c_posts.identity_public_labels`、`c_dm.blocks`、`c_posts.blocked_authors`。

## 验收标准

- 1M 会话、单身份 10/100/1,000 会话（低/高两侧混合）下，新路径首页 p95 相对改前基线（旧查询且无 0043 索引）改善 ≥40%；同时报告相对“有索引旧路径”的变化，不把索引收益重复计入新路径。
- 同一夹具下旧/新路径对首屏与续页返回完全相同的会话 ID、顺序、对方投影、等待态、置顶/静音/备注/隐藏/删除边界、未读与 acknowledged-unread、来源帖标题、`canSend`/`canShare` 与本人拉黑 ID。
- 1M 会话规模概念下，单身份 10/100/1,000 会话，低侧、高侧、混合侧均可分页；每页仍最多 100 条，游标仍为 conversation_id。
- 两个分支的 EXPLAIN 均可使用新增索引（计划包含对应索引名）。
- 准备后注入：新消息（revision 变化）、设置变化、对方身份注销/账号状态、来源帖隐藏、双向账号拉黑、DM 拉黑、观看者身份注销、会话撤销。返回结果必须来自最终授权后的最新快照；会话撤销/冻结优先返回鉴权错误；连续冲突返回 503。
- 固定 SQL 次数：每次成功请求准备阶段 3 条（候选页、选中页投影、证明）、最终阶段 2 条（证明、范围复查），共 5 条；有界重算一次时最多 10 条；不随页内会话数增长。
- 关闭开关或非原生 Gate 时行为与现状一致。

## 当前进度与证据

| 项目 | 被测版本／环境 | 结果与证据 |
| --- | --- | --- |
| 实现 | 工作区 `perf/dm-directory-reads`（基线 `main@1d829b2` + 未提交改动） | 完成：0043 双游标索引；`DirectMessageConfig.PreparedDirectoryReads`/`preparedDmDirectoryReads` 开关；候选 `UNION ALL`、选中页批量投影、版本证明与区间复查；`preparedDmDirectoryReads` 运行时配置与启动 schema 校验 |
| 静态检查 | WSL Ubuntu 24.04，Go 1.22.2 | PASS：改动文件 `gofmt`、`go build ./...`、`go vet ./...` |
| 等价性与安全 | PostgreSQL 16 私有 socket P2 lab，`-race` | PASS：`TestDMChatDirectoryPreparedEquivalencePostgres`（首屏与续页的旧/新 JSON 逐字节一致）；`TestDMChatDirectoryPreparedRebuildPostgres` 六个子项（新消息、区间内新会话、身份注销、账号拉黑、DM 拉黑、会话撤销）均返回最终快照或鉴权错误 |
| 分页边界 | 同上 | PASS：`TestDMChatDirectoryPreparedPageLimitPostgres` 在夹具 3 条会话外新增 400 条会话（低/高两侧各 200），两分支各自超过一页；依次返回 100/100/100/100/3/0 条，ID 顺序与 conversation_id 游标续页正确，前四页旧/新 JSON 逐字节一致 |
| 固定 SQL 次数 | 同上 | PASS：`TestDMChatDirectoryPreparedSQLCountPostgres` 每次成功请求 5 条；阶段 `prepare_read=1`、`finalize_read=1`、`prepared_read_rebuild=0`；重建路径由六个子项覆盖 |
| 索引计划 | 同上 | PASS：`TestDMChatDirectoryPreparedIndexesPostgres` 在关闭顺序/位图扫描后，候选 SQL 的 EXPLAIN 同时命中 `dm_conversations_low_cursor` 与 `dm_conversations_high_cursor` |
| 1M 规模 p95 | PostgreSQL 16 私有 socket P2 lab；合成 1,000,000 会话、1,414 个对方身份；观看者各 10/100/1,000 会话（低/高各半）；每档 30 样本、5 预热、首页 100 条；`DM_CHAT_RACE=0`，`shared_buffers=512MB` | PASS：`TestDMDirectoryScalePostgres`。p95（改前基线 → prepared）：10 会话 807.08 → 10.07ms（+98.8%）；100 会话 803.65 → 13.38ms（+98.3%）；1,000 会话 87.84 → 15.78ms（+82.0%），达到 ≥40%。重复运行同规模为 +98.5%/+97.2%/+81.1%，结论稳定。同档相对“有索引旧路径”为 −4.2%/−38.1%/−31.3%，如实记录 prepared 的两阶段固定开销。首页是旧路径最佳场景；更深页旧路径沿 pkey 走得更远。合成种子，不等同生产验收。 |
| 1M 查询计划 | 同上 | PASS：改前旧路径 `Index Scan using conversations_pkey`，1,000 会话观看者过滤 93,983 行才取满 100 条；候选 SQL 两分支均为 `Index Only Scan using dm_conversations_low_cursor/high_cursor`；范围复查同样命中两索引；证明 SQL 按页主键点查。 |
| 运维回退 | 同上，`-race` | PASS：`TestDMDirectoryIndexRollbackPostgres` 执行 `ops/0043_dm_directory_cursor_indexes_rollback.sql` 后索引数为 0，prepared 启动被 schema gate 拒绝，旧路径仍可读；执行 `_reapply.sql` 后索引数为 2，prepared 读取恢复。账本保持 0043，不依赖 goose Down。 |
| 迁移版本唯一性 | WSL Ubuntu 24.04，Go 1.22.2 | PASS：`TestMigrationVersionsAreUnique` 拒绝重复或不可解析的数字前缀，允许历史空号 0032；防止 03/06 并行分支占用同一版本号后，其中一个迁移在已升级库上被 goose 静默跳过。 |
| DM 回归 | 同上 | PASS：`TestDMChatMediaLifecyclePostgres`、`TestDMChatIndependentPendingLimitsPostgres`、`TestDMPostCardsPostgres`、`TestDMPostCardGovernancePostgres`、`TestDMPostCardLegacyPaginationPostgres`、`TestDMFirstContactConcurrencyAndRuntimePostgres`、`TestDMChatHTTPSPostgres`、`TestDMPostCardHTTPSPostgres`、`TestDMReliableTextPostgres` |
| 迁移链 | 同上（DM lab） | PASS：`TestDMGooseFreshAndUpgradePostgres` fresh/upgrade 均为版本 43 且 Down 保持 forward-only；`TestIdentityAvatarGooseFreshAndUpgradePostgres` fresh/upgrade 均为 43 |
| 迁移链 | 同上（feed lab） | PASS：`TestFeedGooseFreshAndUpgradePostgres` 六个子库（fresh/upgrade/interactions/versions/reposts/history/legacy_search）均为版本 43 |

1M 证据复现（隔离 lab，产物写到源码外）：

```sh
cd /mnt/e/speechful
export PATH=/usr/local/go/bin:/usr/lib/postgresql/16/bin:/usr/bin:/bin
export POST_MEDIA_PYTHON=/home/dev/hnuhole-dev/venv/bin/python
export COMMENT_TEXT_GOOSE=/home/dev/hnuhole-dev/bin/goose
export DM_CHAT_RACE=0 DM_CHAT_SHARED_BUFFERS=512MB
export DM_CHAT_TEST_PATTERN='^(TestDMDirectoryScalePostgres)$'
export DM_DIRECTORY_SCALE=1 DM_DIRECTORY_SCALE_CONVERSATIONS=1000000
export DM_DIRECTORY_SCALE_SAMPLES=30 DM_DIRECTORY_SCALE_WARMUP=5
export DM_DIRECTORY_SCALE_EXPLAIN=1
export DM_DIRECTORY_SCALE_OUTPUT=/tmp/dm-directory-scale-1m.json
sh services/api/authlab/run-dm-chat-isolated.sh
```

压测先在种子后执行 `ops/0043_..._rollback.sql` 测量“旧查询且无游标索引”的改前基线，再执行 `_reapply.sql` 测量有索引旧路径与 prepared；不把索引收益重复计入 prepared。

## 阻碍与下一步

- 无实现阻塞。1M p95 已在合成 lab 达标并保留完整计划/分位数，但仍是单客户端、首页、预热缓存证据，不等同生产验收；上线前应在生产规模快照或专用压测库复测（含更深游标页与冷缓存）。
- 0043 必须先于 03 的 0044+ 合入。`TestMigrationVersionsAreUnique` 只能阻止版本号碰撞，不能改变 goose 只应用更高版本的顺序约束，合并顺序仍需人工协调。
- 0043 Down 按仓库策略保持 forward-only；放弃优化时先关闭 `preparedDmDirectoryReads`，再执行运维回退脚本，恢复用 `_reapply.sql`。
- 当前工作区存在其他 Session 的未跟踪/未暂存改动，本任务未提交、未推送、未创建 PR。
