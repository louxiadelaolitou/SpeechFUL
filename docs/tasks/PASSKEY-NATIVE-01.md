# PASSKEY-NATIVE-01 — 国内系统凭据与可选恢复

- 模式：常规；负责人／状态维护人：当前 Session；无委派。
- 分支／Worktree／PR：`codex/feat/passkey-native-domestic`；`E:/Hnuhole-env/source/SpeechFUL-passkey-native-domestic`；[PR #118](https://github.com/zewbby/SpeechFUL/pull/118)。基线 `e13fe171659eb5d1ed36ef468f0d2e032e4fcd54`。
- 授权依据与边界：2026-11-08 用户明确确认“Passkey 优先用手机自带的国内凭据管理能力，不依赖 Google，也不要求额外装 App；不支持的手机继续用密码、校邮登录和恢复码”。允许实现、本地回归及必要提交；本轮用户进一步明确授权推送本分支、创建 PR、检查并修复 CI，通过后合并 main。正式部署、用户账号操作和 iOS 真机验收不在范围。
- 需求确认：沿上一轮 `grill-with-docs` 收敛并由本轮用户确认。Passkey 仍是可选恢复凭据，不改成直接登录；校邮登录沿现有协议，不变成校邮重置旧号。Android 走系统提供方，不引导安装外部密码管理器或 Google 服务；不支持时可见提示且密码／校邮登录／恢复码流程可用。已有 Passkey 列表和服务端移除功能不因本机不支持而隐藏。无未决产品取舍。
- 更新日期：2026-11-08。
- 状态：实现 已完成；验证 本地适用检查 PASS，真实系统／RP 关联待验；合并 未合并；发布 未发布。

## 目标与范围

移除 Google Play Passkey 适配器及普通 App 的历史 Google 诊断入口；增加系统能力检测、绑定与恢复入口状态、能力丢失后的安全降级及四语言提示。保留全部最终 C Gate、凭据版本、会话／资源归属和恢复码机制。

## 接口与依赖

- Android 14+ 原生 Credential Manager；保留 AndroidX 核心库，仅作系统接口与标准数据封装，不保留 GMS 兼容层。
- 注册与恢复请求要求系统提供方。只在发现可用的预装系统公钥凭据服务、设备已设置锁屏且没有 Google 系统提供方候选时开放；检测不确定时关闭可选入口。用户无须安装额外 App。第三方普通凭据服务不会因已安装而被当作手机原生能力。
- 系统服务声明及提供方查询是本机能力判断，不代表具体国产机型、RP 关联或成功系统仪式已经验收。
- iOS 沿用 AuthenticationServices；增加设备能力查询，不依赖 Google。真实系统／域关联验收单列。
- App 启动及回到前台刷新能力；绑定／恢复在发起 challenge 前重新检测，原生调用前再次检查。提供方报告不支持／配置不可用时关闭可选入口。能力刷新与原生取消沿用操作代次，迟到结果不可提交。
- 历史 Google 诊断 Java 源移至 `tools/android-passkey-diagnostic/legacy/`，只供显式隔离诊断构建；普通 App 不再声明该 Activity 或 Google 编译依赖。

## 验收标准

- 不支持／检测失败时不发起 Passkey challenge 或系统绑定／恢复，用户仍可正常使用已有密码、校邮登录和恢复码。
- 能力变化、取消及页面／账号变更不能提交旧原生结果；禁用绑定不阻断已有凭据查看／移除及恢复码轮换。
- Android 普通 debug／profile／release 不携带 GMS 运行依赖或 Google 诊断 Activity；标准请求仅允许系统提供方。
- 适用 Flutter 分析、组件／协议回归、Android 构建及原生测试覆盖；缺少真实机型或环境时明确 NOT_RUN，不把 mock 当成国产凭据服务验收。

## 当前进度与证据

| 项目 | 被测版本／环境 | 结果与证据 |
| --- | --- | --- |
| 需求与基线 | 上述用户确认；main `e13fe17` | 已核对共享仓库、远端、worktree 与干净分支；本任务隔离，不改其他工作区。 |
| 平台核对 | Android SDK 36、已锁 AndroidX 1.6.0；官方 API／AOSP 实现 | 系统提供方限制与 Android 14 原生接口可用；旧系统不再借助 GMS 回退。 |
| 产品差异与环境 | 上述基线＋承载本记录的本地提交；Flutter 3.47.5／Dart 3.13.4、Windows、JDK 17、Android SDK 36 | 仅任务记录更新不改变被测产品差异；无服务端、API、SQL、安全 Gate 或正式配置改动。 |
| 静态分析 | `flutter analyze --no-pub`，分别在 `apps/mobile`、`packages/auth_passkey` | **PASS**，均无问题；`mobile-final-analyze.log`、`native-final-analyze.log`。 |
| 移动端认证回归 | `apps/mobile`：`flutter test --no-pub test/passkey_recovery_flow_test.dart test/security_management_controller_test.dart test/security_management_screen_test.dart test/email_login_test.dart test/auth_flows_test.dart test/auth_screen_test.dart --reporter expanded` | **PASS**，107 项；覆盖密码／校邮登录、恢复、凭据管理、可选入口变化、移除及恢复码轮换可用性；`auth-tests-fixed.log`。 |
| 原生客户端契约 | `apps/mobile`：`flutter test --no-pub ../../packages/auth_passkey/test/native_passkey_client_test.dart --reporter expanded`，与 CI 的跨包入口相同 | **PASS**，13 项；含检测失败、提供方丢失、检测期间取消及原生请求启动后的迟到结果；`native-client-final.log`。不代表设备仪式。 |
| Android 策略与依赖 | `apps/mobile/android`：`gradlew.bat --no-daemon --max-workers=2 :hnuhole_auth_passkey:testDebugUnitTest :app:dependencies --configuration releaseRuntimeClasspath` | **PASS**，3 项 JVM 策略测试；release 依赖图无 `com.google.android.gms`／`credentials-play-services-auth`；`android-final-policy-runtime.log`。 |
| Android APK | `apps/mobile`：`flutter build apk --debug --no-pub --target-platform android-arm64`；检查生成 APK 全部 DEX | **PASS**；无 Google FIDO 诊断 Activity、GMS 类或 Play Credential Provider 实现；`android-final-build.log`、`verification-summary.json`。APK SHA256 `e69a721190dcfd7b05d9a829bc2b3d2d7278cac488d3136475b39263318ef902`。未装机。 |
| 隔离诊断工具 | `python -m unittest discover -s tools/android-passkey-diagnostic -p test_build_google_fido_diagnostic.py -v`；`python -m unittest discover -s tools/tests -p test_google_fido_diagnostic_runner.py -v` | **PASS**，8＋25 项；`diagnostic-tool-tests.log`、`diagnostic-runner-tests.log`；只验证历史工具保留隔离及保护规则。 |
| CI 配置 | Auth feedback 增补客户端／恢复／安全管理测试及路径；Android prerelease 增补 JVM 策略和无 GMS 依赖检查；YAML 解析 | 配置检查 **PASS**；已推送并在 PR #118 运行远端 CI，当前结果以 PR Checks 为准；通过后按用户授权合并。 |
| 系统设备与关联 | 国产 OEM Android 系统提供方的真实绑定→移除→恢复→取消／会话失效；真实 RP／签名关联 | **NOT_RUN**；缺对应设备及已批准的实际关联配置。本地替身、编译与依赖检查不能替代。 |
| 本地未执行项 | iOS 本机构建／设备、Android profile／release APK 本地构建 | **NOT_RUN**；无 macOS，Android 本地只构建 debug，release 本地仅检查依赖图。PR CI 另含 iOS 模拟器及 Android 候选构建，见实时检查。 |

以上日志与验证摘要保留在本机 `E:/Hnuhole-env/passkey-native-domestic-20261108/`，不提交日志或构建产物。早期检查发现的 Dart 类型转换、测试异步等待和旧诊断源码构建问题均已修正，表中列最终复验结果；缺少 JUnit 离线缓存后通过正常在线解析完成原生测试。

## PR CI 跟进

- `07dfa7a`：认证回归及 iOS build **PASS**；Post images 的全量移动端回归发现 2 个生产根节点测试留下新增能力查询的 3 秒计时器（1179 PASS／2 FAIL／4 SKIP）。不是设备通过证据。
- 修复共享 `production_startup_test_support.dart`：明确模拟无原生凭据提供方，结束后撤销该 channel handler；不改变生产超时、取消或安全策略。
- 本地 `flutter test --no-pub test/b5_candidate_root_test.dart test/telegram_appearance_test.dart --reporter expanded` **PASS**，4 项，日志 `startup-ci-fix.log`。最终 head 全部 CI 与合并状态以 [PR #118](https://github.com/zewbby/SpeechFUL/pull/118) 为准。

## 阻碍与下一步

本地实现和适用检查已完成，由承载本记录的本地提交交付。国内各厂商实际服务声明、系统仪式与真实 RP 关联仍需在具备对应设备／配置后验收；不据此宣称所有国产手机支持 Passkey。Android 14 以下或检测不到符合条件的系统能力时继续使用原有方式。本轮按用户授权推送并创建 PR，适用 CI 通过后合并；正式部署不在范围。当前 PR／CI 状态以 PR 实时页面为准。

## 2026-11-08 域名关联续接

PR #118 已合并 main `310fed6`。用户本轮确认使用 `speechful.dpdns.org` RP 及原公网试用包／签名，Apple Team ID 暂缺。实际配置、同签名新候选、服务端挑战及公网文件证据统一维护于 [PUBLIC-TRIAL-ACCESS-01 本轮记录](PUBLIC-TRIAL-ACCESS-01.md#2026-11-08-最新-main-与-passkey-并行收尾)，不复制并行状态。原生业务实现不变；iOS Team／AASA 和系统仪式仍独立待配／待验。
