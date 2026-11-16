# REQ-003 — 评论首屏与楼层加载加速（需求清单第 7 项）

- 模式：常规
- 负责人／状态维护人：当前 Session
- 执行 Session（指挥模式）：无
- 分支／Worktree／PR：`perf/comment-load-accel`，Worktree `E:\speechful-comment-accel`，基线 `main@1d829b2`；实现提交 `4f824cd`；集成提交 `b8f6b24`（合入 PR #4）；集成 head `44f07a4`；PR [#6](https://github.com/zewbby/SpeechFUL-Develop/pull/6)（open，未合并，基线调整为 `perf/dm-directory-reads`）
- 本轮规则版本（指挥模式）：不适用
- 授权依据与边界：用户 2026-11-16 在定位卡后指令“你就修就行了，修好再停下来”，随后确认“03和07你都提交pr了么”并要求补交 PR。仅优化评论首屏与楼层读取；不修改 HTTP、OpenAPI 或客户端契约；已推送 `perf/comment-load-accel` 并创建 PR #6；不合并、不部署、不打 tag、不发布、不 force push。迁移 0043 由 REQ-006 占用，本任务使用 0044。
- 需求确认：来源为用户提供的需求清单第 7 项“评论首屏与楼层加载加速”，对应 REQ-001 中“需求 003 独立索引／评论查询优化”。已核实连续模式下客户端按可见楼层懒加载（`apps/mobile/test/comment_views_test.dart:2131`），不是本次改动点；服务端首屏与楼层共同的放大点有两处：点赞数只按 `c_comments.likes.comment_id` 过滤，而主键前缀是 `account_id`；回复数只按 `root_comment_id` 过滤，既有复合索引以 `post_id` 为前缀。前者新增单列索引；后者给查询补上恒真的 `post_id` 条件，复用 `comment_task_thread(post_id,root_comment_id)`。曾评估 `root_comment_id` 单列索引，A/B 显示它与查询改写路径等价且实际更慢，故不保留。
  - 未决取舍：无（索引与等价谓词不改变可见性、拉黑、图片、replyTo 语义）。
- 更新日期：2026-11-16
- 状态：实现 完成；验证 PASS（迁移 44/44、真实 SQL、查询计划、等价性、与 #4 的组合回归）；产品级 P95 NOT_RUN；合并 未合并（待 #4 → #6 → #5 顺序）；发布 未发布

## 目标与范围

缩短评论首屏和楼层（回复预览／展开）的数据库读取时间，且不改变任何响应内容与可见性判定。

包含：

- 迁移 0044：为 `c_comments.likes(comment_id)` 增加单列索引。
- `readTextComment` 回复计数补 `z.post_id=t.post_id`（`community_comments_read.go:296`），使既有 `comment_task_thread(post_id,root_comment_id)` 直接定位；发布触发器已保证回复与根同帖，结果等价。
- 更新完整迁移链测试的账本断言与 comments CI 路径过滤。
- 用真实 PostgreSQL 验证迁移可应用、Down 拒绝回退、索引存在、关键统计查询命中，以及新旧回复计数等价。

不包含：

- 不新增 `c_comments.tasks(root_comment_id)` 单列索引。隔离库 A/B 中该索引与“补 `post_id` 后复用复合索引”访问路径等价，且 11 万任务规模下实测更慢；保留会形成重复索引与写放大。
- 不修改 `readTextComment` 的逐条投影结构；批量投影留作后续独立步骤。
- 不修改移动端、OpenAPI、HTTP 状态码或响应结构。
- 不改变回复数、点赞数、拉黑、删除、审核隐藏或 replyTo 的语义。

## 接口与依赖

- 迁移顺序：REQ-006 的 0043 已合入本分支（集成提交 `b8f6b24`），账本收敛为 `44/44`；0045（PR #5）在本 PR 之后叠加，届时为 `45/45`。合并顺序固定为 #4 → #6 → #5。
- 读取路径：`community_comments_read.go:296`（回复计数谓词、点赞统计）、`community_comment_snapshots.go:99`、`community_comments_business.go`；点赞索引对 legacy、snapshot、prepared 三条读取路径同时生效。
- 运行时：不新增配置开关；新增谓词恒真（发布触发器 `0016_comments.sql:159` 强制 `reply.post_id = root.post_id`），无需数据回填。

## 验收标准

- 全新库与升级库执行 `goose up` 均成功到 0044；重复 `up` 无副作用；`goose down` 被 0044 拒绝且账本仍为 44。— PASS
- `c_comments.comment_likes_comment` 存在；不创建 `c_comments.comment_task_root`。— PASS
- 回复计数计划使用 `comment_task_thread` 的 `(post_id,root_comment_id)` 条件；点赞统计使用 `comment_likes_comment` 的 Index Only Scan。— PASS
- 新旧回复计数在隔离数据上 0 差异；既有评论读取测试保持通过。— PASS
- 产品级首屏／楼层 P95 与真机链路。— NOT_RUN

## 当前进度与证据

| 项目 | 被测版本／环境 | 结果与证据 |
| --- | --- | --- |
| 实现 | 基线 `1d829b2` + 工作区差异；代码补丁 `git patch-id --stable` = `3c4f62d27eba0c37a53ee3260d8b2b57995177bc` | 0044、回复计数谓词、账本断言、CI 路径已改；0044 工作区 SHA-256 `759d0da8b54313d5a6d8a4dcd9870761a787ded5d6ac9404dd1e3ba257ab90f0` |
| 迁移 fresh/upgrade | WSL2 Ubuntu-24.04 / PostgreSQL 16；集成提交 `b8f6b24` | PASS：评论链 `TestCommentTextGooseMigrationChainPostgres` fresh/upgrade 通过；`TestDMGooseFreshAndUpgradePostgres`、`TestIdentityAvatarGooseFreshAndUpgradePostgres`、`TestFeedGooseFreshAndUpgradePostgres` 七个环境均为账本 44、`down` 被拒且账本不变 |
| 集成回归 | 同上；`b8f6b24` | PASS：DM 迁移 44/44、迁移版本唯一性、短页新增会话重建、运维回退/恢复；feed 迁移七个环境 44/44；评论/通知/审核 CI 聚焦集全部通过（含真实图像校验与 HTTP 组件） |
| 构建与静态检查 | 同上；`b8f6b24` | PASS：`go build ./...`、`go vet ./internal/authprivacy ./internal/authprivacyruntime ./internal/comments ./internal/authprivacyhttp`；改动文件按 LF 副本 `gofmt` 无差异 |
| 真实 SQL 行为 | 同一隔离 fixture；`TestCommentTextTransactionsPostgres` | PASS：首屏读取含 `ReplyCount==2` 断言 |
| 查询计划 | 110,000 个任务、100,200 个根、9,800 条回复、10,000 个点赞的单帖隔离库 | PASS：回复计数 17.8ms / cost 1394.70 / 13,095 buffers → 11.7ms / cost 165.90 / 8,263 buffers，`Index Cond: post_id AND root_comment_id`；同库 A/B 点赞统计 14.2ms → 3.9ms、楼层预览 11.7ms → 1.3ms，均 `Index Only Scan using comment_likes_comment` |
| 等价性 | 100,200 个根、9,800 条回复 | PASS：新旧回复计数逐根比对 0 差异 |
| 构建与静态检查 | 同上 | PASS：`go vet ./internal/authprivacy ./internal/authprivacyhttp`、`go build ./cmd/authdev` |
| 产品 CI | PR #6 集成 head `44f07a4` | PASS：21 项检查 20 success、1 skip（`actual-process-acceptance` 按设计跳过），无 failure；`go-postgres`、`feed-postgres`、`flutter`、`policy` 等均 success。另有 1 个重复 `policy` 运行被 concurrency 取消，不影响结果；PR 状态 clean |
| 产品级 P95 | 真实规模数据与客户端链路 | NOT_RUN |

## 阻碍与下一步

- 已合入 0043（集成提交 `b8f6b24`），账本 44/44；0045（PR #5）在本 PR 之后叠加为 45/45。本 PR 以 `perf/dm-directory-reads` 为基线，合并顺序固定为 #4 → #6 → #5；集成 head `44f07a4` 的 CI 已全绿。
- 产品级首屏／楼层 P95 与真机链路尚未验证，需真实规模数据与客户端联调。
- `readTextComment` 的逐条投影（每评论 3–6 条 SQL）仍是后续独立优化项；本轮不扩大范围。
