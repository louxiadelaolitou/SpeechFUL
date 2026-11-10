# 运行与升级入口

本页只索引运行说明，不复制部署状态、密码、机器路径或历史命令清单。

| 场景 | 说明 |
| --- | --- |
| 新建隔离 C/V 开发环境 | [API 初始化与启动](../../services/api/README.md)、[基础设施](../../infra/README.md) |
| 评论、权限与快照配置 | [评论运行说明](../../services/api/comment-schema/README.md)、[Redis 快照运行与回退](../design/comment-snapshot-redis/runtime-and-rollback.md) |
| 已批准的日常开发部署恢复 | [development_runtime](../../tools/development_runtime/README.md)；使用部署所有者的私有清单，不猜测机器上的最新版本 |
| 预发布流程 | [Android 预发布手册](../release/android-prerelease.md) |
| 公网试用环境 | [PUBLIC-TRIAL-ACCESS-01](../tasks/PUBLIC-TRIAL-ACCESS-01.md) |
| Android 发布与验收回执 | [ANDROID-BETA-LAUNCH-20261109](../tasks/ANDROID-BETA-LAUNCH-20261109.md) |

新环境初始化、原环境恢复和正式升级是不同操作。文档中的历史配置不代表当前机器状态；执行前检查实际配置、版本、归属和任务授权。不得对未知数据库重置 schema、删除卷或绕过认证与 Safety Gate。
