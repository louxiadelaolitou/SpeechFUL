# FEED-VECTOR-CARDS-01 — 浏览页卡片化

- 模式：常规；负责人：当前 Session
- 分支／Worktree：`codex/feat/feed-vector-cards`，`E:/Hnuhole-env/source/SpeechFUL-feed-vector-cards`；基线 `158beea0a2c1c3abbbb9ebb07673bd21d9870162`（origin/main）。
- 授权依据：用户要求将 SpeechFUL 浏览页每条帖子改为卡片、删掉日期、做一版矢量感，并告知 vivo 已连接；后续明确授权推送和合并，不涉及正式发布。
- 需求确认：浏览页采用圆角卡片与留白，隐藏帖子时间；视觉作为首版供用户查看。用户确认滚动时进入视野的帖子采用气泡入场，离开视野后再次进入可重复播放；浏览卡片三点改横向。不感兴趣按账号长期隐藏，并覆盖推荐、最新、分类、搜索、收藏及点赞列表；隐藏后在原位置保留一小行“撤销”。收藏、点赞关系不删除；别人发来的直达链接仍可打开。用户随后确认移除浏览量，底部从左至右依次为点赞、评论、收藏；分享和实际应用已有的转帖移入三点菜单，连同不感兴趣和举报。
- 更新日期：2026-11-08
- 状态：实现完成；本地验证通过，产品 CI 待运行，真机受阻；合并未合并；发布未发布。

- 本轮新增授权依据：用户于本聊天提出滚动入场、横向三点和菜单；确认账号长期隐藏、搜索和收藏也隐藏，并提供撤销。用户明确授权实现后推送和合并，不含正式部署。

## 验收标准

- 推荐、最新和分类浏览的帖子各自呈现为分隔的卡片，列表不显示日期；明暗主题均可阅读。
- 滚动时进入视野的帖子以气泡入场，滑出后再进入可重复播放；减少动画设置生效。
- 浏览卡片底部按点赞、评论、收藏排列，不显示浏览量；横向三点菜单提供分享、转帖、不感兴趣和举报，并沿用各操作的权限与交互。
- 不感兴趣保存后账号各设备长期隐藏，原位置仅保留撤销行，撤销可恢复；搜索、收藏及点赞列表同样隐藏，直达详情仍可打开。
- 帖子详情、互动、刷新与分页保持可用，刷新保留已加载帖子的组件状态；个人页保留现有显示。

## 当前进度与证据

| 项目 | 被测版本／环境 | 结果与证据 |
| --- | --- | --- |
| 实现 | `158beea` + 本任务工作树差异 | 浏览列表卡片、时间隐藏与平面样式；帖子数据与互动保留。 |
| 静态分析 | `158beea` + 本任务移动端差异，Windows Flutter，`flutter analyze --no-pub` | PASS。 |
| 菜单与交互回归 | 同基线及本任务移动端差异，Windows Flutter；8 个浏览、个人页、分享、转帖相关测试文件 | PASS，115 项；覆盖菜单、三个按钮顺序、隐藏撤销、账号切换、动效及大字体。 |
| 受影响模块回归 | 同基线及本任务移动端差异，Windows Flutter；21 个动效、搜索、媒体、导航和无障碍相关测试文件 | PASS，261 项。 |
| API 本地检查 | 同基线及本任务服务端差异，WSL；`go test -mod=readonly ./...`、`go build -mod=readonly ./...`、`go vet -mod=readonly ./...` | PASS；未配置数据库的测试会跳过真实 SQL，另行记录真实 SQL 结果。 |
| OpenAPI | 同基线及本任务契约差异；锁定的 Python 验证环境，`python -X utf8 packages/openapi/check-specs.py` | PASS，完整规范与仓库契约检查。 |
| 数据库、迁移及 HTTPS | 同基线及本任务服务端差异；WSL 隔离 PostgreSQL 16、Redis 7，`feed-card-validation-final-sql.sh` 及表清单单项复验 | PASS：新装和升级迁移、权限／账号注销、隐藏及撤销、排行过滤、真实 HTTPS、数据库表清单；排行榜 Dart 探针未设置，单项 SKIP。 |
| 跨模块迁移回归 | 同基线及本任务服务端差异；WSL 一次性 PostgreSQL，`TestIdentityAvatarGooseFreshAndUpgradePostgres` 与 `TestDMGooseFreshAndUpgradePostgres` | PASS：新装与升级；初次头像 CI 因旧测试仍期待 0035 失败，已更新到 0036，私信同类断言一并更新。 |
| Dart 真实接口链路 | 同基线及本任务差异；Linux Dart、隔离 HTTPS／PostgreSQL，`feed-card-validation-dart-sql.sh` | PASS：真实客户端隐藏、撤销、账号隔离、直达详情和旧请求防覆盖。 |
| Android 构建 | 同基线及本任务移动端差异；Windows Flutter，ARM64 debug `flutter build apk --debug --no-pub --target-platform android-arm64` | PASS；APK SHA-256 `28a733c5d6f44658b069c7817d37bd77e28659310d094fc754c8926f808917b9`；仅构建，不代表设备或生产验收。 |
| 回归（上一版） | 同基线及上一轮卡片差异；`flutter test --no-pub test/feed_ui_test.dart test/feed_interactions_test.dart test/personal_profile_test.dart` | PASS，57 项；覆盖明暗主题、帖子入口、互动、刷新、分页及个人页。 |
| Android 构建（上一版） | 上一轮卡片差异，Windows Flutter，ARM64 profile，沿用既有 USB 开发配置 | PASS；APK SHA-256 `a0c047024e74dc898e4269c38b879768eecc8701c11fa13ed7c86cab4128d412`；证书与既有日常包一致。 |
| vivo 真机 | V2323A 已连接 | BLOCKED；开发后端恢复拒绝，报告 `database-process-drift`；未安装或清理数据。 |
| 可见预览 | Flutter 交互式示例 | PASS；卡片动效、头部与菜单使用产品共用控件，示例内容及操作不连接真实账号，真实业务设备验证仍未完成。 |
| 产品 CI／合并／发布 | [PR #121](https://github.com/zewbby/SpeechFUL/pull/121) | 当前 head 的 CI 与合并状态以 PR 实时结果为准；发布未授权、未发布。 |

## 阻碍与下一步

开发后端进程与固定清单不一致，需安全核对后再恢复；上线前须按部署流程应用 0036 迁移并更新运行时授权。等待产品 CI，合并不等于部署。

## 后续布局调整

- 用户要求将点赞、评论、收藏缩小并等距集中在卡片右下角，继续沿用已确认的顺序和操作；分类箭头收起朝右，选择器打开时朝下，关闭后恢复。
- 分支 `codex/fix/feed-action-spacing`，基线 `931b5f8d970600b871648652dedc8aaee2b1e1f5`（origin/main）；实现完成，产品 CI 和合并状态以关联 PR 为准，发布未授权。
- 验证：基线加本次布局差异，Windows Flutter 的 `flutter analyze --no-pub`、`flutter test --no-pub test/feed_interactions_test.dart test/feed_ui_test.dart`（59 项）和 `flutter build bundle --debug --no-pub` 均 PASS；预览以示例内容展示。
