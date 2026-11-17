# SpeechFUL 变更日志

记录本仓库的写入型改动，最新条目在最上面；格式与门禁见 `.agents/skills/minimal-change/SKILL.md`。

## 2026-11-17 17:02 · 点击图片复用已加载预览

- 状态：已完成（本地提交，未推送）
- 改动：查看器新增调用方所有的纯视觉 `previewProvider`；帖子、评论、私信打开查看器时先显示已加载图片，原图首帧到达后替换；评论 provider 记忆化，避免重复请求 `/original`。预览不进入来源缓存、`ready` 判定、保存或资源上限，组件不驱逐调用方 provider。
- 文件：`packages/image_viewer/lib/src/viewer_controller.dart`、`viewer_preview.dart`、`viewer_surface.dart`、`viewer.dart`、`packages/image_viewer/README.md`、`packages/image_viewer/test/viewer_contract_test.dart`、`packages/image_viewer/example/test/component_example_test.dart`、`apps/mobile/lib/src/posts/post_images.dart`、`apps/mobile/lib/src/posts/post_media_repository.dart`、`apps/mobile/lib/src/comments/comment_views.dart`、`apps/mobile/lib/src/messaging/dm_chat_ui.dart`、`apps/mobile/test/post_media_repository_test.dart`、`apps/mobile/test/comment_runtime_test.dart`、`docs/design/image-viewer-component-plan.md`、`docs/design/post-image-media-contract.md`、`docs/design/modules/post-detail.md`、`docs/design/modules/comments.md`、`docs/tasks/IMG-VIEWER-PREVIEW-01.md`（新增）
- 验证：环境 Windows 11 25H2 x64 / Flutter 3.47.5 / Dart 3.13.4，基线 `315f151`。查看器包 `flutter analyze --no-pub` PASS、`flutter test --no-pub` 147 项 PASS；移动端 `flutter analyze --no-pub` PASS、`flutter test --no-pub --concurrency=2` 1353 项通过／4 项跳过 PASS、`flutter build bundle --debug --no-pub` PASS；示例 `flutter analyze --no-pub` PASS、`flutter build bundle --debug --no-pub` PASS，测试 6 项通过、2 项失败（iOS 保存门控与重试场景 `pumpAndSettle` 超时），两项已在基线查看器代码复现，非本改动回归；`git diff --check` PASS；锁定文件已恢复。真机慢网／保存／账号切换 `NOT_RUN`。
- 回滚：`git revert <本条目实现提交>`（本地提交，未推送）；无迁移、服务端或发布步骤。
- 任务：`docs/tasks/IMG-VIEWER-PREVIEW-01.md`
- 授权：用户 2026-11-17 指令“现在有一个问题，我们的页面图片明明已经加载出来了，但是直接点击图片却又要再加载一遍……你看看能不能优化成这样”；仅本地实现与验证，未授权推送、PR、合并、发布或部署。

## 2026-11-16 04:24 · 加速通知列表与未读统计读取

- 状态：已完成
- 改动：
  - 新增迁移 0049，为 `c_inbox.records` 增加 `latest_like_version`（当前仍存活的最新点赞事件版本），由 `c_inbox.like_changed()` 在同一记录行锁内维护；`c_inbox.account_closed()` 直接删除点赞事件后同步重算，并先按 `notification_id` 排序锁住受影响记录，避免关闭账号留下幽灵未读。历史数据按 `like_events` 最大值回填（本任务合成库 40,000 行）。
  - 新增部分索引 `c_inbox.inbox_unread`，同时服务未读统计聚合与“只看未读”列表排序；行在 `read_version` 追上最新事件后自动离开索引。
  - `ReadInbox` 按是否只看未读选择专用 SQL，去掉逐行 `unread` 表达式与逐条 `EXISTS c_inbox.like_events` 探测。
  - `ReadInboxCounts` 合并为一次账号级分组查询（同时得到各身份 all/reply/like）加一次系统通知计数；选中身份时取对应分组，不再重复扫描账号全部记录。
  - `readInboxNotice` 仅在 `latest_like_version > read_version` 时统计新增点赞数。
- 文件：`services/api/migrations/0049_notification_read_indexes.sql`（新增）、`services/api/internal/authprivacy/community_inbox.go`、`services/api/internal/authprivacy/community_inbox_test.go`、`services/api/internal/authprivacy/community_inbox_migration_test.go`、`services/api/internal/authprivacy/community_direct_messages_migration_test.go`、`services/api/internal/authprivacy/community_feed_migration_test.go`、`services/api/internal/authprivacy/identity_avatar_migration_test.go`（迁移账本与回填断言）、`.github/workflows/comments-backend-text.yml`（路径过滤）、`docs/tasks/REQ-007-NOTIFICATION-READ-ACCEL.md`（新增）
- 验证：
  - 环境：WSL2 Ubuntu-24.04 / PostgreSQL 16；基线 `main@1d829b2`，分支 `perf/notification-read-accel`，Worktree `E:\speechful-notif-accel`；Go 1.22.2。旧集成提交 `743cc4c`（#4 `a25aa8b` → #6 `44f07a4` → #5，账本 45/45）；合并 #6 冲突修复 head `591b7ba` 后为提交 `633b287`，迁移账本 49/49。
  - 构建：`gofmt -l` 无输出；`go build -mod=readonly ./cmd/authdev`、`go vet -mod=readonly ./internal/authprivacy ./internal/authprivacyhttp` PASS；集成后 `go build ./...`、`go vet` 与 LF 副本 `gofmt -l` 同样 PASS。
  - 真实 PostgreSQL：`TestNotificationInboxPostgres`、`TestNotificationInboxIdentityPaginationPostgres`、`TestNotificationInboxHTTPSPostgres`、`TestInboxRoutesAndDisabledMentionsBoundary`、`TestNotificationInboxGooseMigrationPostgres`（fresh/upgrade，含回填=2、列默认值、索引存在、Down 拒绝、账本 max=49）、`TestDMGooseFreshAndUpgradePostgres`（fresh/upgrade）、`TestFeedGooseFreshAndUpgradePostgres`（7 模式）PASS；`TestIdentityAvatarGooseFreshAndUpgradePostgres` 在独立空 fixture 中 PASS（与其它迁移测试共用 fixture 时会因库非空而按设计失败，非回归）。
  - 集成回归（`743cc4c`，合并 #6 修复前）：DM 目录重建 7 子测试、索引回滚/重放、迁移版本唯一性 PASS；DM、feed（7 模式）、avatar、通知迁移均到 45/45；评论/通知 CI 聚焦集（`TestCommentText*`、`TestCommentBusiness*`、`TestNotificationInbox*`、`TestInboxRoutesAndDisabledMentionsBoundary`、moderation 迁移集）PASS。
  - 合并后复验（`633b287`，合并 #6 head `591b7ba`）：`TestNotificationInboxGooseMigrationPostgres` fresh/upgrade 49；DM fresh/upgrade、feed 7 模式、avatar fresh/upgrade 均到 49；`TestNotificationInboxPostgres`、`TestNotificationInboxIdentityPaginationPostgres`、`TestNotificationInboxHTTPSPostgres`、`TestInboxRoutesAndDisabledMentionsBoundary`、`TestMigrationVersionsAreUnique` PASS。
  - 行为断言：点赞→取消→全部取消时 `latest_like_version` 依次为 1/2→1→0，`version` 保持单调；关闭点赞者账号后其事件删除、`latest_like_version` 归零且未读统计不再计数；运行角色仍不能更新新列（`CheckCommentSchema` 的列级 UPDATE 断言 PASS）。
  - 查询计划 A/B（同一隔离数据集副本：50,000 条互动记录、120,000 条点赞事件、5,000 条系统通知、账号 5 个身份；VACUUM ANALYZE 后热缓存）：未读统计 160.5 ms（选中身份 91.6 ms + 账号级逐身份 68.9 ms）→ 7.4 ms（分组 6.9 ms + 系统 0.5 ms）；全部列表 50.7 ms → 16.9 ms；未读列表 75.1 ms → 6.7 ms。基线每条 LIKE 通知一次 `like_events` 探测（选中 24,000 次、账号级 40,000 次，约 74k/122k shared buffers）；最终统计为 `Index Only Scan using inbox_unread`（324 buffers）。
  - 等价性：各身份未读数、选中身份 all/reply/like、系统计数与基线逐项一致；全部列表与未读列表前 21 条 ID/时间逐行 `diff` 一致。
  - 产品级 P95 与真机链路 `NOT_RUN`（需真实规模数据与客户端联调）。
  - 产品 CI：PR [#5](https://github.com/zewbby/SpeechFUL-Develop/pull/5) 集成前 head `3b25645` 的 21 项检查 20 success、1 skip（`actual-process-acceptance` 按设计跳过）；旧集成 head `38f93f4`（含 0043/0044/0045、账本 45/45）同样 21 项 20 success、1 skip，无 failure，PR 状态 clean；最终 head `1437eb4` 的 13 个 workflow 全部 success，23 项检查 22 success、1 skip（`actual-process-acceptance` 按设计跳过），无 failure。
- 回滚：`git revert 44243ca`（本条目实现提交；随后追加的文档提交仅记录该 SHA）。0049 的 Down 与 0028/0041/0042 一样拒绝回退；线上如需撤销读取路径，应按前向修复发布新的迁移恢复 0028 函数与查询，不要直接 `DROP COLUMN`。仅一次性开发库可在确认无依赖后执行 `DROP INDEX IF EXISTS c_inbox.inbox_unread; ALTER TABLE c_inbox.records DROP COLUMN IF EXISTS latest_like_version;` 并恢复 0028 定义。
- 任务：`docs/tasks/REQ-007-NOTIFICATION-READ-ACCEL.md`
- 授权：用户 2026-11-16 指令“继续完成07优化”（需求行“通知列表与未读统计优化”）；随后指令“完成后向仓库提交pr”及审查要求按 #4 → #6 → #5 集成。已推送 `perf/notification-read-accel` 并创建 PR #5，base 调整为 `perf/comment-load-accel`；PR #5 已由用户合入 `perf/comment-load-accel`（合并提交 `6dbdfb0`），`main@bf5c0ae` 尚未包含 0049；未部署。

## 2026-11-16 03:22 · 为评论首屏与楼层读取补索引

- 状态：已完成
- 改动：
  - 新增迁移 0048，为 `c_comments.likes(comment_id)` 建索引。原 `(account_id,comment_id)` 主键无法服务仅按 `comment_id` 的点赞统计，逐条投影会退化为全表扫描。
  - `readTextComment` 的回复计数补 `z.post_id=t.post_id`。发布触发器（`0016_comments.sql:159`）已保证回复与根同帖，计数结果不变；既有 `comment_task_thread(post_id,root_comment_id)` 因此可直接定位。曾一并评估 `root_comment_id` 单列索引，A/B 显示该路径与“补 `post_id` 复用复合索引”等价且实际更慢，最终不保留，避免重复索引的写放大。
- 文件：`services/api/migrations/0048_comment_read_indexes.sql`（新增）、`services/api/internal/authprivacy/community_comments_read.go`、`services/api/internal/authprivacy/community_direct_messages_migration_test.go`、`services/api/internal/authprivacy/community_feed_migration_test.go`、`services/api/internal/authprivacy/identity_avatar_migration_test.go`（迁移账本断言）、`.github/workflows/comments-backend-text.yml`（路径过滤）
- 验证：
  - 环境：WSL2 Ubuntu-24.04 / PostgreSQL 16；基线 `1d829b2` + 工作区差异。代码补丁 `git patch-id --stable` = `3c4f62d27eba0c37a53ee3260d8b2b57995177bc`；原 0044（现 0048）工作区 SHA-256 = `759d0da8b54313d5a6d8a4dcd9870761a787ded5d6ac9404dd1e3ba257ab90f0`。
  - 迁移：`TestIdentityAvatarGooseFreshAndUpgradePostgres`、`TestDMGooseFreshAndUpgradePostgres` 均 PASS（fresh/upgrade 均到 48；重复 up 无副作用；Down 被拒且账本仍为 48）。`TestCommentTextTransactionsPostgres` PASS（含首屏 `ReplyCount==2` 断言）。`go vet ./internal/authprivacy ./internal/authprivacyhttp`、`go build ./cmd/authdev` PASS。
  - 查询计划（110,000 个任务、100,200 个根、9,800 条回复、10,000 个点赞的隔离库）：回复计数 17.8ms / cost 1394.70 / 13,095 buffers → 11.7ms / cost 165.90 / 8,263 buffers，计划为 `Index Cond: post_id AND root_comment_id`；同库 A/B 中 50 条点赞统计 14.2ms → 3.9ms，49 条楼层预览 11.7ms → 1.3ms，均走 `Index Only Scan using comment_likes_comment`。
  - 等价性：对 100,200 个根逐条比对新旧回复计数，0 差异。
  - 产品级首屏/楼层 P95 与真机链路 `NOT_RUN`（需真实规模数据与客户端联调）。
  - 产品 CI：PR [#6](https://github.com/zewbby/SpeechFUL-Develop/pull/6) 冲突修复后 head `1dc8aa0`（合并 `main@3746d5a`）的 22 项检查 21 success、1 skip（`actual-process-acceptance` 按设计跳过），无 failure；PR `mergeable: clean`。
- 回滚：`git revert $(git log --format=%H --grep='^perf(comments): accelerate first-screen and floor reads$' -1)`；目标库执行 `DROP INDEX IF EXISTS c_comments.comment_likes_comment;`。0048 的 Down 按 0041/0042 约定拒绝回退，需显式前向修复。
- 任务：`docs/tasks/REQ-003-COMMENT-LOAD-ACCEL.md`
- 授权：用户 2026-11-16 在定位卡后指令“你就修就行了，修好再停下来”，随后确认并要求补交 PR。已推送 `perf/comment-load-accel` 并创建 PR #6；未合并、未部署。
