# POST-EDIT-01 — 编辑已发布帖子

- 模式：常规；负责人／状态维护人：当前 Session。
- 分支／Worktree：`codex/feat/post-edit`，`SpeechFUL-post-edit-01`。基线 `06bee794257fd25b9e3221b01ffcd181752c7a79` 与 GitHub `zewbby/SpeechFUL` 的 `main` 一致。共享仓库的 `origin` 使用重命名前的 `zewbby/HnuHole` 地址。
- 授权依据与边界：2026-11-04 用户要求本地调查、开发、验证和必要提交。随后用户明确授权推送、创建 PR、检查产品 CI，通过后合并和部署。部署沿用已核实的本机 WSL 日常开发环境。
- 需求确认：已发布帖字段、原身份与通道、互动保留、未保存退出、明确保存失败的当前页重试，按用户任务与 `EDIT-POST-01`、`EDIT-POST-DRAFT-01`。
  - 2026-11-04 用户确认，原账号同机保留已提交的内容与凭据，重进先核对原次结果。已成功则加载当前公开版，已确认失败可恢复这次提交，仍未知仅允许再次核对或手动重试原次提交。
  - 2026-11-04 用户确认，本次编辑标题、正文和图片。标签编辑随标签功能接入，原有标签保留。
  - 2026-11-04 用户确认，同版先保存者生效。后保存的旧版输入留在页面，可复制文字，再主动载入新版继续编辑。按 DELETE-POST-01 与 MOD-TAKEDOWN-01，确认删除或下架则提示状态、关闭编辑并丢弃本次修改。
- 更新日期：2026-11-04。
- 状态：实现已完成；本地验证 PASS；产品 CI 13/14 PASS、应用回滚演练 FAIL；合并未合并；发布未发布。

## 目标与范围

作者从已发布帖详情的「编辑帖子」进入。保存更新同一帖子，详情发布时间旁显示「已编辑」，点按查看最后修改时间。失败发布草稿沿用原发布任务入口。

- 本次编辑标题、正文和有序图片，支持保留、添加、移除和排序。
- 标签编辑随标签功能接入，现有标签保留。

## 接口与依赖

- [帖子 API](../../packages/openapi/post-api.yaml) 提供作者能力、带公开版本的编辑读取、按原命令核对与保存，以及逐图暂存。旧客户端仍使用原能力响应和详情正文格式。
- [迁移 0029](../../services/api/migrations/0029_post_edit.sql) 保存当前编辑版本和命令回执，保留初次发布事实。新图通过现有图片字节校验器，暂存随帖子、编辑命令和原账号绑定，24 小时过期。最后一次事务核对作者、原身份、帖子状态、会话和 C Safety Gate 后，原子切换公开内容及图片。
- 原账号同机的加密 SQLite 保存已提交输入、原命令和新增原图。提交结果由服务端回执判定。普通未提交编辑留在页面内。

## 验收标准

- 本人详情菜单进入编辑，读取当前公开版本。文字和图片一次保存到同一帖子，刷新读回最新内容，详情时间旁可查看最后修改时间。原作者、通道、发布时间排序、互动和已提交举报的证据保留。
- 未保存退出丢弃本次修改。明确失败时旧版继续公开，编辑页可重试。结果未知可在原账号同机重进核对并重试原次提交，重复提交不会产生第二次修改。
- 旧版冲突保留输入。删除、下架、原身份删除、他人会话和无效授权均不能保存。编辑与举报并发时，证据完整取自某一次公开版本。

## 当前进度与证据

- 基线详情从各来源页复用 `PostDetailScreen`，通过本人能力查询展示操作。基线服务端 `canEdit` 固定为 `false`。新帖草稿、失败发布重试和删帖分别走已有任务及命令链路。
- 图片清单与字节读取在最终授权事务校验帖子可见性及当前公开版本。帖子举报持久化当时的文字与图片证据，见 `REPORT-EVIDENCE-01`。

被测版本为主线 `06bee794257fd25b9e3221b01ffcd181752c7a79` 加本地交付提交中全部产品及测试差异。Android 采用同一暂存源码的独立副本。

| 项目 | 环境与命令 | 结果与证据 |
| --- | --- | --- |
| Go 宿主 | WSL Ubuntu 24.04，Go 1.22.2，在 `services/api` 执行 `go test -mod=readonly ./...`、`go build -mod=readonly ./...`、`go vet -mod=readonly ./...` | PASS。需要真实数据库的项目单独执行。 |
| 真实 HTTPS、SQL 与受限角色 | 非 root WSL Linux 文件系统，Go 1.22.2、PostgreSQL 16.15、Goose 3.22.1、Pillow 12.3.0。以 `HNULIMIT_FORMAL_GOOSE=1`、现有合成图片校验器与 `FEED_TEST_PATTERN` 运行 `sh authlab/run-feed-isolated.sh`，runner 使用 `go test -race -count=1 -p 1` | PASS。运行 `TestRealHTTPSPostEditPostgres`、`TestRealHTTPSPostEditMediaPostgres`、既有发帖删帖、图片撤权、治理、受限运行角色共六项。覆盖同版并发、重复回执、读回、已存在点赞、整组图片切换、原举报证据、删除／下架及授权拒绝。 |
| 迁移升级 | 同一 WSL 工具版本，`FEED_TEST_PATTERN='^TestFeedGooseFreshAndUpgradePostgres$' sh authlab/run-feed-isolated.sh` | PASS。新库与四种历史升级路径，原帖子、通道及互动不被迁移改写。两轮数据库均由 runner 创建私有 Unix socket 集群并在结束时停止清理。 |
| Flutter 与客户端 HTTPS | Windows，Flutter 3.47.5 / Dart 3.13.4，`flutter analyze --no-pub`，`flutter test --no-pub --reporter expanded test/post_edit_screen_test.dart test/post_screens_test.dart test/post_api_test.dart test/post_controller_test.dart test/post_media_repository_test.dart test/post_composer_photos_test.dart test/feed_ui_test.dart` | PASS，187 项，分析 0 问题。Widget 用例自验本人菜单、未提交退出、明确失败重试、保存后读回与时间、同机 SQLite 重开后核对与原图重传、成功回包丢失和权限拒绝。客户端 TLS 契约和服务端真实 HTTPS 分开验证。宿主测试临时使用已有 Windows SQLite 系统库，配置已恢复。 |
| OpenAPI | Windows UTF-8，`python packages/openapi/check-specs.py` | PASS，12 份契约。 |
| Android ARM64 Debug | Windows，Flutter 3.47.5 / JDK 17，`flutter pub get --offline --enforce-lockfile`、`flutter build apk --debug --target-platform android-arm64 --no-pub`、`apksigner verify --verbose` | PASS。C/E 跨盘的 Kotlin 增量构建失败后，以暂存源码的 E 盘同盘副本重建。APK v2 签名验证通过，SHA-256 `ba65aaac5329a0b1b72a9465654ccc3083175c41293b3367f1d9bd551fab2659`。 |
| 协作规范 | Windows，`git diff --cached --check`、`python tools/check-collaboration.py --staged` | PASS。 |

## 阻碍与下一步

- 远端产品 CI、真机和生产验收为 NOT_RUN。本次授权限本地交付。

## PR 与部署续办

- 2026-11-04：复核 c521d9d 并发、权限和断网恢复，真实 PostgreSQL 6 项、Flutter 聚焦 38 项 PASS。
- 已整合最新 origin/main 55663f0（设置与四语言），无冲突。当前产品 CI 待执行，尚未合并和部署。
- PR #100 首轮产品 CI 发现旧契约检查固定 16 个操作；已纳入 4 个编辑操作并补版本必填、禁止改身份/通道及图片上限的负向回归。本地兼容性检查和 10 项回归 PASS，待新 head CI。
- 完整 CI 补充发现主线日期函数签名整合漏项、三处旧 schema 模拟与表清单未同步 0029。已修正；Flutter analyze、编辑页/详情页 32 项测试与 Android Profile 构建 PASS。真实 PostgreSQL 复验中；此前聚焦验证不代表完整 CI 通过。
- 旧能力 DTO 回归仍将 canEdit=true 视为非法，已改为接受真实编辑能力，继续拒绝非法类型和私有字段。本地整套 Flutter test PASS（4 项依赖外部 fixture 的用例 SKIP）；历史 schema/受限权限/运行时回归 PASS。Profile APK 已构建及原日常签名验证，vivo 未连接，未安装其他设备。

### 最终 PR 检查与发布阻碍

- [PR #100](https://github.com/zewbby/SpeechFUL/pull/100)，远端 head `d962cdb1b4651f5bb4fa17e77f02ac2bc97b30d1`；最新基线 `55663f020ff7cacad87c684544c42a3b7f8a341b`。13 个工作流 PASS；[Android prerelease](https://github.com/zewbby/SpeechFUL/actions/runs/41799921917) 的候选构建 PASS、rehearsal FAIL。
- 失败发生在 `tools/prerelease/environment.py` 的 `self.start(prefix='rollback-')` 后严格就绪检查。此前候选启动、真实 HTTPS、数据库备份/新实例恢复、旧会话拒绝均 PASS。不能将此失败解释为测试夹具漏项：旧主线的帖子权限白名单不接受 0029 新增列权限；旧读取代码也不会读取新版编辑内容，直接退旧程序不满足内容一致性。
- 2026-11-04 依仓库需求确认规则重新使用 grill-with-docs，询问分两次兼容发布（建议）或明确采用仅前向修复。结论待用户确认；当前不跳过或削弱回滚 CI，不合并、不执行部署。
- 最终本地整套 Flutter `flutter test --no-pub --reporter expanded test`：1061 PASS、4 SKIP（依赖独立真实 fixture）；分析 0 问题。真实 PostgreSQL `post-edit-pr100-schema-check.sh` 的历史 schema、清单及运行时检查 PASS。开发 ARM64 Profile 包已构建并用原日常证书验签，242 个受检产品文件与最终 PR head 相同。包 `E:/Hnuhole-env/deliveries/SpeechFUL-post-edit-2026110402-profile.apk`，SHA-256 `99a0ac0ce38293058b7797714264deac075f4c4834a0ebf5011a7a7077400c82`；vivo 未连接，未安装其他设备。
- 现有 WSL 日常环境只读预检：C28/V7，C/V Gate OPEN，严格 TLS ready PASS，现行证书有效至 2026-11-11。已在私有目录编译后端候选，尚未停服务、迁移或替换批准清单。候选与部署辅助脚本留在本机，不包含正式生产发布。
