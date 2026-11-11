# AUTH-REGISTRATION-RECOVERY-01 — 注册结果核对与校邮找回闭环

- 模式：常规；负责人／状态维护人：当前 Session。按 grill-with-docs 依赖的事实调查要求委派一次只读入口核查，无并行实现。
- 分支／Worktree／PR：`codex/fix/registration-recovery-20261111`；`E:/Hnuhole-env/source/SpeechFUL-registration-recovery-20261111`；初始基线 `89b592e2761a75f053cf6ec87444c71a52afb070`；已整合 PR #134 的 67cd245，后续整合最新 main `6efa79d39fc70295d7098b8a2b79b14ce73f60e8`；本任务 PR [#135](https://github.com/zewbby/SpeechFUL/pull/135)。
- 授权依据与边界：2026-11-11 用户报告新手机注册后一直“开户结果待核对”、忘记用户名密码且无找回／邮箱登录，要求全面核对并及时提问。允许本地调查、修复、验证；不修改真实用户凭据或数据，不沿用其他任务的推送／合并／发布授权。 用户随后明确要求空间释放后继续剩余验证、打包并提供安装包；本轮据此构建与原版同包同签名的本地 release 覆盖候选，其后用户授权推送／PR／CI／合并和兼容部署，再以最新要求收窄为“只合并”：不再部署后端、不再次打包、不分发或发布新包，等待后续统一上线。
- 需求确认：用户先后提供两段录屏，确认 v1.1.0 新手机创建账号后被限制在用户名密码核对页，忘记密码后没有其他入口。最新已确认流程：注册成功后弹窗原文 **“注册成功，请重新登陆”**，关闭后进入完整普通登录页，支持用户名密码／校邮验证码登录、找回账号、创建账号与返回；退出重进不再强制待核对。注册结果未知采用真实提示并同样回普通登录页；核实未提交后才允许原槽位继续注册。找回密码使用校邮，重置完成回登录页。注册不自动进入首页，不新增激活状态／链接／期限；Passkey／恢复码仍暂停。无未决产品取舍。
- 更新日期：2026-11-11
- 状态：实现 已完成；验证 本地整合范围 PASS、远端 CI 以 [PR #135 当前 head](https://github.com/zewbby/SpeechFUL/pull/135/checks) 为准；合并 由已授权的 [PR #135](https://github.com/zewbby/SpeechFUL/pull/135) 保留实时回执；发布 本轮不执行。

## 目标与范围

注册后改为弹窗提示并进入完整普通登录页，取消待核对状态对路由、登录方式、找回和注册入口的强制限制；退出重进和页面返回保持正常导航。程序处理原注册结果，已提交、未提交和结果未知采用真实反馈，不把未知提示成注册成功；保持 C/V 分离、最终 Safety Gate 和不重复开户边界。

## 接口与依赖

- v1.1.0 正式标签 `1289de6a91f43acfa2a8dceec3c46acf5395d95b` 与基线的注册／登录／邮箱重置业务实现一致；标签额外包含字标、注销输入建议及版本包装。发布信息已通过 GitHub 只读 API 核对。
- 现有注册只在首次成功回会话，相同原键重放不重发秘密；现有本机仅保留 intentId/key，恢复时没有远端核对。
- 校邮登录底层已允许注册未知锚并存，校邮重置被本机检查阻挡；UI 异常分支同时隐藏两者。

## 验收标准

- 原注册结果未知／冷启后，可使用用户名、校邮验证码或校邮重置；入口切换与实际模式一致。
- 原结果核对只返回非秘密状态；待处理、已提交、权威未提交、结果不可用分别处理，未知不清锚、不创建新注册。
- 未提交只在原事务无法迟到提交后确认；重新验证沿用原 bootstrap／slot，已创建账号不重复开户。
- 校邮重置不要求用户名／旧密码，成功后回登录页，不自动登录；错误／过期／迟到结果保留安全边界。
- 相关 Flutter、HTTP、真实一次性 PostgreSQL、安全 Gate、API 契约、Android 构建证据绑定最终差异；真实用户手机与生产状态单列。

## 当前进度与证据

被测版本：基线 `89b592e2761a75f053cf6ec87444c71a52afb070` 加本记录所在任务分支的产品／测试差异。历史 APK（提交 a35dad6，不含 #134）与该时点分支的 402 个产品／锁文件逐文件比对一致，仅保留已发布 v1.1.0 的字标、NOTICE、注销输入建议和字标生成器兼容差异，以及显式发布包装；见证据目录 `final-source-proof.json`、`android-packaging.json`、`android-source-manifest.json`。最终承载提交通过本记录 Git 历史定位；后续仅进度文档更新不使产品证据失效。

| 项目 | 环境／命令 | 结果与证据 |
| --- | --- | --- |
| 录屏与原故障 | 两段用户录屏，本地抽帧；只读校邮元数据匹配 | PASS：原注册意图 16:47 创建、已过期、无活动账号；没有读取密码／OTP或修改生产数据。入口隐藏及强制路由根因已确认；首次提交失败的具体触发原因尚未证明 |
| Flutter | Linux 独立源码，Flutter 3.47.5／Dart 3.13.4，锁文件依赖；最终 `flutter analyze --no-pub`；完整 auth-feedback 客户端测试列表另加 crypto，单并发 | PASS：整合 #134 后分析零问题，290 项通过（包含首轮 287／180 项，不叠加计数）；覆盖成功／未知提示、两种登录、找回、冷启、返回、重置验证后返回与重入、迟到响应和会话边界。`flutter-analyze-final.log`、`flutter-ci-final.log` |
| 页面 | 375px 实际 Flutter 渲染；320px／1.6 倍文字回归；最终候选保留 v1.1.0 字标配置 | PASS：指定成功文案、登录页邮箱／找回／创建入口均可见，最终图片 `ui-final/`；`flutter-visual-final.log`。不冒充设备截图 |
| Go 主机检查 | Linux，Go 1.22.2；authprivacy／authprivacyhttp／runtime 的 test、vet，C API 候选构建 | PASS：`go-host-final.log`、`go-vet-final.log`、`go-candidate-build.log`；候选程序 `community-api-candidate` 已生成，未部署；V 业务代码无变更 |
| 真实 SQL 与 HTTPS | runner 私有 PG16 socket 库，`AUTHLAB_EMAIL_TEST_PATTERN='^Test(SignupResult\|EmailResetPasswordAndPausedCredentials\|EmailLogin)'`，另运行 `TestHTTPSPostgresEligibilityAndSignup` | PASS：7 项 SQL 顶层测试及实际 HTTPS 注册前／后结果查询；包含已提交、未提交期限、迟到提交拒绝、初始及最终 Gate 冻结、校邮登录／重置与封禁。`go-sql.log` |
| 实际 Dart→HTTPS→C/V SQL | `AUTHLAB_REGISTRATION_FLOW_ONLY=1`、`authlab/run-mobile-isolated.sh`；仅客户端原生存储端口为替身，邮件由私有测试 sender 捕获 | PASS：原提交未执行且合法终止后，冷启查询 NOT_COMMITTED、保留 bootstrap／slot、等待真实发码冷却后重验并注册；注册不导入首次会话；账号密码与校邮登录、恢复原未知锚、校邮改密、旧密码拒绝、新密码登录及各阶段重启／退出。207.72s，`flutter-real-https-sql.log`。真实 C/V Gate、HTTPS、签名与 SQL 均运行；不代表外部 SMTP 投递或真机 Keystore／Keychain通过 |
| API 与 CI 配置 | `PYTHONUTF8=1 python -B packages/openapi/check-specs.py`；workflow YAML／步骤核对 | PASS：13 份完整规范；新增原注册结果读取接口，无迁移。Auth CI 已纳入新 UI、真实 SQL、实际客户端专项；对应客户端命令整合 #134 后本地 290 项 PASS；远端 CI 已启动，结果绑定 [PR #135 当前 head](https://github.com/zewbby/SpeechFUL/pull/135/checks) |
| Android（历史产物，本轮不分发） | Windows，整合 #134 前的 release／ARM64，`lib/main.dart`，版本 1.1.1／2026111103；正式 C/V HTTPS 地址 | PASS：`release-build.log`、`release-signature.log`、`release-badging.log`、`release-receipt.json`。37,308,390 字节，SHA256 `331f9feb1b0c00a6c1c5236c2e138daa0ad007ed8992f9ba937e42db20142be5`。与 GitHub v1.1.0 实物（摘要 b09a0f...）相同 publictrial 包名及证书 `1538fb30...b3dc9ae`，minSdk29／targetSdk36，覆盖版本号递增。首次缺 release config-only 导致开发插件残留的 FAIL 保留在 `release-build-first.log`，按已有发布脚本修正生成步骤后复建 PASS；旧磁盘／内存故障日志仍保留 |

证据目录：`E:/Hnuhole-env/registration-recovery-audit-20261111/`。测试数据库均为 runner 新建的私有一次性库，结束后按 runner 正常停止并回收。

实现要点：取消注册未知状态对普通登录路由的覆盖；成功返回的兼容首次会话不落为本机登录态，先持久排入定向撤销队列再报告成功，迟到响应不能替换后来的账号；服务端新接口只读原结果并保留最终 C Gate，未提交判定不能由 404／410／超时推断。校邮验证后的未提交重置意图可保留，但不会强行占住登录页。

历史集成夹具适配：注册后的 F4、媒体联合和认证夹具改为明确主动登录；原合成密码包含空格、不符合现有策略，已修正测试数据，没有放宽产品策略。旧完整客户端夹具还在暂停范围的恢复码轮换处出现空值失败，本轮不继续推进或宣称该段通过；本次实际客户端专项以单独范围及测试名称运行，默认完整夹具未被跳过或删除。

## 阻碍与下一步

用户要求清理旧构建、将构建写入 E 盘并查明最近大额占用。已核对后删除 C 盘 11 个旧 Flutter 临时目录（1.31 GiB）、E 盘 17 个 11 月 11 日前的旧 `apps/mobile/build`（10.87 GiB），保留源码、签名、SDK及今天两个任务的构建。已持久设置 TEMP/TMP、Flutter/Gradle/Go 缓存及 Android 用户目录到 E 盘，`env-windows.ps1` 同步配置 Java 临时目录；新进程生效，已运行 IDE 需要重启应用。磁盘审计仅查看元数据；观察到 C 盘分页文件由 4705 MiB 增至 6667 MiB，同期又占用约 1.92 GiB，清理后 C 盘仍仅约 0.50 GiB 可用，E 盘约 112.70 GiB。系统分页配置未改动，不继续高内存构建。审计与删除清单保存在证据目录的 `disk-audit-summary.json`、`disk-growth-*.json`、`old-*-cleanup.json`；新建／修改文件的当前大小不等于净增长。

历史 release 候选 a35dad6 已完成，但未包含稍后合入的 PR #134。该 APK 仅保留旧时点验证证据，本轮不作为安装包分发。用户最新要求只合并代码，后续统一打包部署；本轮未再次打包、未安装／分发或操作用户手机。

正式服务只读探针：使用正常 Dart 客户端 UA 时，既有 password-reset-result 返回 401，新增 registration-result 返回 RESOURCE_NOT_FOUND／404；此前 Python 默认 UA 的 403 不代表业务接口判定。新接口尚未正式部署，当前旧未知锚的重新注册仍依赖兼容服务端上线。C 候选已构建，无迁移、V 业务改动或正式数据修改；拟在当前服务配置和 Gate 保持的前提下替换 C 程序，保留原二进制可回滚，核对进程摘要、路由及原有服务可用性。

推送／PR、产品 CI 通过后合并已获明确授权；用户随后撤回本轮部署和新安装包交付范围。正式服务和公开下载保持现有版本，后续统一安排。前段兼容上线计划只保留说明，本轮不执行。新手机真实操作、原生存储及远端 CI 均未宣称通过。

最终专项补测说明：私有夹具先尝试缩短意图截止，被数据库不可变约束正确拒绝（证据 `not-committed-fixture-initial.log`）；最终使用受 C Gate 保护的合法 ABANDONED 终态及现有材料清除规则模拟权威未提交，未关闭触发器或约束。真正过期截止与迟到提交拒绝仍由上表真实 SQL 专项独立证明。

整合 #134：PR #134 于 2026-11-11 21:29（Asia/Shanghai）合入 67cd245，新增“该邮箱未找到账户信息，请先注册”反馈；与本次导航／校邮恢复修复已在同一分支合并，无冲突。补齐结果接口精确 CORS 请求头，仅该 GET 路径可用；允许／错误方法／错误路径回归后纳入同一 PR。

本轮交付约定：整合 #134 后本地 290 项与 analyze、Go test／vet／编译 PASS；只在 #135 当前 head 的适用产品 CI 全部通过后按授权 Squash 合并。合并 SHA、检查与回执集中在该 PR，正式部署和安装包统一留待后续安排。

CI 环境修正：c860bb0 的认证完整流程、Android 构建／恢复演练、iOS 和其余已完成产品组 PASS；三个 Redis／SQL job 在 Ubuntu Azure 镜像 DNS 解析处失败，失败 job 重跑仍复现。复用仓库 moderation workflow 已有的官方 HTTPS Ubuntu 镜像替换，并对三条依赖安装添加有限重试和超时；依赖版本、签名验证与所有测试保持原有要求。来源：[Ubuntu 官方 Archive 说明](https://documentation.ubuntu.com/project/how-ubuntu-is-made/concepts/package-archive/)。此变更仅作用于 GitHub 临时 runner，不修改本机或正式环境的软件源；合并仍等待新 head 的全部适用 CI。

后续 CI 日志证明 archive.ubuntu.com 同样解析失败，单个 Azure 地址替换不足。新增仅限 GitHub Ubuntu 24.04 临时 runner 的镜像选择脚本：以系统 Ubuntu keyring 验证 noble 与 noble-security InRelease 签名及 suite，选择可用 HTTPS 镜像，再由 APT 照常执行完整签名和有效期校验；四条相关工作流共用。脚本 --check 模式不写软件源；未修改本机或正式环境的软件源、凭据或安全 Gate。运行中安装 678 秒后取消的诊断回执在 ci-install-diagnosis.json，取消不计通过；新 head 仍需完整 CI 验证。

同类入口审计：新增 helper 触发的 Authorization Gate job 同样安装 Redis，之前遗漏接入；已补齐 Gate 和仅手动触发的实际进程验收安装步骤。已解析全部 workflow，确认五个 Redis 安装块全部在同一步调用签名镜像选择；手动验收条件未变，不自动运行历史延期项。最后新增提交仅 CI／记录差异，仍等待相应当前 head 产品检查。

依赖准备入口补齐：最后的移动端 SQLite 安装等待超过 18 分钟，镜像故障影响所有 Ubuntu APT 依赖，不只 Redis。已按 job 审计并在所有 APT 首次调用前接入同一签名镜像选择，覆盖库依赖和 PostgreSQL／Redis；调用使用 GITHUB_WORKSPACE 绝对路径，Android 指定历史 source SHA 缺 helper 时保留既有准备方式。所有工作流 YAML 解析及逐 job 入口审计通过，测试内容和触发条件不减少。最终合并仍绑定新 head 的实际 CI。

最新主线整合：等待 CI 期间 main 合入 9491e0f 的热榜副标题移除和 6efa79d 的浏览／发布分类分离，且复用相同镜像脚本。合并冲突仅为五处 CI 调用的相对／绝对路径，采用 GITHUB_WORKSPACE 绝对路径；脚本内容相同，发布分类测试与全部主线业务代码保留。旧 head 63aaff5 因冲突未启动 CI，不计通过；最终以整合提交的当前 head 为准。本轮仍不部署或分发安装包。

最新主线组合验证 PASS：analyze 零问题；认证 290 项 PASS；发布分类／active identity／搜索／Feed 六文件额外 108 项 PASS（latest-main-mobile.log）。Git 比对确认 main.dart、feed_screen.dart、post_search_screen.dart 与 6efa79d 完全一致，五处路径冲突外无产品冲突。
