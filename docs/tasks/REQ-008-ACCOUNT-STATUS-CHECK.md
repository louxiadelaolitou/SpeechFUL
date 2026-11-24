# REQ-008 — 社区账号状态检查优化

- 模式：常规
- 负责人／状态维护人：当前 Session
- 执行 Session（指挥模式）：无
- 分支／Worktree／PR：`perf/account-status-check`，Worktree `E:\speechful-account-status`；基线 `develop/main@bc599b3`（远端 `zewbby/SpeechFUL-Develop` 的 `main`）；实现提交 `de6303c`；[PR #31](https://github.com/zewbby/SpeechFUL-Develop/pull/31)，base `main`，创建时 head `87451f8`（本记录提交后顺延）
- 本轮规则版本（指挥模式）：不适用
- 授权依据与边界：用户 2026-11-23 提供《SpeechFUL-社区账号状态检查优化实施方案-20261123.md》并指令“阅读并完成”。仅执行方案中已确认的评论事务 SQL 去重；阶段二、三按方案自带启动条件以证据判断，不满足即记录理由并关闭。用户 2026-11-24 授权推送分支并创建 PR；合并、部署仍未授权。不修改 HTTP、OpenAPI、数据库 schema、移动端或 Redis。
- 需求确认：方案已给出三个阶段的边界、30 秒参数和“可只交付有实证收益阶段”的规则；阶段二、三的启动条件是可核实事实，按方案执行，不产生新的业务取舍。未决取舍：无。
- 更新日期：2026-11-24
- 状态：实现 完成（阶段一；阶段二、三按条件关闭）；验证 PASS（阶段一自动化 + Xiaomi 15 真机评论链路）、SKIP（阶段二、三按条件不实施）；PR #31 CI PASS（`ebedd2d`：21 success / 1 条件 skip / 0 fail）；合并 未合并；发布 未发布

## 目标与范围

减少评论创建/重试与评论任务详情在同一事务内对账号状态的重复读取，不改变封禁、禁言、会话、身份、目标可见性和幂等回执语义。

包含：

- `community_comments.go` 的 `executeCommentCommandOnce → applyCommentIntent`：删除与最终会话校验重复的 `SELECT state FROM c_auth.accounts WHERE account_id=$1` 及其不可达的 `ACCOUNT_CLOSING` 分支。
- 同文件 `GetOwnCommentTask`：在 `v.CanRetry && !mutedAt(m, d.TrustedAt)` 分支直接执行原有 `checkCommentContext`，删除同一账号状态 SELECT。
- 新增 `TestCommentAccountStateReadReusePostgres`，覆盖禁言中、禁言到期可重试、目标已删除、已发布四种任务状态，并用 pgx tracer 断言目标 SQL 为 0；同时将 `TestCommentAccount` 加入 `comments-backend-text.yml` 的 `COMMENT_TEST_PATTERN` 与 `run-comment-text-isolated.sh` 默认 pattern，使该回归在 CI 执行。

不包含：

- `applyCommentMutation` 中点赞/删除路径的同类账号状态读取。方案将最小改动限定在上述两个调用点；该分支保留自己的 `PERMISSION_DENIED` 判断，本次未扩大范围。
- 移动端提示合并与 Redis 缓存。阶段二、三不满足方案启动条件，依据见下文。
- HTTP 状态码、OpenAPI、响应字段、数据库迁移、部署配置。

## 接口与依赖

- 无对外接口变更。评论命令回执与 `OwnCommentTask.allowedActions` 的既有顺序和内容保持不变。
- 前置不变量：`withCommentSession → withPostSessionAtPrepared` 在 C Safety Gate 的最终回调中先执行 `session.validate(a, final)`；账号行在该事务内以 `FOR UPDATE` 锁定，`validate` 要求 `a.state == "ACTIVE"` 且会话代次、撤销、封禁和到期均通过，之后才调用业务回调。因此回调内按同一账号 ID 再读 `state` 只会得到刚校验过的 `ACTIVE`。
- `ACCOUNT_CLOSING` 在评论回执映射中原本就落入默认的 `PERMISSION_DENIED`，且删除前不可达；删除该分支不改变可观察回执。
- 后台 `PublishAcceptedComments` 使用另一笔事务，其 `SELECT ... FOR UPDATE` 和发布规则未改动。

## 验收标准

- 实际走到 `applyCommentIntent` 的创建或合法重试不再执行 `SELECT state FROM c_auth.accounts WHERE account_id=$1`。— PASS
- 可重试任务详情不再执行同一条 SELECT；禁言中、目标删除后、已发布任务仍不返回 `RETRY`，禁言到期后仍返回 `RETRY`。— PASS
- 删除、点赞、放弃、此前已返回回执的重放保持原行为；外层锁、锁顺序、C Safety Gate、会话代次和既有续期路径不变。— PASS（放弃与重放由既有测试覆盖；点赞/删除的专用业务测试因缺少真实图片校验器 SKIP，代码路径未改动）
- 隔离 PostgreSQL 测试通过；全量 Go 测试、vet、build 和改动文件 gofmt 通过。— PASS
- 真机评论链路：用任务分支构建的服务端和 arm64 debug APK，在同一账号下创建评论，重新登录后评论仍可见。— PASS（Xiaomi 15）
- 真机失败任务重试 UI（禁言到期、目标删除）未被界面路径触发；同一分支由隔离 Go 测试覆盖。— SKIP
- 阶段二：若存在同一提示的重复取数，才实施客户端 30 秒合并；否则记录轨迹并关闭。— SKIP（无该提示接口）
- 阶段三：若能指出 Redis 命中替代的具体 PostgreSQL 读取，才实施；否则关闭。— SKIP（无可替代读取）
- 产品级 P50/P95、真实规模数据与 30 秒缓存行为。— NOT_RUN（阶段二未实施，无缓存行为；本轮只做功能真机烟测）

## 当前进度与证据

| 项目 | 被测版本／环境 | 结果与证据 |
| --- | --- | --- |
| 实现 | 基线 `bc599b3` + 实现提交 `de6303c` | 删除两处重复 SELECT；新增回归测试与精确 SQL tracer；仅改动 2 个 Go 文件 |
| 改前回归 | 基线源码 `develop/main@bc599b3` + 新测试 | FAIL：`duplicate account state read 3`（脚本流程内 3 次重复读取） |
| 改后回归 | `de6303c` | PASS：同一脚本流程目标 SQL 为 0 |
| 定向隔离测试 | WSL2 Ubuntu-24.04 / PostgreSQL 16；`services/api/authlab/run-comment-text-isolated.sh`，`COMMENT_TEST_PATTERN=^(TestCommentTextTransactionsPostgres\|TestCommentTextFailureRetryAbandonPostgres\|TestCommentTextFrozenGatePostgres\|TestCommentAccountStateReadReusePostgres)$` | PASS：4 项全部通过；评论 HTTP 包无匹配测试，按 runner 正常跳过 |
| CI 覆盖 | `.github/workflows/comments-backend-text.yml`、`services/api/authlab/run-comment-text-isolated.sh` | 新增回归已纳入 `COMMENT_TEST_PATTERN`，在 `text-component` 作业执行；结果以 PR 实时 CI 为准 |
| 完整评论区 CI 模式 | 同一隔离 fixture；runner 默认 pattern | PASS：`TestCommentTextTransactionsPostgres`、`TestCommentTextFailureRetryAbandonPostgres`、`TestCommentTextFrozenGatePostgres`、`TestCommentTextRuntimeAndSQLGuardsPostgres`、`TestCommentTextUnavailablePostPostgres`、`TestCommentTextClosureCleanupWithFeatureDisabledPostgres`、迁移链、帖子 schema 与 moderation 升级均 PASS；`TestCommentTextHTTPSPostgres` 的 original/prepared 均 PASS；`TestCommentBusinessMediaDeleteLikesNoticesPostgres` 与 `TestCommentBusinessHTTPSPostgres` 因“dedicated real validator not configured” SKIP |
| 全量 Go | 同一 WSL2；`go test -mod=readonly -count=1 ./...` | PASS：所有包通过；无 DSN 的集成测试按既有逻辑 skip |
| 静态检查 | 同一 WSL2；`go vet -mod=readonly ./...`、`go build -mod=readonly ./...`、改动文件 LF 副本 `gofmt` | PASS：vet 无输出、build 成功、两个改动文件 gofmt 无差异 |
| 真机评论创建与持久化 | Xiaomi 15（`24129PN74C`）/ Android 17 / API 37 / arm64-v8a；服务端由任务分支 `961a5ab`（含 `de6303c`）构建，binary SHA-256 `b5798808…`；一次性开发 C/V 服务与一次性开发库；debug APK `1.2.0 (2026111701)`，2026-11-24 04:13 安装；账号 `synthetic_viewer`（身份“南风”） | PASS：在帖子 `82079d7c-f3bb-4061-af4b-9f45de1415a6` 发布评论 `device-smoke-20261124`；界面“评论”计数 0→1，正文与作者显示；开发库 `c_comments.attempts` 版本 1 为 `PUBLISHED`，`accepted_at 04:15:58`、`terminal_at 04:15:59`；重新登录后该帖仍为 1 条评论且正文可见。截图 `device-comment-{07,10,16}-*.png` 留存于本地未提交 `.zcode/` |
| 真机任务详情读取 | 同一服务端与账号；`GET /api/v1/me/comment-tasks/907a5f5d-3730-47ef-849e-f825ac01ba67` | PASS：HTTP 200，`state=PUBLISHED`，`allowedActions=[]`，没有触发账号状态重复读取或回执变化 |
| 开发环境修复 | 本机 WSL2 一次性开发库 | 启动任务分支服务前，开发库停在 0054 之前且运行时缺 0053 新增的 `password_login_limits` 权限；已前滚 0054 并重跑 `grant-*`（本地开发环境修复，不是产品 schema 或仓库迁移改动）。首次启动的 `authentication process failure` 由此产生，修复后 C/V `health/ready` 均为 ready |
| 远端分支与 PR | 远端 `zewbby/SpeechFUL-Develop` | 分支 `perf/account-status-check` 已推送；[PR #31](https://github.com/zewbby/SpeechFUL-Develop/pull/31) 已创建，base `main`；22 项检查：21 success、1 按 `workflow_dispatch` 条件 skipped（`actual-process-acceptance`）、0 fail、无运行中项（`ebedd2d`，2026-11-24） |
| 阶段二判定 | 基线移动端源码 | SKIP：无可复用的全局“账号是否受限”提示接口，详见下表 |
| 阶段三判定 | 基线服务端源码 | SKIP：无“Redis 命中替代某条 PostgreSQL 读取”的候选，详见下文 |
| 产品级延迟 | 真实规模数据 | NOT_RUN（真机只做功能烟测，未采集 P50/P95） |

### 阶段二：移动端一次浏览/提交的提示请求轨迹

| 用户动作 | 客户端调用 | 是否返回可复用的账号限制提示 |
| --- | --- | --- |
| 启动恢复会话 | `GET /api/v1/auth/session`（`http_auth_api.dart:749`） | 否；仅 `accountId`、`username`、`expiresAt`（`community-auth-api.yaml` 的 `SessionResponse`） |
| 临近到期的界面活动 | `POST /api/v1/auth/session-renewals`（`http_auth_api.dart:770`） | 否；仅返回新 `expiresAt`。`auth_session_controller.dart:255` 的 `onActivity` 只在已认证、非忙且距到期 ≤7 天时续期，不能按界面活动次数统计为提示请求 |
| 首页列表 | `GET /api/v1/channels/{channelId}/posts`（`http_post_api.dart:689`） | 否 |
| 帖子详情 | `GET /api/v1/posts/{postId}`（`http_post_api.dart:710`） | 否 |
| 评论/回复 | `GET /api/v1/posts/{postId}/comments` 及楼层/回复子路径（`comment_views.dart:838` 等） | 否 |
| 打开发帖页 | `GET /api/v1/me/identities` + `GET /api/v1/me/post-composer-context`（`post_controller.dart:859`、`http_post_api.dart:912`） | 否；仅身份目录与默认身份选择 |
| 查看某帖操作按钮 | `GET /api/v1/me/posts/{postId}/capabilities`（`http_post_api.dart:879`） | 否；仅该帖的 `canDelete`/`canEdit` |
| 提交评论 | `PUT /api/v1/comment-commands/{commandId}`（`comment_http.dart:289`） | 仅在拒绝时返回 `POSTING_RESTRICTED` 等错误码；不是可预先复用的提示 |
| 评论任务详情/失败页 | `GET /api/v1/me/comment-tasks/{taskId}`（`comment_http.dart:345`） | `allowedActions` 可能含 `RETRY`，但同时依赖任务状态、版本、目标评论可见性和禁言，属于资源级判定，不是全局账号提示 |

结论：移动端没有为“上次核对时账号是否受发言限制”单独取数的消费者，也没有两个组件在同一会话短时间内请求同一份全局提示。提示只来自一次实际写操作的权威拒绝，或单条评论任务的 `allowedActions`；后者包含帖子/评论可见性，不能按账号级提示缓存。按方案第 5 节“若界面本来只读本地状态……则记录原始轨迹，结束这一阶段”，阶段二关闭，不新增 30 秒客户端缓存或接口调用。

### 阶段三：Redis 判定

- 阶段二未产生客户端合并后的提示读取，因此不存在“合并后仍有可观重复提示读取”的进入条件。
- 受保护读写路径（列表、详情、评论、发帖、评论命令）都已在最终事务内锁定账号行、处罚行和会话行以完成 C Safety Gate 校验；额外一次 Redis GET 不能替代这些锁和 SQL，只会增加一次往返。
- `GetOwnCommentTask` 剩余读取依赖任务版本、身份绑定和目标评论状态，属于事务内资源级判定，不能由账号级 Redis 值替代。
- 因此未新增 Redis 键、连接、失效逻辑或淘汰策略；阶段三关闭。

## 阻碍与下一步

- 已推送分支并创建 [PR #31](https://github.com/zewbby/SpeechFUL-Develop/pull/31)；合并、部署仍未获授权，需另行确认。
- CI 结果对应 `ebedd2d`（产品代码与 `de6303c` 相同）；此后仅回填证据文档，按 CONTRIBUTING 不使产品验证失效，最终 head 的适用 CI 以 PR 实时状态为准。
- `TestCommentBusinessMediaDeleteLikesNoticesPostgres` 因未配置真实图片校验器 SKIP，点赞/删除的专用正向覆盖未在本轮执行；相关代码路径未改动。
- `applyCommentMutation` 仍保留同类账号状态读取。它不在方案限定的两个最小改动点内，且拒绝码与执行分支不同；若后续要覆盖点赞/删除，需单独确认并补充行为对照。
- 真机评论创建、评论持久化和评论任务详情读取已 PASS。用户最初点名小米 8，实际接入的是 Xiaomi 15（`24129PN74C`），经用户确认按该设备继续；本记录不把它外推为小米 8 或其他机型结果。
- 真机未通过界面触发失败任务重试分支（需要可控的禁言到期或目标删除时序）；该分支仍以隔离 Go 测试为证据。
- 真机测试期间另一次命令行登录替换了 App 会话，App 按既有单会话机制进入“请重新核对登录状态”，重新登录后恢复；不是本次评论改动的回归。
- 产品级 P50/P95 仍 NOT_RUN；本轮可核实的收益是脚本流程内 3 次重复 SELECT 降为 0。
- 阶段二、三仅在出现可复用的账号提示接口或可明确归因的重复读取时重新评估。
