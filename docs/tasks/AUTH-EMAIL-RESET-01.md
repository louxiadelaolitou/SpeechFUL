# AUTH-EMAIL-RESET-01 — 校邮重置密码与暂停 Passkey

- 模式：常规；负责人／状态维护人：当前 Session；无委派。
- 分支／Worktree／PR：`codex/feat/email-password-reset`；`E:/Hnuhole-env/source/SpeechFUL-email-password-reset`；基线 `158beea0a2c1c3abbbb9ebb07673bd21d9870162`；[PR #120](https://github.com/zewbby/SpeechFUL/pull/120)。
- 授权依据：用户明确“都先不用passkey了。可以用邮箱验证来进行账号密码的重置”。允许调查、设计、实现、聚焦验证及必要本地提交；后续用户明确“把这个分支推送并创建 PR，检查 CI，有问题就修复，通过后合并到 main”，已授权对应推送／PR／CI 修复及通过后合并；正式部署仍未授权。沿用不真机验收、不操作真实用户账号的边界。
- 需求确认：停用 Android/iOS 的 Passkey 绑定与恢复；不删除既有系统私钥或服务端凭据。新增校邮 OTP 重置已有账号密码，替代旧“邮箱不可重置密码”的本项限制；保持 C/V 分离，C 不收邮箱或 OTP，V 不收密码／恢复码。保留现有密码／校邮登录。用户 Q1 明确先不用恢复码，保留该功能实现；邮箱重置不要求保存／确认新恢复码。Q2 确认完成后回登录页，不自动登录。
- Q3 已确认“所有用户流程都暂停”：新注册不再保存／确认恢复码，账号安全页隐藏相关入口，找回密码仅走校邮。已有恢复码实现和数据保留但不开放使用；不为兼容旧接口伪造用户已确认恢复码。无未决产品取舍。
- 更新日期：2026-11-08。
- 状态：实现 已完成；验证 PASS（已执行的本地范围；远端 iOS 编译 PASS）；合并 未合并；发布 未发布。

## 目标与范围

用户在找回密码入口使用原注册校邮验证码证明账号控制权并设置新密码。未注册邮箱不自动开户；已正式关闭的旧号不恢复；封禁、注销缓冲期及最终 Safety Gate 保留。邮箱重置不要求恢复码、完成后回登录页；注册及其余恢复码流程全部暂停。

## 已核实事实与接口方向

- 一精确校邮地址对应一个活动槽位；既有邮箱登录经 V 的签名证明绑定 C 安装标识与原操作键，C 按活动槽位找到原账号。旧任务已允许邮箱登录，但明确没有授权邮箱重置或敏感凭据免复验。
- 现有 V OTP 只有 REGISTER／LOGIN 用途，登录证明只能用于创建会话；不能把登录证明直接当成密码重置许可。邮箱重置应有独立用途／绑定与一次性结果核对，复用现有限流，避免换用途绕过预算或重放。
- 现有恢复码重置生成受限 reset intent，最终事务校验凭据版本、账号状态、限制、注销申请与 Gate，不自动登录。可复用安全事务骨架，不能在客户端改密码或绕过 C 最终检查。
- Passkey 停用覆盖客户端入口及服务端新挑战／证明受理；恢复码停用覆盖注册确认、查看／轮换及恢复。客户端停止原生能力查询。现有凭据与用户数据不清理；邮箱重置只更新密码、撤销旧会话，保留暂停凭据的原数据及版本（旧凭据不会因密码重置重新生效）。

## 验收方向

- 正常校邮取码／错误改码／过期／限流／未知结果及原操作恢复；未注册、已关闭账号与用途混用拒绝。
- 重置期间会话接替、凭据更换、账号状态变化、冻结／恢复及迟到响应不能越权或影响其他账号；封禁不能借重置解除。
- Passkey／恢复码暂停状态在 Android/iOS 一致，密码／校邮登录继续可用。注册不展示／保存／确认恢复码；找回密码只走校邮，未决产品取舍为零。
- OpenAPI／协议、Go 与真实一次性 PostgreSQL／C/V Gate 回归、Flutter 分析／交互测试及适用平台构建；真机与正式发布单列。

## 当前进度与证据

被测内容：基线 `158beea0a2c1c3abbbb9ebb07673bd21d9870162` 加本任务分支中的校邮重置、暂停策略、迁移、协议、共享客户端、测试和契约差异；由本任务本地提交承载。未合并到 main，也未加载公网运行环境。

- C/V 新增 EMAIL-RESET/V1 与独立 C 安装／操作键绑定；REGISTER、LOGIN、RESET 不能混用。沿用精确邮箱、共享发送／验证预算和冷却；未注册不分配槽位或自动建号。
- C 新增受限邮箱重置意图，原证明／键重试返回同一活动意图，消费后不能重新开启。最终事务保留 Gate、凭据版本、重置代次、会话代次、注销期限检查，撤销所有旧会话；既有暂停凭据数据不删除。
- 新注册跳过恢复码确认；Android/iOS 共用入口隐藏 Passkey／恢复码流程，停止原生能力查询。邮箱验证成功后只保存受限意图，清除本地 OTP／证明；新密码仅驻留内存。重置结果未知时保存原提交锚点，仅核对原结果，不另发密码提交。
- 新增 Go／HTTPS 边界／Dart 操作测试，补入现有产品 CI 的邮箱与认证回归入口。PR #120 已触发远端产品 CI；最终结果核对该 PR head，不把本地测试当作远端 CI。

| 项目 | 环境／命令 | 结果与证据 |
| --- | --- | --- |
| Go 协议、服务、HTTP、运行入口 | WSL2 Ubuntu24.04 Linux 独立源码；Go1.22，`go test -mod=readonly ./internal/authprivacy/protocol ./internal/authprivacy ./internal/authprivacyhttp ./internal/authprivacyruntime`；`go build -mod=readonly ./...`；`go vet -mod=readonly ./...` | PASS；无 DSN 的主机测试不冒充 SQL 验收 |
| SQL 与旧恢复／会话回归 | 本任务 runner 创建的私有 PostgreSQL16 socket 库，`authlab/run-email-login-isolated.sh`；按 EmailLogin／Password／Session 及 Recovery／Reset 分组执行 | PASS 的旧恢复／Gate 组见 `E:/Hnuhole-env/email-password-reset-20261108/email-sql-final.log`；原会话／邮箱登录用例亦通过，早期新用例的有效期精度问题已修复 |
| 最新邮箱重置与正式迁移 | 同一独立 Linux 源码，`HNULIMIT_FORMAL_GOOSE=1 AUTHLAB_EMAIL_TEST_PATTERN="^TestEmailReset" sh authlab/run-email-login-isolated.sh` | 新注册、独立 OTP、错误码去重、用途／绑定拒绝、原意图重放、密码与会话失效、冻结／恢复／凭据变化、未注册分流 PASS。`email-sql-latest.log` 中注销 fixture 初次因漏撤销测试会话 FAIL；补正 fixture 后 `^TestEmailResetPreservesBanAndClosure` 的缓冲期／到期两场景 PASS，见 `email-closure-final.log`。没有修改真实用户数据 |
| Flutter 静态与认证／导航回归 | Windows Flutter3.47.5，锁定依赖；`flutter analyze --no-pub`；auth_api、email_reset、email_login、auth_flows、auth_screen、security_management_controller／screen、auth_session_controller、startup_entry、login_home_navigation、personal_route_account | PASS，无分析问题；199 项通过，见 `flutter-auth-final.log`。最后注册两步进度／文案调整后 auth_screen＋email_reset 35 项复验 PASS，见 `flutter-ui-final.log` |
| PR 期间启动夹具修复 | 同一独立 PostgreSQL16 owner/runtime 库，`MODERATION_TEST_PATTERN="^(TestModerationLifecycleCommonCStartup|TestFeedCommonStartup|TestFeedRepostsLeastPrivilege)" sh authlab/run-moderation-isolated.sh` | PASS，3 组真实 SQL 启动／权限／生命周期回归及 8 项安全形状子场景；见 `ci-startup-ledger.log`。仅修正测试台账，不放宽产品启动门槛 |
| PR CI 迁移／隐私修复复验 | 同一私有 PostgreSQL16 与锁定 Goose：头像 fresh/upgrade、私信 fresh/upgrade、帖子 7 条升级分支；正式迁移下重置全链＋`TestPostSchemaPartyInventoryAndPrivateColumnsPostgres` | PASS；见 `ci-avatar-migration.log`、`ci-dm-migration.log`、`ci-feed-migration.log`、`ci-reset-privacy-final.log`。摘要列改名后重置／Gate／隐私库存 108 秒回归通过；Go 主机测试、build、vet 复验 PASS（`ci-go-host-final.log`） |
| OpenAPI 完整校验 | 现成 PyYAML6.0.1＋openapi-spec-validator0.9.0，`python -B packages/openapi/check-specs.py` | PASS，全部13份主规范；不是仅 YAML 解析 |
| Android 构建 | Windows SDK36／JDK17，`flutter build apk --debug --no-pub` | PASS；首轮 GitHub SQLite 下载超时，复用本机已有三 ABI 缓存，由 sqlite3 官方 hook 检验锁定哈希后构建；无依赖、缓存策略或产品源码绕过 |
| iOS 编译／交互、真机、线上 SMTP 重置 | 本机无 macOS；远端编译随 PR #120 跟进，未部署或使用真实收件邮箱 | 本地 iOS 编译 NOT_RUN；远端 PR #120 在 `2dcb958` 的 macOS 无签名 iOS 编译 PASS（[run 42441286457](https://github.com/zewbby/SpeechFUL/actions/runs/42441286457)）；设备验收 SKIP（用户边界）；线上发信与重置 NOT_RUN。共享 Dart 场景通过不代表 iOS 原生编译或生产链路通过 |

证据目录：`E:/Hnuhole-env/email-password-reset-20261108`；仅存本机，不提交构建产物、日志或个人配置。WSL runner 自行回收本任务私有临时 PostgreSQL 进程／库，不触及现存公网数据库和服务。

## 阻碍与下一步

已推送并创建 PR #120，按当前用户授权跟进最终 head 的产品 CI（包括 macOS iOS 编译），失败修复后重验，通过后合并。检查发现启动 SQL fixture 实际安装全部正式迁移，却把版本台账硬编码为 26；本轮改为从实际安装的最新迁移文件取版本，保留产品 C36／V8 启动门槛与安全检查。正式部署需先前向迁移 C37／V8，再同时更新服务及客户端；公网当前仍为旧版本，未执行新配置加载。

PR CI 续验：`2dcb958` 的头像迁移测试因完整迁移链断言仍为 C35 失败（[run 42441286488](https://github.com/zewbby/SpeechFUL/actions/runs/42441286488)）；新增 C36 后，头像、私信、帖子完整迁移链断言同步更新到 36，保留精确版本、迁移数量、升级数据保留与禁止破坏性回滚检查。

同轮隐私库存回归识别到 C36 的摘要列名含 email。C 只保存签名证明及 C 操作键摘要，不保存邮箱、邮箱摘要或 OTP；现将列名准确标为 `proof_digest`／`operation_digest`，保留原隐私库存检查，未加白名单或放宽 C/V 边界。

Android prerelease 续验：`a4ade65` 的候选 APK 与签名／无 Google 依赖检查通过，但实际恢复演练的 `probe-lifecycle` 仍要求注册恢复码并调用旧重置接口而失败。runtimeprobe 已支持无恢复码注册及独立校邮重置，遵守 V 的真实发送冷却，核对原重置结果、旧会话失效及主动新密码登录；仅在旧 main 实际返回恢复码时验证其旧契约，未放开当前服务的暂停功能。

PASS：固定提交 `295f0fd52a9fb48b815a4bb70a3984a67b3f7592` 在本任务 Linux 干净检出和私有 5 实例 PostgreSQL16／Mailpit 中运行完整 `tools/prerelease/environment.py init --rollback-sha 158beea0a2c1c3abbbb9ebb07673bd21d9870162`、`rehearse`。真实 HTTPS 注册→校邮重置→撤销旧会话→主动登录→文字/图片/举报读写、普通重启、独立 Gate 不匹配拒绝、冻结拒绝、备份恢复到两个新实例、拒绝旧会话、旧 main 应用回滚及候选重新部署均通过。全部本任务进程和五个 PostgreSQL 实例已停；私有材料保留。脱敏证据 `E:/Hnuhole-env/email-password-reset-20261108/rehearsal-evidence.json`，输出 `rehearsal-run.log`；SMTP 为本地 Mailpit 合成账号，不代表真实校园邮箱送达。随后仅任务记录更新，不改变被测工具或产品源码。
最新集成基线：等待 CI 期间，另一个功能提交 `89621ddffa2b2005d944fa4ad089381979c2a537` 已合入 main，新增 `0036_hidden_posts.sql`。GitHub 的 PR 合并测试树因此同时包含两条 0036，Goose 在迁移收集阶段退出；这也是本轮迁移工具异常的确定原因，不是生产数据库异常。已在任务分支合并该 main，保留已合并迁移，本任务改用 `0037_email_password_reset.sql`、运行入口要求 C37/V8；完整迁移链断言更新至 37。以上 C36 和旧 SHA 证据保留其当时范围，当前集成版本需重新核对 SQL、移动端、恢复演练及最终 PR head CI。
PASS：最新集成提交 `10b5f431ce9d282ce60eaa92f78ffe4c614e63ce`，基准 main `89621ddffa2b2005d944fa4ad089381979c2a537`，C37/V8 的完整五实例实际恢复／回滚演练再次 17 项全部通过，全部本任务进程和实例已停（`integrated-rehearsal-evidence.json`、`integrated-rehearsal.log`）。Windows Flutter analyze 无问题，认证＋最新浏览页＋导航的 199 项集成回归全部通过（`integrated-flutter-analyze.log`、`integrated-flutter-test.log`）。随后仅任务记录更新；最终 PR head 的全部 CI 将以该集成基准重新核对。推送前再次确认远端 main 仍为 89621dd。