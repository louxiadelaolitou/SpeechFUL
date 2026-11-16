# SpeechFUL 变更日志

记录本仓库的写入型改动，最新条目在最上面；格式与门禁见 `.agents/skills/minimal-change/SKILL.md`。

## 2026-11-16 03:22 · 为评论首屏与楼层读取补索引

- 状态：已完成
- 改动：
  - 新增迁移 0044，为 `c_comments.likes(comment_id)` 建索引。原 `(account_id,comment_id)` 主键无法服务仅按 `comment_id` 的点赞统计，逐条投影会退化为全表扫描。
  - `readTextComment` 的回复计数补 `z.post_id=t.post_id`。发布触发器（`0016_comments.sql:159`）已保证回复与根同帖，计数结果不变；既有 `comment_task_thread(post_id,root_comment_id)` 因此可直接定位。曾一并评估 `root_comment_id` 单列索引，A/B 显示该路径与“补 `post_id` 复用复合索引”等价且实际更慢，最终不保留，避免重复索引的写放大。
- 文件：`services/api/migrations/0044_comment_read_indexes.sql`（新增）、`services/api/internal/authprivacy/community_comments_read.go`、`services/api/internal/authprivacy/community_direct_messages_migration_test.go`、`services/api/internal/authprivacy/community_feed_migration_test.go`、`services/api/internal/authprivacy/identity_avatar_migration_test.go`（迁移账本断言）、`.github/workflows/comments-backend-text.yml`（路径过滤）
- 验证：
  - 环境：WSL2 Ubuntu-24.04 / PostgreSQL 16；基线 `1d829b2` + 工作区差异。代码补丁 `git patch-id --stable` = `3c4f62d27eba0c37a53ee3260d8b2b57995177bc`；0044 工作区 SHA-256 = `759d0da8b54313d5a6d8a4dcd9870761a787ded5d6ac9404dd1e3ba257ab90f0`。
  - 迁移：`TestIdentityAvatarGooseFreshAndUpgradePostgres`、`TestDMGooseFreshAndUpgradePostgres` 均 PASS（fresh/upgrade 均到 44；重复 up 无副作用；Down 被拒且账本仍为 44）。`TestCommentTextTransactionsPostgres` PASS（含首屏 `ReplyCount==2` 断言）。`go vet ./internal/authprivacy ./internal/authprivacyhttp`、`go build ./cmd/authdev` PASS。
  - 查询计划（110,000 个任务、100,200 个根、9,800 条回复、10,000 个点赞的隔离库）：回复计数 17.8ms / cost 1394.70 / 13,095 buffers → 11.7ms / cost 165.90 / 8,263 buffers，计划为 `Index Cond: post_id AND root_comment_id`；同库 A/B 中 50 条点赞统计 14.2ms → 3.9ms，49 条楼层预览 11.7ms → 1.3ms，均走 `Index Only Scan using comment_likes_comment`。
  - 等价性：对 100,200 个根逐条比对新旧回复计数，0 差异。
  - 产品级首屏/楼层 P95 与真机链路 `NOT_RUN`（需真实规模数据与客户端联调）。
  - 产品 CI：PR [#6](https://github.com/zewbby/SpeechFUL-Develop/pull/6) head `4f824cd` 的 21 项检查 20 success、1 skip（`actual-process-acceptance` 按设计跳过），无 failure。
- 回滚：`git revert $(git log --format=%H --grep='^perf(comments): accelerate first-screen and floor reads$' -1)`；目标库执行 `DROP INDEX IF EXISTS c_comments.comment_likes_comment;`。0044 的 Down 按 0041/0042 约定拒绝回退，需显式前向修复。
- 任务：`docs/tasks/REQ-003-COMMENT-LOAD-ACCEL.md`
- 授权：用户 2026-11-16 在定位卡后指令“你就修就行了，修好再停下来”，随后确认并要求补交 PR。已推送 `perf/comment-load-accel` 并创建 PR #6；未合并、未部署。
