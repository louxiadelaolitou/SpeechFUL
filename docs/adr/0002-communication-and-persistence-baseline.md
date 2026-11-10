# 通信与持久化基线

状态：accepted（2026-09-22）。Go 服务端使用标准库 `net/http` 配合 `chi`，核心数据使用 PostgreSQL，通过 `pgx` 执行显式 SQL；移动端使用 Drift/SQLite 保存本机业务副本，并用系统安全存储保存敏感凭据。客户端与服务端的业务接口采用 REST/JSON + OpenAPI，前台在线实时事件按需使用 WebSocket，后台通知以后接入 APNs/FCM。

2026-11-10 实装核对：原 sqlc 分类查询生成方案未进入运行链路，当前使用 `channels/postgres.go` 的查询常量和 pgx。清理已退役未使用的生成配置和重复查询源，运行 SQL、正式 Goose 迁移和权限审计不变；见[清理任务](../tasks/STALE-FILE-CLEANUP-20261110.md)。

## 边界

- REST 负责业务命令和查询；WebSocket 不替代业务接口，只负责前台在线事件；离线补收由服务端查询和确认机制完成。
- SQLite 保存草稿、会话索引、消息本地副本和待发送任务；敏感凭据不放入普通业务表。
- 迁移工具与 OpenAPI 生成链已在 [ADR 0003](0003-repository-auth-and-first-slice.md) 确认；对象存储与部署基线见 [ADR 0004](0004-ci-testing-email-and-deployment.md)。聊天本地加密方案、推送供应商和生产部署细节仍待决定。
