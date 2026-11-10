# 代码与开发导览

## 目录职责

| 路径 | 职责与入口 |
| --- | --- |
| `apps/mobile` | Flutter App；`lib/main.dart` 装配模块，`lib/src` 按认证、身份、帖子、评论、私信等组织 |
| `apps/admin` | 原生 HTML/CSS/JavaScript 审核界面，由 C 的同源 `/admin` 提供 |
| `apps/website` | 静态官网；页面内手机演示使用本地虚构数据，不连接社区 API |
| `services/api` | Go 后端；`cmd/api` 启动社区 C，`cmd/verifier` 启动验证 V |
| `packages` | 原生安全存储、Passkey、媒体、图片查看器、OpenAPI 与共享协议向量 |
| `infra` | 数据库角色／授权、容器和部署配置 |
| `tools` | 当前静态检查、开发与设备工具；具体环境要求见各入口说明 |
| `docs` | 当前文档导航、设计、ADR、任务和有版本边界的历史资料 |

## 跟踪一次首页加载

1. `apps/mobile/lib/main.dart` 创建 `HttpPostUploadApi`（继承 `HttpPostApi`）、`PostController` 并传给 `FeedScreen`。
2. `lib/src/posts/feed_screen.dart` 的 `initState()` 调用 `loadTimeline()`。
3. `feed_controller.dart` 先核对账号分类屏蔽设置，再通过 `feed_http.dart` 请求 `GET /api/v1/feed`；默认每页 20 条。
4. `http_post_api.dart` 的 `_send()` 向固定 C HTTPS origin 发送 Bearer 请求并严格解析响应。
5. `services/api/internal/authprivacyhttp/posts_endpoints.go` 解析查询，调用 `Community.ReadPosts()`。
6. `internal/authprivacy/community_posts.go` 在会话与最终 Gate 授权事务内执行查询；`community_posts_read.go` 筛选帖子 ID，再读取正文、作者和浏览量等数据。
7. Controller 接收当前授权窗口的响应、持久更新会话截止并通知页面；随后批量读取点赞、收藏和评论数。

`authprivacy` 包目前也包含社区业务事务。`internal/posts` 等目录包含规则和协议，但不能仅在其中寻找全部 SQL。`PostController` 的多个 `part` 文件共享状态，阅读时要结合主文件。

## 开发与验证

先按 [CONTRIBUTING](../CONTRIBUTING.md) 核对 Git 状态并建立任务分支。使用已有工具链与锁文件，不为了阅读仓库安装 SDK 或恢复历史运行环境。

| 层次 | 常用入口 | 证明范围 |
| --- | --- | --- |
| Go 构建与 host 测试 | 在 `services/api` 运行 `go build ./...`、`go test ./...`、`go vet ./...` | 无隔离数据库的测试不证明 SQL 已验收，注意测试输出中的 SKIP |
| 真实 C/V、SQL 与升级 | [API README](../services/api/README.md) 中的 `authlab` 隔离 runner；[评论运行说明](../services/api/comment-schema/README.md) | 使用本次专属一次性库；正式迁移和 grants 分别执行 |
| Flutter 单元与组件 | 在 `apps/mobile` 运行 `flutter pub get --enforce-lockfile`、`flutter analyze --no-pub`、`flutter test --no-pub` | host 行为，不代替原生设备、真实 HTTPS 或 SQL |
| 原生与跨进程 | `apps/mobile/integration_test`、对应设备工具和任务记录 | 必须具备实际设备、证书、固定端点和资源归属条件 |
| 静态与契约 | `packages/openapi/check-specs.py`、相关 `tools/check-*-contract.py` | 结构／协议检查，不代替真实行为测试 |
| 协作规范 | `python -B -m unittest discover -s tools/tests -p test_collaboration.py -v`、`python tools/check-collaboration.py --staged` | 提交、路径与敏感文件规则，不是产品验收 |
| 网页 | [admin 检查](../apps/admin/README.md)、[website 检查](../apps/website/README.md) | 按各自 README 执行 |

Go／PostgreSQL／Gate 文件和 POSIX runner 在 Windows 上使用 WSL2 的 Linux 文件系统；Flutter／Android 可在 Windows 宿主机单独构建。不要把宿主机测试结果当成真实数据库或整机通过。

## 测试与 SQL 为什么分开

- Go 同包 `*_test.go` 与 Flutter 的 `test`、`integration_test` 各有发现和访问规则，保持原布局。
- 单元、HTTP、SQL、进程和设备测试可以覆盖同一功能的不同边界，是否冗余要看行为断言，而不是名称。
- `services/api/migrations` 和 `verifier-migrations` 是 C/V 的正式升级历史，不能因为已执行就删除或重编号。
- `infra/postgres/grant-*.sql` 管运行权限，审计 SQL 检查角色边界，二者都不替代建表迁移。
- 本地 SQLite 的初始化与升级由 Dart 存储实现维护；早期 F2 设计锁、专用检查和 SQL 快照已退役，通过 Git 追溯，避免另存一份会漂移的 schema。

适用的产品 CI 在 `.github/workflows`；不要用历史回执自检代替当前源码测试。旧交接检查的退役范围见[本次整理任务](tasks/REPOSITORY-CLEANUP-01.md)。
