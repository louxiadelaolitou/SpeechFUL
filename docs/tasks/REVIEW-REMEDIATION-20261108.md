# REVIEW-REMEDIATION-20261108 — 十项审查最终清单修复

- 模式：常规；负责人／状态维护人：当前 Session。
- 分支：`codex/fix/review-remediation-20261108`；Worktree：`E:/Hnuhole-env/source/HnuHole-review-remediation`；PR：[SpeechFUL #123](https://github.com/zewbby/SpeechFUL/pull/123)。
- 基线：`dbc97798d3e0f9521b1032980905929bcb6eb351`；相较审查基线 `4107194` 仅 README 变化。实现范围为承载本记录的提交相对该基线的差异。
- 独立补丁审查基线：`d8b7d8334e7f41607170d20caf9a7ef83d8d8570`；本轮增量为承载本记录的提交相对该 SHA 的差异。
- 授权：用户要求“按最终修改清单的优先级开始修复，全部修复”；包含 R01–R22 的本地实现、验证和必要本地提交；未授权推送、合并或部署。
- 本轮授权：用户要求“对 R04、R05、R06、R08 的实际修复补丁做独立审查，发现问题就修复并复验，完成后更新报告”。两名只读 AI 审查者分别核对事务与授权恢复；实现、测试及状态维护仍由当前 Session 执行。
- 收尾授权：用户随后要求“做完整个收尾”，据此推进本任务分支推送、PR 和 CI 跟进；合并与正式部署仍按仓库要求另获明确授权。远端仓库已更名为 `zewbby/SpeechFUL`，2026-11-08 核对 main 仍为 `dbc97798`。
- 需求确认：用户已确认旧搜索游标失效并提示刷新；编辑后移除不匹配结果并保留其余列表位置；个人回复自动分页、失败可重试；私信本机历史缺失采用明确提示且不自动清未读。无未决取舍。
- 更新日期：2026-11-08。
- 状态：实现已完成并推送；R04／R05／R06／R08 独立实际补丁审查及问题修复复核 PASS；本地全套移动端测试、静态分析、ARM64 构建及定向真实 SQL 复验 PASS（移动端 4 项环境条件 SKIP）；远端产品 CI 绑定当前 PR head，见下述实时检查链接；真机与发布验收 NOT_RUN；未合并；未发布。

## 目标与范围

执行[已核验报告](https://chatgpt.com/space/page_748333c9091c8191970227d701523f5f)的 22 个修复包，覆盖原报告 34 个编号项及新增注销／私信锁环路。优先处理材料保护和受限入口，再处理授权、事务与恢复问题，最后完成投影、分页、资源和状态表达。原报告暂缓项不扩大为本次范围。

## 接口与依赖

- 保留最终 C Safety Gate、会话代次、资源归属、未知结果原键与权威回执检查。图片校验移到事务外后，最终提交重新核查全部条件。
- 新增 `0038_search_content_revision.sql`：已发布内容编辑递增搜索版本，旧游标返回 `CURSOR_INVALID`。搜索可选 `postId` 用于按服务端相同谓词核对编辑后成员关系；摘要绑定该条件。
- C runtime 最低迁移版本为 38，并检查版本表、启用的触发器及读写权限。部署需先应用迁移和 `infra/postgres/grant-community.sql`；本次仅在一次性测试库执行。
- 单例搜索版本采取保守失效：其他已发布帖子的内容编辑也可能使旧搜索游标失效。删除仍沿用既有分页语义。

## 验收标准与最终修改清单

以下每项均已实现；PASS 的范围以证据列为准，不等于生产或真机验收。

| 修复包 | 已实现结果 | 本地验证 |
| --- | --- | --- |
| R01 P1 私信草稿 | 重试实际保存当前输入，成功才清错误；保存代次防止迟到失败覆盖新成功 | PASS：草稿 SQLite／widget 失败恢复回归 |
| R02 P1 评论材料 | 图片读取失败保留原日志、图片引用和拒绝状态 | PASS：新增读取失败材料保留场景 |
| R03 P1 受限入口 | 欢迎页和登录页可进入独立通知／申诉流程 | PASS：入口 widget 与既有治理回归；未宣称设备全链路 |
| R04 P2 注销锁序 | 身份行锁移到最终 Gate 后，匹配文字／图片私信锁序 | PASS：SQL 两种私信交错、冻结／旧代次拒绝；补强注销、身份墓碑和 RELEASED 回执终态 |
| R05 P2 图片 Inspect | 外部校验在预检事务回滚后执行，最终事务重验权限、身份、限制、配额和回执 | PASS：SQL 拉黑、冻结／换代、真实撤销、同键封口、竞争发送与重放；检查消息及回执持久终态 |
| R06 P2 授权窗口 | 请求序号与授权代次分离；元数据持久化同样按授权代次校验，同令牌续期保留页面和输入 | PASS：设备写入交错、实际评论 runtime 会话／内容／草稿保留、真实授权暂停拒绝及既有回归 |
| R07 P2 OTP 过期键 | 仅权威过期且尚未进入确认／证明／提交时释放发送键，未知保留原键 | PASS：登录和重置各自超时→原键过期→新键发送 |
| R08 P2 重置拒绝 | 策略拒绝持久化 NOT_COMMITTED；新输入用新键；已失效意图确认未提交后转重验，未知保留；重启纠正恢复用户名 | PASS：SQL 终态／迟到提交；客户端跨重启、INVALID／EXPIRED 与未知分支、终态页重新开始点击路径 |
| R09 P2 评论终态 | 限流重试和封口均消费终态，先持久化拒绝再清等待 | PASS：新增两条终态回归 |
| R10 P2 编辑配额 | 暂存配额拒绝后仍以原键、完整图文调用最终保存取得终态 | PASS：新增 widget 核对原键／完整图片 ID、保留输入；既有编辑回归 |
| R11 P2 治理结果 | 放弃时发现已提交回执显示真实结果；评论处罚准确指向评论 | PASS：新增已提交／未提交分支；治理与后台回归 |
| R12 P2 OpenAPI | 去掉当前模式不再输出或要求的三处恢复码 required，保留兼容字段 | PASS：13 份完整规范校验及现有认证 HTTP 回归 |
| R13 P2 编辑同步 | 已确认内容覆盖个人页、喜欢、收藏及搜索旧投影；迟到旧页失效；服务端核对后移除不匹配结果 | PASS：编辑后搜索成员关系 SQL、迟到续页与移除 widget；其余入口既有回归 |
| R14 P2 删除投影 | 评论合并过滤墓碑，私信每个候选 ID 重读最新持久投影 | PASS：评论／私信全套回归；合并路径代码核对 |
| R15 P2 后台分页 | 同一列表续页串行化，防止同页并发追加；失败后释放在途状态 | PASS：浏览器双击续页仅一次请求且列表无重复 |
| R16 P2 搜索续页 | 内容版本变化拒绝旧游标，保留当前列表并提供显式刷新；重叠结果也可恢复 | PASS：真实 SQL 编辑失效、启动权限检查；客户端显式刷新 |
| R17 P2 个人赞藏 | 个人卡片显示未知／失败状态及沿既有 CAS 重试的核对按钮 | PASS：个人页、赞藏核心既有回归；按钮与既有恢复路径代码核对 |
| R18 P2 超时 | 后台及频道读取覆盖响应体取消；限流元数据读／恢复和总调用受预算约束，固定指标记录耗时 | PASS：Node 响应体停滞；真实 SQL Recover 锁等待取消及后续恢复；三类元数据预算测试 |
| R19 P2 运维 | 保留连接、schema、权限、监听等安全分类；限流配置路径触发产品 CI | PASS：诊断分类与敏感信息测试、启动 SQL 回归；CI 路径静态核对 |
| R20 P2 图片资源 | 草稿按显示尺寸与 DPR 解码并释放缓存；已确认上传去重，未知可重试且换授权窗口清缓存 | PASS：上传确认／失败恢复新增测试及图片回归；九图真机峰值 NOT_RUN |
| R21 P2 响应性 | 私信卡片点击等待在途刷新；搜索防抖、分批扫描及取消；个人回复逐页显示、自动续页和失败重试 | PASS：点击场景；个人回复首批可见／续页失败保留／原游标重试新增 widget；搜索回归 |
| R22 P2 状态表达 | 本机历史不可用明确提示且不清未读；限流与身份控件翻译，遵守减少动画设置 | PASS：私信、翻译、语义与减少动画回归 |

## 独立补丁审查及修复

审查对象为 `dbc97798 → d8b7d83` 的实际实现及完整关联调用链，随后复查本轮修复差异。只读审查者：`review_transaction_patch`（R04／R05）、`review_authority_reset_patch`（R06／R08）。二次复核均未留下确定性未解决问题；这是独立 AI 审查证据，不是 GitHub 成员批准，也不替代此前 XHIGH 方案核验或真实环境验收。

| 结论 | 问题及实际修复 | 验证边界 |
| --- | --- | --- |
| R04 无新增代码缺陷 | Gate 后取得身份锁的顺序成立；补强测试，要求注销实际完成、对方发送 FAILED／CONTACT_UNAVAILABLE、身份墓碑及释放回执一起提交 | 真实 PostgreSQL 文字与图片两条交错 |
| R05 无新增代码缺陷 | 预检已回滚后才 Inspect，最终 apply 重跑业务校验；新增 Inspect 等待期间的权限／命令竞争测试 | 冻结、恢复换代、真实撤销均无消息或命令副作用；封口／成功赢家回执保持唯一；成功重放不再次 Inspect。未新增额度竞争、身份删除的暂停校验交错测试 |
| R06 P2 已修复 | `_persistDirectoryExpiry` 仍用 `_operation` 围栏；正常续期与 vault 写入重叠会误拒绝有效响应，设备页停留 loading，评论 runtime 还可能按 SESSION_CHANGED 清有效令牌。改为 `_authorityVersion`，保留 token、调用方、销毁及授权失效校验 | 缺陷先由 `independent-r06-before.log` 复现；正向验证会话／评论／草稿保留，反向验证真实授权暂停仍拒绝 |
| R08 P2 已修复 | 服务端废弃意图也可返回 NOT_COMMITTED，旧本地有效期不能证明意图仍可复用。INVALID／EXPIRED 先查原键，取得权威终态后同一次持久写入清匹配旧意图；结果未知保留。终态页提供重新开始恢复，明确确认后清旧邮箱意图 | API／vault 替身配合实际流程和页面，覆盖重启、纠正、重新验证及未知不可清；原页面已有重验入口，因此不称“永久死锁” |
| R08 P3 已修复 | 跨重启进入纠正密码页时同步恢复 `_username` | 断言纠正后用户名与原意图一致 |

本轮最终需要修改的清单：上述 2 项 P2、1 项 P3 及 R04／R05 测试断言均已落实；无已知待改项。服务端产品代码本轮未再变更。

## PR 收尾复验

首轮 PR head `ea6bb1d` 的 CI 暴露了 0038 迁移的配套遗漏：隐私角色审计尚未允许 runtime 对搜索版本表的 SELECT／UPDATE；schema 清单遗漏该表；私信、信息流和头像迁移测试仍固定预期 37。已补齐精确允许项、表清单和版本 38 断言，并新增额外表权限／其他角色权限必须被拒绝的真实 SQL 测试。没有放宽其他表或角色的权限。

本地 `closeout-schema-audit-final.log`：13 个 schema／头像受限生命周期顶层测试 PASS；3 个专用 Goose 迁移测试因缺对应升级库夹具 SKIP，交由当前 PR head 的专用产品 runner 验证。发布工具与角色审计测试使用该日志末尾的结果；其一次性双库由测试自行创建清理。初次本机审计库启动失败源于过长的 Unix socket 临时路径，改用 `/tmp` 下唯一夹具目录后重新执行，未修改生产代码规避限制。

发布工具与角色审计最终 16 项 PASS。第二轮 CI 又验证出 C29 搜索兼容路径误读 0038 表；已让既有不支持编辑的 schema 使用固定内容版本，可编辑 schema 仍强制读取版本表，C runtime 最低迁移版本仍为 38。`closeout-feed-compatibility.log` 使用完整专用 FEED 升级库夹具，11 个顶层测试 PASS、无 SKIP，覆盖七种迁移起点、旧查询及搜索编辑失效；新增可编辑 schema 缺版本表必须失败关闭的负向测试 PASS。日志中其他三个包为无匹配测试，不计入本轮 PASS 数。Go vet／命令构建 PASS，见 `closeout-go-check.log`。迁移／审计和该兼容修复均属于 R16 的配套收尾。

## 验证证据

证据目录：`E:/Hnuhole-env/remediation-validation-20261108`。移动端为 Flutter 3.47.5／Dart 3.13.4；SQL 为 WSL2 Linux 文件系统内任务独占 PG16 双库／Redis，Go 1.22.2。测试数据库由隔离 runner 创建和清理，未连接共享或正式数据。

| 检查与命令 | 结果 | 证据 |
| --- | --- | --- |
| 本轮 `flutter test --no-pub --reporter expanded` | PASS：1225 项，4 SKIP | `independent-mobile-full.log`；绑定 `d8b7d83` 加本记录所属提交的全部代码／测试差异 |
| 本轮认证／评论专项 5 文件；邮箱重置专项 | PASS：83 项；16 项 | `independent-mobile-round4.log`；`independent-reset-ui.log` 含两条终态页重新开始 widget，均随后纳入全套 |
| 本轮原补丁认证／注销／DM SQL `go test -race` | PASS：14 个顶层测试，无 SKIP | `independent-sql-baseline.log`；`d8b7d83` 服务端实现，覆盖注销／DM、冻结／旧代次、密码工作围栏、并发重置、邮箱证明与关闭／封禁保留 |
| 本轮补强后 SQL `go test -mod=readonly -race -count=1 -timeout=10m ./internal/authprivacy -run '^Test(ClosureDMSendGateOrder\|DMImageInspection.*\|ResetPolicyRejectionFencesLateCommit\|ReplacementResetIntentAbandonsPreviousProof)Postgres$' -v`；`go vet -mod=readonly ./internal/authprivacy` | PASS：5 个顶层、10 个最终场景，无 SKIP；vet 退出 0 | `independent-sql-final2.log`；最终源码经 overlay 复制到任务专属 WSL Linux 目录，真实一次性 PG16／Redis |
| `flutter test --no-pub --reporter expanded`（apps/mobile，原实现） | PASS：1212 项，4 SKIP | `mobile-suite-final.log`；绑定 `d8b7d83` 产品代码，本轮增量另行复验 |
| `flutter test --no-pub test/comment_views_test.dart test/governance_test.dart --reporter expanded` | PASS：103 项，含新增回复分页与举报回执场景 | `mobile-final-scenarios2.log` |
| `flutter test --no-pub test/post_edit_screen_test.dart --reporter expanded` | PASS：5 项，含新增配额终态场景 | `mobile-edit-final2.log` |
| 本轮 `flutter analyze --no-pub` | PASS：No issues found | `independent-mobile-analyze.log` |
| 本轮 `flutter build apk --debug --no-pub --target-platform android-arm64` | PASS | `independent-android-build.log`；78.7 秒，SDK XML 版本警告后构建成功，未装机。继续使用已核对包内 SHA256 的 ARM64 SQLite 缓存 |
| `node --test apps/admin/test/core.test.mjs` | PASS：14 项 | `admin-tests.log` |
| `node apps/admin/test/browser-smoke.mjs` | PASS：合成 HTTPS + Edge；7 次业务命令，0 页面错误、0 外部请求，含双击续页 | `admin-browser-final.log`；真实浏览器、替身 API，不代表生产服务端 |
| `python3 packages/openapi/check-specs.py` | PASS：13 份规范 | 本机既有固定 Python 校验库；原审查的三处 required 冲突已删除 |
| `go test -mod=readonly -race -count=1 -timeout=10m -p 1 ./internal/authprivacyruntime ./internal/ratelimit ./internal/posts -v` | PASS | `go-final4.log`；真实 SQL／Redis，含缺权限、停用触发器、元数据超时 |
| authprivacy 五项定向 SQL `go test -race` | PASS | 同上：Inspect 期间拉黑、pre-0015 兼容、密码策略终态、Recover 预算、编辑搜索游标；两组共 65 个顶层测试，无 SKIP |
| 注销与私信、冻结换代、邮箱重置／HTTP、旧搜索 SQL 定向回归 | PASS | `sql-round2.log`；对应实现此后未改变 |
| `go vet -mod=readonly ./internal/authprivacy ./internal/authprivacyhttp ./internal/authprivacyruntime ./internal/posts ./internal/ratelimit`；`go build -mod=readonly ./cmd/...` | PASS | `go-final4.log` 同一隔离 runner，退出码 0 |
| `git diff --check`；`python tools/check-collaboration.py --staged` | PASS | 已检查最终暂存差异；协作检查不冒充产品 CI |

4 项 SKIP：`dm_chat_http_test.dart`、`dm_http_fixture_test.dart`、`dm_governance_http_test.dart` 缺专用 HTTP 夹具；`identity_avatar_photo_test.dart` 缺 `AVATAR_REPRO_INPUT`。没有将它们计入 PASS。前轮测试失败记录保留，最终修正夹具 ID／标题、旧语义断言、测试客户端清理及无障碍标签后，受影响检查均已复验。

本轮中间失败日志也保留：`independent-r06-before.log` 包含修复前设备 loading 反例；其负向测试最初误用了仍有有效会话时无效的 clear 调用，后改成实际 sessionUnavailable。新测试早期还修正了枚举拼写、UNKNOWN 拒绝后应重读持久记录、Gate 初始代次已为 2 的夹具认识。最终 `independent-mobile-full.log` 与 `independent-sql-final2.log` 才是本轮完成证据，未把夹具失败算作新增产品缺陷。

## 阻碍与交付边界

无剩余产品取舍或本次清单内已知待改项。移动端产品代码提交 `32b55dfce265702cc4f189bad9cd7dad6731a215`；此后收尾补齐上述权限审计与迁移验收，以及交付记录。任务分支已推送并创建 PR #123；未合并、未部署。保留分支、Worktree、日志和 APK 供送审及后续设备验收；一次性数据库由各 runner／夹具自行清理。

- 远端适用产品 CI：[当前 PR head 的实时检查](https://github.com/zewbby/SpeechFUL/pull/123/checks)。收尾时创建 PR 触发并跟进，最终结果写入 PR 描述及修改报告；本记录不固定复制会随提交变化的检查快照，合并前须核对当前 head。
- R04、R05、R06、R08 独立实际补丁审查：PASS；发现的问题已修复并二次只读复核。此前 XHIGH 核验针对缺陷与方案，本轮审查针对实际补丁，二者分别记录。
- Android 真机操作、iOS／原生 Passkey、生产高可用与时钟故障、九图内存及历史 P95：NOT_RUN，不由包测试、构建或浏览器替身替代。
- 0038 迁移与权限尚未应用共享或正式环境；进入合并／发布前按仓库规定完成上述适用验收。
