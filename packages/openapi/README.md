# OpenAPI 契约

本目录包含完整 OpenAPI 文档和分阶段组件契约。下表按业务提供入口；文件中的早期“评审稿”或恢复凭据说明不代表当前开放流程。认证的后续确认见 [AUTH-EMAIL-RESET-01](../../docs/tasks/AUTH-EMAIL-RESET-01.md)，具体实现和验收见对应任务与 HTTP 回归。

| 契约 | 范围与状态 |
| --- | --- |
| [feed-api.yaml](feed-api.yaml) | 跨通道时间列表、账号屏蔽、真实浏览；FEED-INTERACTIONS-01新增账号点赞／收藏与评论数摘要，严格feed／旧post响应不变；前端接线见[说明](../../docs/design/feed-backend-contract.md) |
| [channel-api.yaml](channel-api.yaml) | 当前通道目录切片的接口唯一来源；服务端和客户端据此实现目录 |
| [identity-api.yaml](identity-api.yaml) | B02本人身份列表／创建／改名／删除与账号归属幂等结果核对；业务联动随对应模块实现 |
| [verifier-auth-api.yaml](verifier-auth-api.yaml) | V 的校邮资格、原结果核对和内部收据；登录／重置的用途及当前边界结合对应任务和实现核对 |
| [community-auth-api.yaml](community-auth-api.yaml) | C 的开户、登录、会话、注销及凭据协议；保留的恢复码／Passkey 接口不表示当前用户流程开放 |
| [post-api.yaml](post-api.yaml)、[post-image-api.yaml](post-image-api.yaml) | 帖子命令、读取和受保护图片 |
| [post-share-api.yaml](post-share-api.yaml)、[repost-api.yaml](repost-api.yaml) | 原帖分享与私人转帖 |
| [comment-api.yaml](comment-api.yaml)、[comment-business-component.yaml](comment-business-component.yaml) | 评论基础与完整业务；[comment-text-component.yaml](comment-text-component.yaml) 保留早期文字阶段范围 |
| [direct-message-text.yaml](direct-message-text.yaml)、[direct-message-chat.yaml](direct-message-chat.yaml)、[direct-message-governance.yaml](direct-message-governance.yaml) | 私信文字、聊天与治理 |
| [report-api.yaml](report-api.yaml)、[moderation-api.yaml](moderation-api.yaml) | 举报、处理、处罚与申诉 |
| [system-notification-api.yaml](system-notification-api.yaml)、[notification-inbox-component.yaml](notification-inbox-component.yaml) | 系统消息与通知收件箱 |

认证架构和固定签名字节见[逻辑契约](../../docs/design/auth-privacy-data-api-contract.md)与[注册协议](../../docs/design/auth-privacy-registration-protocol.md)；表约束、事务、留存和升级顺序见[数据库迁移设计](../../docs/design/auth-privacy-database-migration-design.md)及正式迁移。早期设计中的限制按[当前设计索引](../../docs/design/README.md)列出的明确变更理解，不通过本次文档整理修改协议文件。

目录 `200` body 仍为 `{channels:[…]}`、七记录与五字段。增加必需 `Session-Expires-At`、`X-Request-ID`、`Cache-Control: no-store`，错误统一为 `error:{code,message,details?}`＋顶层 `requestId`；公共边界的 400／403／429 与会话 401／服务或目录 503 均列入契约。只有最终授权事务提交成功后才能发送截止和目录；客户端持久写截止后发布节点。缺失或非法截止不能当作目录成功。

## 安全约束的表达

V/C 使用真实独立 HTTPS origin。`servers: /` 只是相对部署位置，不授权在同一网关／运维主体下共用身份、密钥或访问权限。固定二进制票据用规范无填充 base64url；解码重编码、实际字节长度、重复 JSON 键、跨字段关系、锁内截止和幂等状态等不能只靠生成器检查。

内部路由另设入口，`x-mtls-required: true` 与 `x-allowed-peer` 强制实际客户端证书和预期对端验证；OpenAPI 3.0.3 没有原生 mutualTLS 方案，`security: []` 只表示没有普通 HTTP 凭据，**不是公共匿名接口**。扩展不会自动生成 mTLS 部署；固定消息签名仍须验证。不得使用可伪造的证书请求头代替传输认证。[OpenAPI 3.0.3 规范](https://spec.openapis.org/oas/v3.0.3.html)

所有认证响应含 no-store 与各方独立请求编号；未知结果先核对，原码／令牌不重放。完整幂等结果到期原位收缩为无身份的 EXPIRED 键摘要锚点；注销申请 ID 同样防重执行。长期锚点和槽位终态有存储成本，生产前须完成独立审阅、容量与清理验证。

## 校验与生成

锁定 Python 校验依赖见 `requirements-check.txt`；脚本拒绝重复 YAML 键和缺失本地引用，并核对分类目录与身份契约。默认选择 `*-api.yaml`；可传入完整 OpenAPI 文件路径。`*-component.yaml` 等组件和私信分阶段文件不应仅因默认命令通过就视为已验证，按对应产品 CI 的契约及实现测试核对。用已有工具环境执行：

```sh
python packages/openapi/check-specs.py
# 只有 PyYAML 6.0.1 时可先检查结构；此项不能代替 OpenAPI 完整校验
python packages/openapi/check-specs.py --yaml-only
```

目录生成固定使用 `oapi-codegen v2.4.1`，由已有安装或 `OAPI_CODEGEN` 指定可执行文件；脚本不下载工具。只生成目录类型与 chi-server 接口到被忽略的 `packages/openapi/generated/channel-api.gen.go`。本地生成后保存该文件，后续 `--check` 重生成到私有临时目录并逐字比较，可发现本地契约与生成物漂移；此文件不提交，CI 每次从契约重新生成并做 HTTP 契约回归。

```sh
sh packages/openapi/generate-channel.sh
sh packages/openapi/generate-channel.sh --check
```

原始日期、规范能力字节、最终授权事务和 Dart 持久 fence 由实现测试验证。生成／静态校验不等于实现测试或独立安全审计；工具缺失时须明确列为未执行，不使用未锁定 latest。
