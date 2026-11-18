# PROFILE-SETTINGS-01 — 我的页面与设置入口调整


## 2026-11-04 主线集成候选

- 2026-11-04 续办：用户明确授权续签本机开发证书、恢复原开发服务，以及推送设置功能分支、开 PR 并运行产品 CI。初版候选 `3a94108` 已推送，随后整合最新主线 `663adbc` 的 SpeechFUL 显示名称。冲突处保留主线名称及设置分支的通知正文、刷新无障碍文案本地化。下列 `06bee79` 验证与推送拒绝为上一时点证据。
- 当前工作为常规交付续办。来源 `9a36e8f`，以 GitHub `main@06bee79` 为基线，在隔离副本的 `codex/feat/profile-settings-delivery` 整合。原设置分支和其他工作区均保留。
- 按已确认的 v3 需求合入新版设置、四语言、设备与安全入口及注销交互。整合保留主线头像处理及首页当前身份头像。通知、私信和个人页标题保留主线主题蓝色并随语言更新，通知字号与私信一致。个人页测试保留主线标题颜色检查，并覆盖设置分支的五项分类选中态。
- Windows Flutter 3.47.5：`flutter pub get --offline --enforce-lockfile`、`flutter analyze --no-pub` PASS。首次全量回归 FAIL，主线头像测试仍通过点击首页头像进入个人页。按已确认新版入口，改从底栏进入个人页，保留头像跟随身份及同身份更新的断言。最终 `flutter test --no-pub --concurrency=2 test` 1057 PASS／4 SKIP；跳过项按各自夹具条件保留。此次复验只改上述测试和本记录，应用源码与通过静态分析和构建的 `bed6eca` 相同。
- Android Profile 构建 PASS：首次构建的 Kotlin 增量缓存失败，使用 Gradle `--offline assembleProfile`、进程内 Kotlin、关闭 Kotlin 增量编译复跑成功，沿用原开发配置与 `AUTH_DEV_PROFILE=true`。包名 `org.hnuhole.hnuhole_mobile`、版本 `2026110401`，APK v2 验签 PASS，证书摘要与原设备交付记录一致；APK SHA-256 `8afb0935263e3daef506d5d8c772fd8cacd2fbf69b730bb4e2a0c7f390131cdd6`。
- vivo 业务验收 BLOCKED：已识别指定 vivo，现装 `2026103104`。两个开发端点拒绝连接，恢复器的固定部署静态校验 PASS，但启动在 C Gate 的证书验证处拒绝。只读确认 C/V 内外证书于北京时间 2026-11-02 18:24:50 到期，原 CA 尚在有效期。未更换证书、重新批准部署清单或执行安全恢复；候选未安装，既有装机结果仍限定在原版本。
- GitHub 产品 CI NOT_RUN。自动审批拒绝向功能分支推送，因为该提交包含项目源码和资源，当前授权未明确涵盖向 GitHub 发送这些内容。PR、主线合并和发布均未执行。续办需要明确授权推送和开 PR；设备联调需要先续签开发证书并核验部署清单与严格 TLS 就绪。

## 当前轮次：v3-implementation（2026-10-30）

本节为最新范围；下方 v2 实现与安装记录保留原时点事实，不代表新增要求已经完成。沿用集成分支，实际依赖基线 `ce22f3c`（已装 vivo 的未合并 v2）；规则仍采用 `a4d7228`，实现契约 `99806ba`。入口 Session 维护唯一状态，三执行者已按该契约分工实现并交叉审查。

- 用户新要求：右滑打开窗口；主设置移除私信通知；注销独立标题／流程／输入「确定注销账号」／红色确认按钮／取消和确认弹窗；删除身份文案换垃圾桶图标；语言新增繁体中文和日语并真正切换；设备管理仅设备与会话；恢复凭据和 Passkey 移到账号与安全；意见反馈只保留意见并投递指定邮箱；支持我们接支付宝。
- 已确定：右滑替代 v2 的左滑开启；图标、入口删减、语言选项、设备与安全入口分离的直接要求不重问。
- 用户已确认 Q1 全 App 自有界面四语言并记住选择，帖子／评论／昵称保留原文；Q2 最终注销弹框输入当前密码，保留七天缓冲期；Q3 改为展示邮箱 `zewbby666@gmail.com`，不再投递反馈；Q4 使用支付宝收款码，并已提供原图。无未决取舍。右滑打开后左滑关闭；设备按用户「当前的设备与会话」仅显示当前会话，不新增多设备机制。
- 状态：实现 已完成；验证 本地全套＋最终聚焦＋Android 构建 PASS（设备／远端缺口见下）；合并 未合并；发布 未发布；vivo 新版安装及启动 PASS（2026-10-31）。用户已授权开发新增功能，原有 Push／合并／正式部署边界不扩大。禁止以真实账号注销、真实转账验证；不改变 C/V Gate、密码复验、会话代次和资源归属边界。

### v3 当前证据（持续汇总）

- 2026-10-31 vivo 安装收口：用户明确要求安装，实查 V2323A / `10CEAG17RY003M7`；原 APK 拉取并验签，与已交付 `a578e65` 安装包同签名。`adb install -r` Success，现版本 `0.1.0-settings-v3 / 2026103016`，firstInstallTime 仍 `2026-10-30 09:57:04`，保留数据。启动 Status ok。用户反馈「暂时无法进入」后核实 USB 重连使 reverse 丢失；恢复 `51245→51245 / 49747→54948`，沿原开发 CA 严格检查两服务 `/health/ready` 均200。点击已定位重试后，设备 UI 显示「打开设置／推荐／最新／首页／我的」，无原进入错误提示，进入首页 PASS；未操作 realme。当前安装事实覆盖下方历史未连接快照；完整原生相册／Passkey／系统通知验收仍不在本次安装结论内。


- 用户最新批注确认收款码下方原文：`您的支持将全数用于开发与维护SpeechFUL项目`；已加入支持页及四语言字典。原图资源 `apps/mobile/assets/support/alipay-receive.jpg` 在 `4fd06e3` 入库，SHA256 `3aa21d1a0807ed9c95fbb3d16a95533447961d3fce33e993a46837434e585764`；不重绘、不执行支付。
- P1 基础／认证四语言集成为 `f75cca9/ed44636/37145a5/6df9e7a`；S1 注销／设备／凭据／身份集成为 `cf25dc4/1ca9c27/4b0cf29`；M1 消息／通知／举报集成为 `50fcc84`。执行者聚焦验证分别 47／85／123 项 PASS，均为主机替身证据，不代表原生或真实 SQL 通过。
- root 基于 `50fcc84` 的本轮明确差异：main 全局语言与新路由装配、右滑开启、设置四语言／邮箱／收款码、社区自有文案与原文边界、CI 和对应回归。Windows Flutter 3.47.5／Dart 3.13.4：analyze PASS；设置／手势／真实字体流程 9 项 PASS；社区语言／主导航／跨账号／消息语言 33 项 PASS。日志位于 `E:/Hnuhole-env/profile-settings-20261030/v3-settings-fixed.log`、`v3-cross-route-fixed.log`。后续固定承载提交后复核差异。
- 图片 A/B 诊断：未解码时高度 0 导致默认 800×600 测试视口 offstage；按原图 2:3 预留尺寸后首次布局稳定，测试按真实长页滚动到说明和保存按钮。未添加等待绕过。
- 独立交叉审查：P1 审 root 文案／用户原文边界，发现社区弹层语言冻结与排序漏译；S1 审 M1 发现语言切换误报新消息及弹层菜单翻译冻结；M1 审 S1 发现单独移除注销页面可留下孤儿弹框。上述已分别由交叉修复提交处理并由入口 Session 复核差异。AI 审查不代表 GitHub 成员批准。远端 CI、本轮真机安装、原生 Passkey／相册／系统通知、iOS NOT_RUN。
- root 实现固定为 `68ba0fb`；审查修复集成为 `a42b775`（自有注销弹框退场／密码控制器释放）和 `ce52ccc`（预览版本与语言独立、两个私信菜单即时翻译），入口 Session 已复核差异。注销主按钮在 `8ef8e12` 定名「确认注销」，短语仍严格「确定注销账号」。
- `219bd53` 修复整套发现的字典自动匹配过宽：只含标点的模板不再自动匹配未知内容，显式命名参数继续有效。`ce52ccc` 全套尝试在 1043 PASS／3 SKIP／1 FAIL（上述已修复词条问题）后测试 host 挂起，入口 Session 通过专属 exec session Ctrl-C 停止，故此轮不记为全套 PASS。独立诊断固定 `ce52ccc` 的最后网络测试单项及整份 `post_api_test.dart` 34 项均 PASS，未修改传输实现；证据 `r-full-post-api-file-ce52ccc.log`，仅证明本机 loopback TLS 替身。
- `219bd53`＋随后由 `8ef8e12` 承载的明确差异：18 项最终聚焦（四语言窄屏渲染、模板边界、注销生命周期、消息语言状态）PASS；注销按钮命名后账号／身份语言与注销 10 项 PASS；支持页面独立 5 项 PASS（原图、用途说明、放大、权限拒绝不冒充保存成功）。日志 `v3-final-targeted.log`、`v3-closure-label.log`、`v3-settings-final.log`。实字体 PNG 已检查：原图／说明可读，四语 320 宽 1.8 字号可换行滚动，无溢出。
- vivo 安装延续同一设置任务的用户授权，仅允许原设备 `10CEAG17RY003M7` 同签名覆盖更新；2026-10-30 16:24 实查 ADB 仅 realme `7e0660f1`，vivo 未连接。已异步请用户接回，不操作 realme。本轮安装保持待设备，不冒用 v2 安装结果。
- 最终验证：`8ef8e12`（后续仅本任务记录文档差异）正常 Windows 用户环境、`flutter test --no-pub --concurrency=2 test`：1051 PASS／3 SKIP，耗时 2:26；3 项原有 HTTP fixture 用例缺外部测试环境，未宣称真实 C／SQL 通过。日志 `v3-all-mobile-tests-retry.log`。此重试使用已修复模板和更低并发，不更改测试语义。
- 社区弹层修复 `46e2ee2` 集成为最终产品源 `a578e65`：自身 context 订阅、PopupMenu Builder、评论排序选项、确认框源文案延迟翻译；动作／服务端排序键／窗口检查保持。修复前有独立失败复现，执行者 140 项 PASS；最终集成 `flutter analyze --no-pub` PASS，14 个受影响测试文件 156 项 PASS，含四语窄屏原图／说明／菜单／注销／账号切换，日志 `v3-release-analyze.log`、`v3-release-targeted.log`。未因无关模块重复扩大验证。
- Android `a578e65`：`flutter build apk --profile --target lib/main.dart --target-platform android-arm64 --no-pub --build-name=0.1.0-settings-v3 --build-number=2026103016 --dart-define=AUTH_DEV_PROFILE=true --dart-define-from-file=<沿用的本机USB配置>` PASS，日志 `v3-android-profile.log`。APK 包 `org.hnuhole.hnuhole_mobile`，minSdk29／targetSdk36，版本 `0.1.0-settings-v3 / 2026103016`；v2 签名验证 PASS，证书 SHA256 `1538fb309499d651d7d12d7711a646c4a80fde4b1397857a9e39ce672b3dc9ae`，与本任务 v2 vivo 安装证书一致。APK 59,294,849 字节，SHA256 `ef689411aab4619f18b7342d987a373940e21c0e47b73718a96332984dd25d1b`；解包比对收款码 JPEG 字节与入库原图完全一致。
- 交付安装包 `E:/Hnuhole-env/profile-settings-20261030/SpeechFUL-settings-v3-a578e65.apk`，脱敏本机证据 `E:/Hnuhole-env/profile-settings-20261030/v3-final-evidence.json`。最终 16:39 ADB 仍仅 realme，未安装或操作该手机。vivo 接回后只允许指定序列号同签名 `install -r`，不卸载／清数据；本轮原生相册实际落盘、系统通知、原生 Passkey、iOS、远端 CI 仍 NOT_RUN，不把主机替身或 APK 构建当作设备通过。

### v3 接口、分工与验收

- 统一语言：`AppLanguage` 四值 `simplifiedChinese/traditionalChinese/english/japanese`，语言码 `zh-Hans/zh-Hant/en/ja`；复用 `AuthPresentationController` 的本机非账号偏好存储，兼容旧 zh/en、主题与 english API，欢迎页和设置共享状态。切换即时生效，不更换 Navigator key、不重置会话／主 tab／阅读位／输入。默认简中，退出和换号保留。
- 翻译边界：只翻译明确的应用自有文字、状态／错误、语义与标准控件；不得全局翻译任意 Text 或改写用户内容。公共函数 `hhText(BuildContext context, String source, {Map<String,Object?> args = const {}})`，纯函数 `translateUi(String source, AppLanguage language, {Map<String,Object?> args = const {}})`；源模板用 `{name}` 等命名参数。`UiTranslation` 为 `(String traditional, String english, String japanese)` 记录，模块字典为 `Map<String, UiTranslation>`。缺项不伪装完整，须审计并补齐相关应用文案。
- P1（`/root/profile_tabs`）：新执行分支／Worktree `codex/feat/settings-v3-languages`／`E:/Hnuhole-env/source/HnuHole-settings-v3-languages`，负责 `src/localization/app_language.dart`、`app_localizations.dart`、`translation_catalog.dart`、`translations_auth.dart`、原 `auth_presentation.dart`、标准／读屏语言、本地化 SDK 依赖及锁文件、相关测试。先交可运行基础接口切片，再补认证翻译与覆盖审计。不要编辑 main.dart、auth_screen.dart 或其他模块字典。
- S1（`/root/home_settings`）：`codex/feat/settings-v3-security`／`E:/Hnuhole-env/source/HnuHole-settings-v3-security`；负责独立注销页面和 AuthScreen 去除混杂注销入口、设备独立只读控制器／页面、恢复码与 Passkey 归账号与安全（沿用既有状态机与生命周期）、身份列表垃圾桶、上述模块 UI 本地化及 `translations_security_identity.dart`、对应聚焦测试。不改 main.dart；交付新页面／构造接口供入口 Session 装配。注销主页面只有流程、固定输入短语和红色按钮，短语错误不可提交；最终「确认注销」弹框含密码与取消／确认，确认后才调用既有 AuthFlows，未知结果沿原核对机制，不自动重发。
- M1（复用 `/root/independent_review`，本轮转为实现者，不再审查自己的变更）：`codex/feat/settings-v3-messaging-locale`／`E:/Hnuhole-env/source/HnuHole-settings-v3-messaging-locale`；负责 messaging、system_notifications、reports 自有 UI、相关时间／错误／语义，字典 `translations_messaging_notifications.dart`，Android 私信通知默认文案。为 `DmNotifications` 增加 `Future<void> setLanguage(String languageCode)`，只配置显示语言，不请求权限、不更改用户声音偏好。主入口 Session 装配语言变更通知；不得翻译消息／通知里的用户内容。
- 入口 Session：main.dart 所有路由与全局语言装配，首页右滑、主设置删私信通知、四语言设置、邮箱联系方式、支付宝原图展示／保存及可识别性，posts/comments/navigation 与设置的自有文案，字典 `translations_community_settings.dart`，联合验收／CI／权威状态。收款码原图来源为用户 `941a76c7ca09673f8bffcd3fbb3ec230.jpg`，不重绘／篡改码或执行转账。
- 字典公共接口由 P1 固定，其余三个字典文件由对应维护者独占。基础翻译文件存在前可先做本模块确定性结构改动；基础切片提交后再接入并测试。根分支先创建三个空字典 stub，P1 聚合导入。
- v3 从本次固定契约提交建立三个干净执行 Worktree，旧执行工作保留；规则仍 `a4d7228`。origin/main 拉取本轮因网络不可达失败，使用明确依赖的本地 `ce22f3c`，不声称验证了新的远端主线。各执行者读契约、确认版本后实现，禁止并行写同一文件和改写本记录。
- 验收：四语言主流程、持久化和旧偏好迁移；切换语言不丢用户内容／输入／列表状态；窄屏／大字号；注销短语／密码／双确认／取消／未知结果／过期会话；设备页不加载或显示恢复凭据；账号与安全保留全部凭据功能；邮箱正确可复制；收款码原样可放大与保存；右滑及图片子手势／返回；适用 analyze、聚焦测试、Android 构建与独立交叉审查。缺真机／原生或远端验证时如实标记，不用旧 v2 结果代替。

---

- 模式：指挥（用户明确调用 `$orchestrator`）
- 负责人／状态维护人：入口 Session `/root`
- 执行 Session：`/root/profile_tabs`（P1）、`/root/home_settings`（S1）、`/root/independent_review`（R1）；各用独立分支与 Worktree。
- 分支／Worktree／PR：集成 `codex/feat/profile-settings-integration`，`E:/Hnuhole-env/source/HnuHole-profile-settings-integration`；基线 `a4d7228`（2026-10-30 核对的 `origin/main`）；无 PR。
- 本轮规则版本：`a4d7228` 的 `AGENTS.md`、`CONTRIBUTING.md`、`.agents/skills/orchestrator/`；需求收敛使用本机 `grill-with-docs`、`grilling`、`domain-modeling` Skill。
- 授权依据与边界：2026-10-30 用户消息明确要求实现下列界面调整并调用指挥 Skill；允许调查、隔离开发、验证及必要本地提交。未授权 Push、PR、合并、正式部署或修改正式数据。
- 需求确认：契约 `v2-direct-settings`。用户先前 Q1 的两层入口已被最新截图说明「图一这一栏直接左滑就能看见不要像图二这种要点设置」覆盖；另明确「不要搜索设置这个功能」及「不要有下边的灰色description」。因此首页左滑直接显示完整设置列表，无中转菜单、无搜索、无条目灰色说明。Q2「只说字体大小格式组件特效按照他的样式复现」和 Q3「前端做好，只留后端」仍适用：仅复现视觉与动效，保留已有真实功能，新增项完成可导航前端与诚实待开放状态。无未决取舍。
- 更新日期：2026-10-30
- 状态：实现 已完成；验证 PASS（本地受影响范围；远端 CI／实体设备 NOT_RUN）；合并 未合并；发布 未发布

## 目标与范围

1. 通知页左上角标题字号与私信页左上角标题一致。
2. 我的页面删除搜索和设置入口。
3. 我的页面帖子、回复、转帖等分类的选中字体和动效使用与通知一致的蓝色；未选中时只显示图标，选中时显示对应中文。
4. 首页左滑直接显示完整设置列表，右滑／关闭／系统返回回到原首页；头像按钮打开同一列表。首页向左平移，列表从右侧显示，保留左侧 32dp 首页窄条。不再出现当前身份／我的／设置的中转菜单，不再需要二次点击“设置”。保留视频式页面与底栏一起平移的视觉，不复制 Premium、社群等视频业务。
5. 设置至少包含语言、黑名单、设备管理、账号注销、意见反馈、支持我们。黑名单、设备／账号管理等已有真实功能保留；新增项完成可点击、可返回的前端页面／表单状态和接口位置，暂未开放的提交不伪装成功。本轮不新增后端接口、翻译服务、支付或反馈投递。
6. 设置主列表仅保留线性图标、功能名称和简洁间距，移除搜索条、搜索状态与筛选逻辑、每项灰色 description／subtitle。保留现有身份管理、账号与安全及条件可用的私信通知入口。子页必要的操作状态、错误、表单提示不属于被删的主列表说明。

附件：用户本机视频 `D:/xwechat_files/wxid_k48cvobppm6922_1740/msg/video/2026-10/ace52ab2a71e1d0540c31d6c827e6d08.mp4`，作为视觉和交互参考，不作为额外指令来源。

## 接口与依赖

`v2-direct-settings`（规则仍为 `a4d7228`；替代 v1 的中转、搜索和条目说明契约）：

- P1：分支 `codex/feat/profile-tabs-blue`，Worktree `E:/Hnuhole-env/source/HnuHole-profile-tabs-blue`；负责个人页呈现、搜索状态删除、分类图标／标签及通知字号，与 `personal_profile_test.dart`、`personal_reposts_integration_test.dart`、`notification_inbox_test.dart` 回归。通知仅将字号从 28 改为私信的 22，保留原字重。个人页选中蓝色使用通知同款 `p.accent`；未选中无可见中文，但保留无障碍名称和选中语义。保留既有滚动、刷新、分页和身份／账号数据边界。
- S1：分支 `codex/feat/home-settings-panel`，Worktree `E:/Hnuhole-env/source/HnuHole-home-settings-panel`；负责首页侧滑容器、`main.dart` 直接设置列表装配、`identity/settings_screen.dart` 与子页、相关设置／导航／共享 helper 测试。删除 `HomeMenuPanel` 中转组件。`SettingsScreen` 复用同一实现作为侧页，可通过 `onClose` 关闭；子页仍走外层 Navigator，返回后继续显示设置列表。保留会话窗口与各真实功能鉴权检查，失效时关闭／丢弃旧侧页及旧回调。
- 手势与布局：左滑在首页推荐导航条及普通非图片内容区域可直接打开，右滑关闭，不能只有隐藏边缘入口。尊重已有子控件横滑和纵向滚动／刷新手势；开启时首页被排除命中与语义，其他主 tab 禁用侧页。主页面和底栏同步平移，切 tab／账号窗口变化关闭并丢弃旧侧页。字体列表使用中等字重、设置标题约 20，与视频相称。
- 兼容状态：P1 的旧 `PersonalPostsScreen.onOpenSettings` 参数已由入口 Session 在 `7738f03` 清理。S1 不再引入该参数；根节点装配直接设置 builder，所有真实功能沿现有受保护路由，不绕过鉴权。
- 测试分工：P1 修改自己的三个测试文件；S1 负责其他引用旧头像个人页／旧设置入口的导航与无障碍测试及 `login_home_test_support.dart`。各测试改为通过 `destination:personal` 进入我的页面，`feed-account` 验证新侧栏行为。不得并行改同一文件；必要协调先回报。
- 集成顺序：P1 与 S1 的 v2 差异均已集成，首页左滑 → 直接设置列表 → 子页 → 返回设置 → 关闭回首页、我的分类与账号窗口失效已验证。R1 独立只读审查，最终分支 `codex/test/profile-settings-v2-review@2be6b80`／Worktree `E:/Hnuhole-env/source/HnuHole-profile-settings-review`；旧审查分支保留。执行者只回报，不改写本记录；必要本地提交允许，Push／PR／合并／部署未授权。

## 验收标准

- 用户直接要求覆盖的标题、入口、颜色和标签行为符合上文；数据筛选、刷新、分页与返回行为保持一致。
- 首页顶部导航及普通内容区域左滑、头像点击均直接显示完整设置列表；无中转设置按钮。右滑、关闭或系统返回可回原首页，原通道、推荐／最新选中态、数据与阅读位置保留；从子页返回先回设置列表。页面不复制视频业务功能。
- 六项设置均在列表中；主列表无搜索控件／逻辑、无灰色条目说明，只有图标和名称。已有真实路由可到达，新增项具备明确前端状态，无虚假投递、支付、翻译或注销结果。
- 我的五项分类在切换动画结束后仅选中项显示中文，选中图标／文字／指示条为通知同款蓝色；可访问性名称保留；320 宽度、放大字体与深浅主题无溢出。
- 使用 Flutter analyze、相关交互测试和目标平台构建验证；设备、远端产品 CI 未执行时如实记为 NOT_RUN。

## 当前进度与证据

| 项目 | 被测版本／环境 | 结果与证据 |
| --- | --- | --- |
| 仓库 | `a4d7228`；Windows | 已核对实际根目录、远端、分支和 Worktree；集成 Worktree 为本次新建，保留其他工作区。 |
| 调查 | `349bfd1 / v0-investigation` | P1、S1 已核对规则、契约、分支与干净工作区；通知 28／私信 22；个人页五项文字常驻且选中为墨色；设置仅从我的进入，已有真实黑名单／设备／账号功能。视频 8.73 秒，左菜单进入和后续设置页已逐帧核对。 |
| 需求收敛 | 2026-10-30 用户 Q1 答复及 Q2／Q3 批注 | PASS；已确认仅视觉复现、左侧栏层级与现有功能保留边界，固定 `v1-implementation`。 |
| P1 实现与聚焦验证 | 执行提交 `aa23b0e`，集成为 `ef1d0a1`；Flutter 3.47.5／Dart 3.13.4／Windows | PASS；搜索／设置控件与搜索状态已删除；五分类蓝色选中、未选中文字收起、通知 22 字号；个人页＋通知 11 项及聚焦 analyze 通过，深浅主题真实字体 PNG 已检查。 |
| S1 可运行切片 | 执行提交 `4f4e39c`，集成为 `2123beb` | 首页整体平移侧栏→设置→六项／原有入口已连通，新增语言、反馈、支持前端及接口回调位置；旧入口导航 helper 已迁移。执行者 analyze 与导航／设置已有回调／语义 24 项 PASS，新增手势与表单回归仍进行中。 |
| 首次联合场景 | `2123beb`＋删除无用个人页设置参数＋新增 `profile_settings_flow_test.dart`；Windows | PASS；`flutter test --no-pub test/profile_settings_flow_test.dart` 2 项，覆盖通道／最新／数据／阅读位置在设置往返后保留、搜索与子页返回；真实字体深浅主题、320 宽／1.8 倍字号截图无溢出。完整 `personal_reposts_integration_test.dart` 亦 PASS。 |
| v1 扩大回归 | `7738f03`；Windows；15 个受影响测试文件，日志 `E:/Hnuhole-env/profile-settings-20261030/affected-regression.log` | PASS；164 项，覆盖首页／主 tab／通知／个人／转帖／私信导航／纵向刷新／详情。属于 v1 证据，不代替 v2 导航形态复验。 |
| 最新产品修正 | 用户移除搜索的消息及随后两张截图、移除灰色 description 的消息 | 固定 `v2-direct-settings`；v1 中转菜单、搜索和列表说明的实现／证据不再代表当前目标。P1 范围不变，S1／R1 已读取确认并按 v2 完成交付。 |
| v2 实现 | S1 `59f9980`，集成为 `1f5136a` | PASS；首页普通内容／推荐导航左滑和头像直达右侧设置，页面与底栏整体向左；无中转、无搜索、无主列表 description，保留图标＋名称；右滑／返回关闭，子页仍使用外层 Navigator。已修复普通点击误触横拖 cancel 与禁用后旧指针结束重开问题。 |
| v2 执行者聚焦验证 | S1 `59f9980`；Windows；`s1-v2-tests.log`／`s1-v2-analyze.log` | PASS；手势／设置新测试 8 项＋旧导航／语义／真实回调 24 项，共 32 项；最终 mobile analyze 无问题。普通文字／顶部左滑、图片横滑优先、上下滚动、快速点击、账号窗口及拖动中禁用均覆盖。 |
| v2 首次联合与视觉复核 | `1f5136a`＋v2 联合场景差异；Windows；`v2-integration-focused.log` | PASS；`flutter test --no-pub test/profile_settings_flow_test.dart test/home_side_menu_test.dart test/settings_screen_test.dart` 共 10 项。真实字体深浅／320 宽 1.8 倍截图已目视确认只有名称＋图标，无中转和搜索；从语言页返回列表再关闭后，首页通道／最新／帖子／阅读位保留。 |
| 工具与依赖 | 同上；现有 Flutter／Pub 缓存 | PASS；显式 `PUB_HOSTED_URL=https://pub.dev` 后 `flutter pub get --offline --enforce-lockfile` 与基线 analyze 通过，无锁文件或依赖更改。默认镜像 URL 不匹配导致的首次依赖准备失败已定位，不作为产品缺陷。 |
| 最终静态／扩大回归 | `2be6b8073a83ebac3366fd8b33a72e40cba373e7`；Flutter 3.47.5／Dart 3.13.4／Windows；完整命令及日志摘要见[固定验证证据](evidence/profile-settings-20261030.json) | PASS；`flutter analyze --no-pub` 0 问题，18 个受影响测试文件共 174 项通过。涵盖完整设置往返、普通内容／顶部左滑、图片子手势优先、刷新、私信／通知、个人五分类／转帖、账号窗口与读屏语义。 |
| 最终独立审查 | R1 审查 `a4d7228..2be6b80` | PASS；未发现确定性缺陷，独立运行个人／通知 11 项及菜单／设置 8 项通过；左 32dp 窄条、图标＋名称列表已目视确认。仅为 AI 审查证据，不代表 GitHub 成员批准。 |
| Android 目标构建 | 同一 `2be6b80`；`flutter build apk --debug --target lib/main.dart --target-platform android-arm64 --no-pub` | PASS；默认真实 `main.dart`，包 `org.hnuhole.hnuhole_mobile`，minSdk 29／targetSdk 36；APK v2 签名验证通过，真实字体已打包。APK 112,070,171 字节，SHA-256 `818a80056fc2f472b397bb7d06ebdf856737726b3c9a2286f52320bf2427b1f1`。Flutter engine 为 ARM64，其他插件仍带通用 JNI ABI；没有冒称独立单 ABI APK。沿用已有经官方包摘要校验的 SQLite 缓存，未改依赖或锁。 |
| 产品 CI 与未验项 | `.github/workflows/feed-ui.yml` 已纳入三份新测试和截图产物；本轮未 Push／PR | 远端 CI NOT_RUN；实体设备执行、iOS、正式签名／发布 NOT_RUN。后端／真实 SQL SKIP：本次仅前端和既有路由，不新增接口或修改数据库。 |
| 可见结果 | `2be6b80` 实际界面、真实字体宿主渲染 | PASS；预览 `E:/Hnuhole-env/profile-settings-20261030/final-preview/preview.png`，原图位于同目录与 `apps/mobile/build/profile-settings-render/`。后续交付记录与 `.gitignore` 仅保留证据并忽略 Android 生成的 `.kotlin/` 本地缓存，无运行时／测试源码变化，以上证据仍适用。 |

## 阻碍与下一步

本次已确认的前端开发、本地验证、独立审查与目标构建均已完成。P1／S1／R1 的源码或只读工作树干净，集成分支保留完整本地提交与本记录。新增项保留接口回调和明确的待开放状态，未实现新的后端接口。后续用户授权的 vivo 安装已完成（见下节）；没有推送、PR、合并或正式发布，远端 CI 与实体设备完整功能验收仍为 NOT_RUN。生成的 Kotlin 缓存保留并忽略，APK／日志／截图仅留本机，不纳入提交。没有领域术语变化或难逆转的架构取舍，不新增词汇表条目或 ADR。

## vivo 试用安装

- 授权：用户后续明确要求「安装到VIVO」，覆盖本次功能的 vivo 保留数据更新与启动，不扩展到其他设备或正式部署。
- 来源：`e1e1198`，运行时代码仍为已测 `2be6b80`；以现有 `usb-defines.json`、`AUTH_DEV_PROFILE=true` 构建 ARM64 profile 包，版本 `0.1.0-profile-settings`／`2026103008`。未更改后端；原 CA 联通检查两个服务均 HTTP 200，保留现有 USB reverse。
- 设备：vivo V2323A，serial `10CEAG17RY003M7`。已先读取原安装包核对签名，新旧均为 `1538fb309499d651d7d12d7711a646c4a80fde4b1397857a9e39ce672b3dc9ae`，并核对包名及递增版本。
- 安装与启动：PASS；`adb -s 10CEAG17RY003M7 install -r` 返回 `Success`，显式启动 MainActivity 成功，取得 PID `15617`；版本读回 `2026103008`。首次安装时间仍为 `2026-10-30 09:57:04`，更新于 `14:56:46`（Asia/Shanghai），未卸载或清除应用数据。
- 产物：`E:/Hnuhole-env/profile-settings-20261030/SpeechFUL-profile-settings-2026103008-profile.apk`；构建日志 `vivo-build.log`。安装／启动通过不代表真机完整手势与业务验收；该项仍待用户试用。


## v4 设置入口统一迁移到「我的」页（2026-11-17）

- 基线：`origin/main` `22071c8`，分支 `feat/profile-settings-entry`。参考 `docs/design/profile-settings-prototype.html`；用户当轮确认：hero 区按原型补齐（两枚胶囊复用身份管理 IdentityEditor 流程）、手势仅点击进入（不做滑动开设置/横滑切页签）。本版本反向调整 v2/v3 的入口位置决策，属当次用户新要求。
- 实现（工作区未提交）：`personal_profile.dart` 顶栏「我的」标题换为齿轮按钮（key `profile-settings`），新增 hero 区（IdentityAvatar 大头像、昵称 `profile-hero-name`、「设置图片/编辑信息」两枚胶囊 `profile-hero-photo/info`，胶囊打开当前身份的 IdentityEditor 底部表单）；`main.dart` 新增 `_openSettings`（设置页路由推入，`_settingsRouteOpen` 防重入）与 `_editPersonalIdentity`（身份快照比较后刷新个人投影，与 `_openIdentity` 共用 `_identitySnapshot/_refreshPersonalAfterIdentityChange`），退出登录改为 `popUntil(isFirst)` 后登出；移除 `HomeSideMenu` 组件（`home_side_menu.dart` 删除），首页头像按钮 `feed-account` 改为直达「我的」Tab；`PersonalPostsScreen` 新增 `onOpenSettings/onEditIdentity` 回调；四语言文案新增「设置图片/编辑信息」。设置页条目本身未改动。
- 验证：`flutter analyze --no-pub` PASS（0 问题）。本机 `flutter test` 因 `.dart_tool/package_config.json` 残留旧 SDK 路径（flutter_3_47_5）编译失败，已用 `flutter pub get` 重新生成（用户环境 PATH 为 D 盘 Flutter 3.50.0）；随后按用户要求不在本机跑测试/调试，widget 测试 NOT_RUN，由用户在手机上直接验证。
- 待验（用户手机端）：①「我的」页顶栏齿轮→设置页往返，八个条目（语言/黑名单/设备管理/账号注销/意见反馈/支持我们/身份管理/退出登录）行为与迁移前一致；② hero 头像/昵称随身份切换刷新，两枚胶囊打开 IdentityEditor 且改名/换头像后返回「我的」页即刷新；③ 首页左上头像按钮直达「我的」Tab，首页左滑不再出现设置；④ 退出登录回到欢迎页；⑤ 窄屏/大字号下 hero 不溢出。相关测试文件已同步更新（`profile_settings_flow/talkback_semantics/personal_profile/active_identity/personal_route_account/login_home_navigation/login_home_test_support/navigation_motion`，删除 `home_side_menu_test`），待设备验证后运行。

## v5 设置头像独立化与编辑信息账号化（2026-11-17）

- 用户三点新需求：①「设置图片」改为「设置头像」并独立流程（不再进昵称表单），目标形态为弹层内嵌系统相册、上滑展开；②「编辑信息」去掉头像设置，增加账号信息显示与对应设置页入口；③齿轮按钮缺少按压动画。
- 已确认取舍：内嵌相册本期不做（需新增 Android 原生通道+相册权限，用户拍板后期实现），「设置头像」先直达系统照片选择器；账号信息方案一（只读+入口）曾按用户要求做出原型，随后用户推翻：「编辑信息」不显示账号信息与入口，只保留昵称编辑；设置新增「字体大小」调节，先出原型样式。
- 实现（代码部分，工作区未提交）：`main.dart` 新增 `_setPersonalAvatar`（`NativePostPhotoPicker().pick(1)` → `newAvatarAssetId()` → `prepareIdentityAvatar` 5MB 校验/≤640 缩放/PNG 重编码 → `uploadAvatar(target:)` → `changeAvatar` → 身份快照刷新，失败 SnackBar `_notifyAvatarFailure`）；`PersonalPostsScreen` 新增 `onSetAvatar` 回调，胶囊 `profile-hero-photo` 文案改「设置头像」；齿轮与两枚胶囊均按 app 标准 `HhPressFeedback`（builder+statesController）补按压动画；`newAvatarAssetId()` 从 identity_management_screen 内联提取到 `identity_avatar_photo.dart` 共享；四语言「设置头像」文案更新。编辑信息仅昵称化：`IdentityEditor` 新增 `avatarEditable` 参数（默认 true），`_editPersonalIdentity` 传 `avatarEditable: false` 隐藏头像区与「从图库选择头像/核对原头像/使用默认头像」按钮，昵称未改时提示「请修改昵称」（四语言已加）。
- 原型更新（`docs/design/profile-settings-prototype.html`）：胶囊与笔记改「设置头像」；`photoSheet` 改为内嵌相册网格 mock（灰块照片占位、上滑浏览提示，并注明当前版本直达系统选择器）；相册选择照片后进入 `avatarEditSheet` 头像编辑界面（圆形裁切视图 + 图片要求/不合规提示卡 + 确认/重新选择，均未实现代码）；`editSheet` 只保留昵称表单；设置「偏好」组新增「字体大小」入口（type 图标），`renderFontSize` 页面四档单选，`applyFontScale` 视口缩放全局即时生效。字体大小与裁切编辑界面均为纯样式，待用户确认后实施代码。
- 验证：代码部分 `flutter analyze --no-pub` PASS（0 问题，含本轮 avatarEditable 改动）；widget 测试 NOT_RUN（用户手机端直接验证）。

## v6 转场/页签动画、扁平卡片、设置分组与内嵌相册（2026-11-17）

- 用户五点新需求（均已确认）：①设置页从左推入转场（旧页向右推出）；②「我的」页五页签帖子卡片改原型扁平样式（通栏白底+底部分割线），范围仅「我的」页；③五页签切换动画（内容平移 22%+淡出/淡入）+ 内容区左右横滑手势（覆盖 v4「不做横滑」决定）；④设置页按原型分组重构（偏好/账号与安全/社区与隐私/支持与关于，行=图标+标题+副标题+值+chevron，红色注销行+红色退出大按钮），新增主题三选、字体大小四档全局缩放、账号资料（只显示有数据字段，平台编号可复制）、关于页；修改密码无可复用 API，按现有机制标暂不可用；⑤内嵌相册弹窗（用户拍板 photo_manager 插件方案）→圆形裁切编辑界面→上传，权限拒绝回退系统选择器。
- 实现（工作区未提交，分支仍 `feat/profile-settings-entry`）：
  - 转场：`main.dart` `_openSettings` 改 PageRouteBuilder（340ms `Cubic(.32,.72,.24,1)`，新页 `Offset(-1,0)`→0），home 子树用 `_settingsPush` 控制器同步右推（pop 反向时由 route status listener 触发 reverse）。
  - 页签动画/手势：`personal_profile.dart` mixin 新增 `_tabSwitch` 控制器（200ms 两段：出向平移淡出→换内容→反向滑入），`_selectTab` 支持 HhMotion.reduced 直接切换；内容 slivers 经自定义 `_SliverTransform`（RenderSliver paint 变换）+ `SliverMainAxisGroup` 包裹；CustomScrollView 外层 GestureDetector 横滑切页签（>72px 或 >420px/s，仅 0-4 范围；原型限定内容区，本实现整页横滑均可触发，hero 区横滑也会切页签）。
  - 扁平卡片：新增 `_ProfileFlatCard`（原型 .post：通栏 surface、padding 18/20/14、底部 1px line 分割线、head=头像+作者+时间+more、title 16 w600、body 14/1.75 最多 8 行、动作行右对齐 comment/favorite/like，`_FlatCardAction` 带选中态），帖子/回复/转帖/收藏五页签全部替换 HhXPostItem；转帖加 accent 色「{v0} 转帖」标签，移除转帖按钮保留于动作行；互动失败提示移入卡片内。
  - 设置重构：`settings_screen.dart` 分组 `_SettingsGroup`（组标签+surface 容器上下边线+行间分隔线）+ `_SettingsRow`；新页面 `_ThemeSettings`（三选，绑 `AuthPresentationController.setThemeMode`）、`_FontSizeSettings`（四档+预览卡）、`_AccountProfile`（平台编号复制+用户名，数据由 main.dart 传入 `accountId`/`username`，无数据显示占位）、`_AboutSettings`；修改密码入口开 `_SettingsUnavailable`；保留原条目 key（settings-language/blacklist/devices/closure/identity/feedback/support/logout/account），ValueKey('settings-logout') 移到退出大按钮。
  - fontScale：`AuthPresentationController` 新增 `fontScale`(small .875/standard 1/large 1.125/xlarge 1.25) 持久化到原偏好 JSON（旧记录缺 key 时默认 standard，不破坏已存主题/语言）；`main.dart` MaterialApp.builder 用 `TextScaler.linear` 全局应用。
  - 内嵌相册：pubspec 新增 `photo_manager 3.12.0`（`flutter pub add` 解析，pubspec.lock 同步 test_api 0.7.14）；AndroidManifest 增 `READ_MEDIA_IMAGES`/`READ_MEDIA_VISUAL_USER_SELECTED`/`READ_EXTERNAL_STORAGE(maxSdk 32)`；新文件 `identity_avatar_flow.dart`：`pickIdentityAvatarImage`（权限 `requestPermissionExtend`，授权→`_AlbumSheet` 4 列缩略图网格（originBytes 选图）；拒绝/有限授权→回退 `NativePostPhotoPicker`；弹窗划走=取消）→ `_AvatarCropEditor`（InteractiveViewer 拖动/双指缩放，圆形裁切环蒙版，5MB/动图/解码失败提示显示在编辑界面内并禁用确认，输出≤640px 圆形 PNG；「重新选择」回到相册/系统选择器）；`_setPersonalAvatar` 接入该流程。
  - i18n：`translations_community_settings.dart` 新增约 40 词条（分组标签/主题/字体大小/账号资料/裁切提示等四语言）。
  - 测试同步：`settings_screen_test.dart` 退出断言改为分组结构（「账号与安全」组标签常驻+条目行不出现），`personal_profile_test.dart` 断言 HhXPostItem→`flat-post-card`；其余引用 `profile-tab-*`/`repost-undo:*`/`settings-logout*` 的测试 key 均保留。
- 验证：`flutter analyze` 0 error / 0 warning；剩余 167 条 info 均为本机 Flutter 3.50.0 预览版弃用 `package:flutter/material.dart` 的 `deprecated_member_use`（改动前基线已有 115 条同源噪音，全仓范围）。widget 测试 NOT_RUN（沿用用户要求，手机端直接验证）。
- 待验（用户手机端）：①齿轮→设置从左推入、旧页右推、返回反向一致（含系统返回）；②五页签点击与横滑切换动画方向、指示条跟随、滚动位置保持；③帖子/回复/转帖/喜欢/收藏卡片扁平样式与分割线、动作按钮（评论/收藏/点赞）可切换、转帖标签与移除按钮；④设置分组/主题切换/字体大小全局生效并持久化/账号资料复制/关于；⑤相册弹窗（首次授权弹窗）、选图→裁切→确认上传；拒绝权限→系统选择器→裁切；超 5MB/动图提示且确认禁用；⑥横滑切页签与滚动手势不冲突。
- 已知取舍：横滑切页签在全页生效（原型仅内容区，因单滚动视图无法限制手势起点，如需严格限制可再议）；修改密码入口暂显示「不可用」；账号资料仅平台编号+用户名（会话无其他字段）。

## v7 用户试用反馈修复（2026-11-17）

- 用户 v6 试用反馈五点（均已按反馈实现，无未决取舍）：
  1. **设置按钮点击无效**（bug）：根因 `main.dart _openSettings` 在 `navigator.push(route)` 之前调用 `route.animation!.addStatusListener`，PageRouteBuilder 的 animation 在 push 内才创建，push 前为 null → 崩溃即「点击无效」。修复：先 `push` 取得 Future，再挂 status listener、`_settingsPush.forward`，`whenComplete` 里摘除；animation 访问改 `?.` 空安全。
  2. **头像选图无反应**（bug）：a) `_setPersonalAvatar` 中 `pickIdentityAvatarImage` 调用在 try 外，相册通道异常（photo_manager 新插件需完整重新构建，hot reload 后 MissingPluginException）未捕获 → 静默失败；修复：调用纳入 try/catch，失败 SnackBar「无法完成头像选择，请重试。」。b) `identity_avatar_flow.dart _pickSource`：`originBytes` 在部分授权（READ_MEDIA_VISUAL_USER_SELECTED）下可为 null → 原实现静默 return；修复：null 或相册通道异常时回退系统选择器（`NativePostPhotoPicker().pick(1)` + `inspectPostPhoto`），弹窗划走仍=取消，权限拒绝仍回退系统选择器（与 v6 一致）。
  3. **页签横滑切换生硬/无动画**：重构两段式动画驱动——`_SliverTransform`/`RenderSliverTransform` 改为持有 `Animation<double>`（controller），RenderObject attach 时 addListener→markNeedsPaint 逐帧重绘，不再每帧 setState 重建整页；widget 仅在相位切换（离场 completed→入场 reverse 开始、dismissed 结束）时重建。离场：内容向切换方向平移 22%+淡出（v:0→1）；入场：新内容反向滑回（v:1→0）。`_tabPhaseOut` 在 `_applyTab` 重建前翻转，确保首帧传对相位。
  4. **页签图标下方文字不显示**：旧实现文字仅在选中态随 progress 展开（v2 契约「未选中只显示图标」被原型覆盖）。按原型 `.ptab` 改为图标+11px 文字常显；选中 accent+w600（FontWeight.lerp 过渡），未选中 secondary+w400；保留指示条动画与「帖子」排序下拉箭头。`personal_profile_test.dart` `expectProfileSelection` 断言同步为五标签常显（颜色仍验选中 accent）。
  5. **横滑手势分区**（覆盖 v6「全页生效」取舍）：hero SliverToBoxAdapter 加 `_heroKey`，`onHorizontalDragStart` 以 hero 底边全局 y 判定起点区域。hero 区（顶栏+头像资料块）**右滑→打开设置**（左滑无动作，不再切页签）；内容区左滑→下一页签、右滑→上一页签，**帖签（index 0）右滑→打开设置**（即侧滑开设置）。阈值不变（>72px 或 >420px/s）。
  6. **修改密码入口复用**：`AuthScreen` 新增 `initialRecovery` 参数（initState 直入 `_AuthMode.recover` 找回分区），并在登录态（session authenticated）且 mode==recover 时渲染找回流程（校邮重置密码），含 `resetCodeConfirmation` 时直接进「设置新密码」页；重置提交后旧会话失效、走既有 resetCommitted 流程。`SettingsScreen` 新增 `onChangePassword` 回调，`main.dart _openPasswordReset` 以 `initialRecovery: true` 推入 AuthScreen（不传 onAuthenticated，避免登录态下 `_awaitingHome` 忙碌锁锁死操作）。入口行为等价「登录页找回账号→校邮重置密码」。
- 验证：`flutter analyze` 0 error / 0 warning（167 条 info 均为本机 Flutter 3.50 预览版弃用 material.dart 的既有噪音）。widget 测试 NOT_RUN（沿用用户要求，手机端直接验证）。
- 待验（用户手机端）：①齿轮点击打开设置、转场推动动画正常；②相册选图→裁切→上传全链路（含部分授权/拒绝回退系统选择器、hot reload 后异常有 SnackBar 提示）；③五页签点击与横滑切换有平移淡出/淡入动画；④页签图标下文字常显、选中高亮；⑤hero 区右滑开设置、帖签右滑开设置、内容区横滑切页签；⑥设置→修改密码进入校邮重置流程，完成重置后旧会话失效、重新登录正常。
- 已知取舍更新：v6「横滑全页生效」已被本手势分区取代；v6「修改密码暂不可用」已被校邮重置复用取代（重置使所有旧会话失效，无「改密不登出」语义——沿用登录页既有流程边界）。

## v8 用户二次试用反馈修复（2026-11-17）

- 原型更新：删除 hero 区「在线」状态（模板+CSS，功能从未实现）。邮箱显示经核实后放弃：后端 `SessionView` 不返回邮箱且用户确认本轮不改后端；「账号资料」副标题由「平台编号、用户名与绑定校邮」改为「平台编号与用户名」（i18n 同步）。
- 六点反馈与实现（均工作区未提交，分支 `feat/profile-settings-entry`）：
  1. **设置行值/箭头停在行中部**：根因 `_SettingsRow` 右值用 `Flexible`（默认 flex:1），与标题 `Expanded` 平分剩余宽度（截图实测 v1.0 停在 48–54% 屏宽）；修复：值改普通非弹性子项，标题独占剩余宽度，值+chevron 贴右。
  2. **我的页滚动**（用户确认保持整页滚动）：整页拉伸根因是 Android 12+ stretch overscroll 指示器，`ScrollConfiguration.copyWith(overscroll: false)` 关闭（仅我的页；RefreshIndicator 依赖 overscroll 通知不受影响）；斜向上滑误触由水平识别器与竖滚同阈值竞争所致，新增 `_ProfileHorizontalDragRecognizer extends HorizontalDragGestureRecognizer`（重写 `hasSufficientGlobalDistanceToAccept` 阈值×2）+ `RawGestureDetector`，横滑需更明确水平意图才接管，竖滑优先。
  3. **浅色主题底色调浅**：light `background` 与 `lightCard` 由 `0xFFF1F1F3` → `0xFFF7F8FA`（更浅更清新；post_theme 引用 lightCard 随之同步）。
  4. **编辑信息→编辑昵称**：hero 胶囊改「编辑昵称」（i18n：繁/英/日）；`IdentityEditor` 标题在 `avatarEditable:false` 时显示「编辑昵称」（身份管理路径保留「编辑身份」，新词条入 security_identity 字典）；标题与输入区间距收紧（无头像区时 24+24 → 14+14）。
  5. **页签切换动画仍生硬**：v7 重构的相位残留 bug——`_selectTab` 在 setState 外直接置 `_tabPhaseOut=true`，第二次起离场相位未传入 RenderObject，动画方向错乱；修复：switch 启动时 setState 传入相位。另加 `CurvedAnimation`（easeOutCubic 入场减速，与全局 HhMotion.curve 一致）。用户确认系统未开减弱动画/无障碍，`HhMotion.reduced` 策略未改动。
  6. UI 检索引用：ui-ux-pro-max（easing 指南：入场减速）与 hnuhole-ui-craft 已加载，定向修正未改版式。
- 验证：`flutter analyze --no-pub` 0 error / 0 warning / 0 info（连既有 deprecated 噪音本次也未出现）。widget 测试 NOT_RUN（用户手机端直接验证）。
- 待验（用户手机端）：①设置行值/箭头贴右（主题/语言/字体大小/关于 v1.0）；②我的页上滑卡片正常滚动、不整页拉伸、斜滑不误触，下拉刷新仍在；③浅色主题底色观感；④「编辑昵称」胶囊→表单标题「编辑昵称」、间距紧凑、改名流程正常；⑤五页签横滑/点击切换有平滑平移淡出动画（重点连续切两次以上验证方向正确）。

## v9 六项交互修复（2026-11-17，上账号积分中断轮，本账号续查）

- v9 六任务核对结论：①滚动/刷新手势冲突（RenderSliverTransform parentUsesSize+hitTestChildren 修复、横滑阈值×2）②相册弹窗根因定位 ③SnackBar 自动消失+手动关闭 ④页签两段式动画 ⑤分组标题 14/w600 ⑥设置子页转场——代码在中断前均已完成；中断遗留仅 profile_settings_flow_test 的调试残留（unused import/变量、debugPrint），已清理。
- 设备端验证（19:55 截图反馈）暴露统一根因：**手机仍运行 v7 时代构建**——build 产物时间戳显示 16:48（v6 期间）后无成功设备构建（17:55 完整构建因 C 盘满失败），v8/v9 改动从未装机；photo_manager 原生通道因此未注册 → 相册弹窗静默回退系统选择器。

## v10 设备反馈四项修复（2026-11-17）

1. **头像选择恒失败**：根因——系统选择器回退路径复用帖子级校验 inspectPostPhoto（10MB/仅 PNG/JPEG/GIF/WebP/像素预算），HEIC 或大尺寸照片抛 FormatException 被外层 catch 吞成笼统“无法完成头像选择”，且到不了裁切界面。修复：回退路径不再套用帖子级校验，仅保留 5MB 读取上限（超限明确提示“照片超过 5MB，请选择更小的照片。”）；格式/动图/解码要求全部由圆形裁切编辑界面在界面内提示；新增统一 notifyAvatarBlocked（3s 自动消失+关闭，main.dart _notifyAvatarFailure 改为复用，原 4s）；上传阶段失败改显示服务端错误或“头像上传失败，请稍后重试。”（裁切输出恒 ≤640px PNG，旧 5MB 格式提示已失真）。i18n 新增 2 词条（相册权限回退提示/上传失败）。
2. **相册弹窗**：代码/权限/弹窗自 v6 就绪；权限被拒回退时新增可见提示“相册权限未授予，已改用系统选择器。”以区分“权限被拒”与“未实现”；通道异常仍 debugPrint 记录。待用户确认是否做过完整重建后复验。
3. **修改密码**：来源= v7 轮确认“复用登录页找回密码流程”→实现为 AuthScreen.initialRecovery+登录态找回分支（与登录页找回账号同一 `_recovery()`/`_emailRecovery()` 表单）。用户以截图澄清：要的就是登录页「通过校邮重置密码」页（校邮输入→发送验证码→设置新密码），当前实现渲染的正是该表单；设备上不符的观感指向旧构建（v6 的修改密码为 `_SettingsUnavailable`「暂不可用」占位）。本轮唯一代码修复：登录态找回分支的「返回登录」在设置推入语境下改为 pop 返回设置页（原实现切到「账号已登录」分区，语义错误）。
4. **动画对齐设置页进入**：新增 HhMotion.pageSlide/pageSlideCurve（340ms Cubic(.32,.72,.24,1)）共享常量；设置子页 _open 改从左推入+同曲线时长（原从右 260ms easeOutCubic）；设置列表经 secondaryAnimation 在子页/修改密码等全屏页推入时同步右移、返回滑回（与首页被设置页右推同模式）；页签内容动画与指示条曲线统一为 pageSlideCurve。设置页转场常量收敛到 HhMotion（删 main.dart 私有 _settingsCurve/_settingsTransition）。从设置打开的 main.dart 全屏页（设备管理/注销/黑名单/修改密码）仍为平台默认转场，是否统一待议。
- 验证：`flutter analyze --no-pub` 0 error / 0 warning（4 条 info 均存量：personal_profile applyPaintTransform deprecated translate、personal_reposts_integration_test 3 处 prefer_const）。widget 测试 NOT_RUN（用户手机端验证，需完整重建后进行）。

## v11 根因收口：预览入口、SnackBar persist、动效门控与滚动恢复（2026-11-17）

- **确认用户手机运行的是 `lib/dev/preview_app.dart` 预览入口**（截图身份「夜航船」与种子帖「期末周的图书馆/食堂新开的窗口」即 preview_app 写死数据），多轮“改了没效果”的观感由此而来：部分问题出在预览配置而非正式代码。
1. **修改密码显示恢复码**：preview_support.dart 构造 `AuthFlows`/`SecurityManagementController` 漏传 `legacyRecoveryEnabled: false`（类默认 true → 渲染恢复码版；正式 main.dart 两处均传 false）。已补齐预览两处配置与正式入口对齐。
2. **通知不自动消失（根因查实）**：本机 Flutter 3.50 预览版 SnackBar 新语义——`persist = persist ?? action != null`（snack_bar.dart:303），带 action 的 SnackBar persist 默认 true，定时器到点直接 return（scaffold.dart:622）。即带「关闭」按钮的通知永不自动消失，duration 形同虚设；v9 那轮“改为自动消失”实际从未生效即此因。修复：notifyAvatarBlocked 显式 `persist: false`（全 app 仅此一处带 action 的 SnackBar）。
3. **动画整体无门的根因**：用户手机有第三方无障碍服务（跳广告类工具注册为 AccessibilityService）→ Flutter `MediaQuery.accessibleNavigation=true` → `HhMotion.reduced` 判真 → 页签切换/指示条/子页转场/按压反馈时长全部归零；唯「进入设置」转场历史代码未接门控（固定 340ms）故仍有动画——形成“只有进设置有动画”的假象。用户最终拍板（覆盖首轮“只认系统开关”方案）：`HhMotion.reduced` 恒为 false，动画完全不受任何门控（无障碍标记、系统减弱动画开关）影响，始终播放；保留方法签名兼容既有调用点。
4. **横滑切页签跳回顶部**：_applyTab 在内容切换帧立即 jumpTo，异步页签（回复/收藏等）数据未到、maxScrollExtent 过小，恢复值被夹到顶部。修复：新增 `_restoreTabScrollOffset`，post-frame 逐帧重试至 extent 足够（上限 30 帧≈0.5s），用户已手动滚动则放弃恢复。
5. **相册权限被拒**：新增 AlertDialog「去系统设置授权」（PhotoManager.openSetting）/「改用系统选择器」替代原 toast 回退；授权后用户重试即见相册弹窗。i18n 新增 3 词条、移除已不用的回退 toast 词条。
## v12 转场统一、相册权限误判修复与连续切页签（2026-11-17）

1. **转场统一**（用户明确指定）：①设置子页全部统一为平台默认转场——`_open` 由自定义 PageRouteBuilder（从左滑入）改为 `MaterialPageRoute`，与「修改密码/身份管理/黑名单」等 main.dart 推入页完全一致（Android 为缩放覆盖式滑入：从右下向左上覆盖当前页）；移除设置列表的 secondaryAnimation 右移包装（子页为覆盖式，不再推动下层）。②设置页进入改为从上方滑入并覆盖当前页（overlay 滑动效果，Offset(0,-1)→0，保留 340ms pageSlideCurve）；随覆盖式转场移除 `_settingsPush` 首页右推机制（声明/监听/forward/reverse/dispose/首页包装全部清理）。
2. **相册权限误判（根因查实）**：photo_manager 3.12 的 `PermissionState.isAuth` 只认完全授权（authorized），Android 14「仅限所选照片」部分授权（limited）被误判为未授权——用户已授权却仍提示无权限即此因。修复：判定改用 `hasAccess`（authorized || limited，部分授权下相册弹窗可用）；按用户要求整体移除 v11 的权限对话框（requestPermissionExtend 未授权时本就自动弹系统授权框，被拒才回退系统选择器）；i18n 移除对话框 3 词条。
3. **连续切页签**：`_selectTab` 动画进行中再次滑动/点击时，先 `_tabSwitch.stop()` + 立即落在当前目标（`_applyTab(_tabTarget)`），随即开始下一次两段式切换——每次手势都推进一个页签，不再等待入场动画播完；替换原“仅离场相位可重定向、入相位丢弃”逻辑。
## v13 转场定稿与右滑卡顿修复（2026-11-17）

1. **设置页进入**（用户指定）：从上方滑入改为**从左向右横向滑入并覆盖**（Offset(-1,0)→0，340ms pageSlideCurve），返回反向；v12 的 _settingsPush 已在覆盖式转场下移除，未再引入。
2. **子页转场定稿**（用户指定，微信式）：新增共享路由 `HhSlidePageRoute`（motion.dart）——新页从右缘**纯位移**滑入覆盖（无渐变/缩放/透明度），340ms pageSlideCurve。落地范围：settings `_open` 全部子页 + main.dart 六个推送点（修改密码/身份管理/设备管理/账号注销/恢复凭据/黑名单），全部脱离 MaterialPageRoute（Android 缩放渐变转场）。设置列表以 secondaryAnimation 做**微信式视差**（子页推入时左移 30%、返回滑回），保证方向/层级/覆盖关系正确。
3. **头像选择器**：本轮零改动（保持 v12 的 hasAccess 修复，用户确认系统选择器已可用）。
4. **页签滑动方向澄清后修复**（用户更正：左滑进下一页签流畅、**右滑回上一页签卡顿**）：根因——右滑回到已访问且滚动较深的页签时，内容切换帧先渲染在错误位置、下一帧才由 _restoreTabScrollOffset jumpTo 恢复，产生可见跳变；左滑目标多为顶部/新页签（目标偏移小）故无感。修复：_applyTab 在 setState 换内容的同一同步块 jumpTo(目标)（不按旧内容范围钳制，forcePixels 无范围断言；新内容布局范围不足时自动收正），换内容那一帧直接渲染在正确位置；原逐帧补偿链保留仅用于未访问页签数据未到的场景。放弃预取方案（左滑本就流畅，不引入额外请求）。
## v14 全屏手势相册弹窗与横滑角度放宽（2026-11-17）

1. **头像选择弹窗升级为手势展开全屏**（用户指定）：`_openAlbumSheet` 改用 `DraggableScrollableSheet`（initialChildSize .62 / maxChildSize 1.0 / snap [.62, 1]）——弹窗打开后上滑跟手展开至全屏（全屏选图模式），下滑收起；相册网格挂 sheet 的 scrollController（滚到顶再上滑即展开）。头部随展开进度补状态栏内边距（AnimatedBuilder 仅重建头部，缩略图 FutureBuilder 不受拖动影响）；新增「从系统相册选择」头部按钮（复用 _useSystem 回退机制，部分授权下网格只见所选照片时可直达系统选择器）与「×」关闭；加载/空/错误态改 SliverFillRemaining。i18n：新增「上滑展开全屏…」「从系统相册选择」，移除旧副标题词条。
2. **横滑角度与阈值放宽**（“偶尔卡在某一页”根因）：原 `_ProfileHorizontalDragRecognizer` 用 2 倍距离阈值（SDK 的 globalDistanceMoved 为带符号路径长度），斜滑在竖滚识别器到达 1 倍 slop 后即被竖滚抢走 → 翻页失效观感即“卡页”。改为自跟踪净位移按角度判定：|dx| > 1.1·slop 且 |dx| > 1.1·|dy|（横向 ±42°内）即接管——接近横向的斜滑可翻页，纵向滚动不受影响（dy 主导时横滑识别器永不接受）。`_onProfileDragEnd` 翻页阈值同步放宽 72px/420 → 56px/320。
## v15 弹窗内相册分类切换（2026-11-17，用户澄清需求）

- 用户澄清：要的是**弹窗内直接切换设备相册分类**（相机/截屏/收藏等），不是系统选择器入口——系统选择器割裂感大，不要。v14 头部的「从系统相册选择」按钮已移除。
- 实现：`_AlbumSheet` 新增相册分类状态（`_loadAlbums` 经 `PhotoManager.getAssetPathList(hasAll: true)` 拉取设备相册列表，默认选 isAll 的“全部照片”）；头部改为相册选择 chip（PopupMenuButton：folder 图标 + 当前相册名 + 下拉箭头，accent 色，复用 HhMotionRoutes.popup 动画），点选即在弹窗内切换、网格重载所选相册（`AssetPathEntity.getAssetListPaged(page, size: 120)`——注意该方法参数是 size 不是 pageCount，与 PhotoManager.getAssetListPaged 签名不同）；相册列表加载失败静默不阻塞网格。错误态的「改用系统选择器」保留仅作读取失败的兜底。i18n：新增「选择相册」「全部照片」，移除「从系统相册选择」。
## v16 相册选择器头部扁平化改版（2026-11-17，参考 ui-ux-pro-max）

- 用户要求：弹窗顶部 UI 再优化，相册选择按钮更明显，下拉列表扁平风格，默认相册 all，按相册默认顺序展示。已加载 ui-ux-pro-max（要点：Android 48dp 触控行高、语义色 token、选中态勾+字重不只靠颜色、零阴影扁平面板）。
- 头部重排：`[设置头像 … ×]` 标题行 → 副标题 → **整宽相册选择按钮**（background 底色块 + 1px line 边框、圆角 12：photo_library 图标(accent) + 相册名(15/w600/ink) + 照片数(12/secondary，assetCountAsync 缓存到 _albumCount 避免随弹窗拖动逐帧重建反复查询) + 下拉箭头），替代原右上角小 chip。
- 下拉面板扁平化：PopupMenuButton `elevation: 0` + surface 底 + 1px line 边框圆角 14 + 透明 surfaceTint（去除 Material 阴影卡片观感）；菜单项 48dp 行高：选中项 check 图标(accent)+w600 字重、未选中 photo_library 图标(secondary)，右端相册照片数（FutureBuilder assetCountAsync，仅菜单打开时构建）。
- 默认相册 all 与排序：维持 v15 逻辑——默认选 isAll 的“全部照片”，`getAssetListPaged` 返回相册原生顺序（新→旧），未另行排序。
## v17 下拉修角、All 置顶改名与网格分页（2026-11-17）

1. **选中/悬停高亮方角突出修复**：PopupMenuButton 增加 `clipBehavior: Clip.antiAlias`——子项高亮（选中/悬停/按压的墨水层）被裁剪到面板圆角内，不再向外突出两个方角；选中态（勾+字重）与悬停态样式一致。
2. **第一项 All 而非 Recent**：查实 photo_manager Android 端 `hasAll: true` 时在头部合成 isAll 相册但名称硬编码为"Recent"（PhotoManager.kt ALL_ALBUM_NAME）。修复：`_loadAlbums` 对相册列表做 isAll 置顶分区排序；UI 层 `displayName` 把 isAll 相册展示名覆盖为本地化「全部照片」（下拉项与触发按钮一致）。
3. **全部照片分页懒加载**：网格由一次性 120 张改为分页（`_pageSize=90`），滚动接近底部（extentAfter<600）时 `_loadMore` 取下一页追加、底部显示小加载圈；取空即止（自然降级为首屏数据），切换相册重置分页。展示所有照片不再受一次性加载限制。
- 验证：`flutter analyze --no-pub` 0 error / 0 warning（同前 4 条存量 info）。待完整重建 preview_app 后复验：①下拉选中/悬停高亮无方角突出；②第一项「全部照片」且默认选中；③全屏下滑动网格持续加载后续照片。

## v18 相册切回顺序与账号资料右对齐收口（2026-11-18）

- 用户续查剩余两个试用 bug；沿用 `feat/profile-settings-entry` 与基线 `22071c8` 的既有未提交工作，不另建分支、不覆盖其他改动。
- 核对确认中断前两项实现均已落盘：①相册首屏与分页请求使用 `_epoch` 代次，切到其他相册再切回「全部照片」时，过期请求不得写回或追加旧列表；分页仍按当前相册原生返回顺序追加，并按资源 id 去重。②账号资料行复用设置行的右对齐布局方式，标题 `Expanded` 占满剩余空间，实际值与「复制」按钮作为普通尾部子项贴右，不再由 `Spacer + Flexible` 停在中间偏右。
- 本轮不重复重写已有修复，仅对上述两个源码文件执行项目标准格式化；`flutter analyze --no-pub <identity_avatar_flow.dart> <settings_screen.dart>` PASS，0 问题。
- widget 测试 NOT_RUN（沿用用户此前要求，不在本机跑测试／调试）；合并、发布 NOT_RUN。待手机完整重建后复验：多次快速切换相册再回「全部照片」顺序保持稳定、无跨相册旧数据追加；账号资料的平台编号、用户名及复制按钮均位于行右侧。

## v19 相册按钮反馈、最新优先与账号长编号溢出修复（2026-11-18）

1. **相册切换按钮反馈突出圆角**：查实 `PopupMenuButton.clipBehavior` 只裁剪下拉菜单，按钮内部 `InkWell` 由独立参数 `borderRadius` 约束。为按钮补 `BorderRadius.circular(12)`，与现有按钮装饰圆角一致，加深反馈不再突出四角；不替换既有弹窗组件。
2. **全部图片最新优先**：查实 `photo_manager 3.12.0` 的 `FilterOptionGroup` 支持 `OrderOption(createDate, asc:false)`。相册列表与无相册兜底分页查询共用同一降序过滤；`AssetPathEntity` 持有该过滤并用于后续分页，首屏和追加页均按创建时间新→旧返回，不在内存中跨页重排。
3. **账号资料横向溢出**：截图中的黄黑区域与红字为长平台编号作为无约束普通子项挤占标题和复制按钮所致。改为标题保持固有宽度且单行，标题与值间距 16，值以 `Expanded` 限定在剩余空间内、右对齐并末尾省略，复制按钮保持最右且可取得完整编号；消除标题逐字换行和 RIGHT OVERFLOWED。
- 验证：`dart format` PASS；`flutter analyze --no-pub <identity_avatar_flow.dart> <settings_screen.dart>` PASS，0 问题。widget 测试 NOT_RUN（沿用用户此前要求）；合并、发布 NOT_RUN。待完整重建后设备复验按钮按压四角、全部图片首屏及分页的新→旧顺序、长编号省略与复制按钮无溢出。

## v20 设置分组标题与相册按钮反馈、箭头动画（2026-11-18）

1. **设置分组标题**：原首标题实际上方留白为列表 12 + 分组 12，下方仅 8，且各分组标题上下不等距。调整：移除列表顶部内边距，分组标题统一上下 12，标题位于上一区域与本组内容间隔正中；字号 14→16（w600 不变）。
2. **相册切换按钮反馈**：v19 的 `borderRadius` 只约束 InkWell 边界，不负责提供水墨宿主；按钮原有 `Container` 背景不足以保证按压反馈稳定绘制。改为外层 `Material`（background 底、12px 圆角、1px line 边框、antiAlias 裁剪）承载 PopupMenuButton 内部 InkWell，背景与边框职责从 Container 移到 Material，反馈沿同一圆角显示。
3. **相册下拉箭头动画**：新增 `_menuOpen` 状态，`onOpened/onCanceled/onSelected` 同步开合；右侧箭头改 `AnimatedRotation`，打开时 180° 反转向上，时长复用 `HhMotion.quick`（与身份切换等既有控件一致）。
- 验证：`dart format` PASS；`flutter analyze --no-pub <identity_avatar_flow.dart> <settings_screen.dart>` PASS，0 问题。widget 测试 NOT_RUN（沿用用户此前要求）；合并、发布 NOT_RUN。待设备复验：各分组标题上下等距且居中、字号观感；按钮按压反馈恢复且不出圆角；下拉打开/关闭箭头上下翻转。

## v21 半屏相册下拉列表不再上翻覆盖按钮（2026-11-18）

- 用户反馈：头像选择器半屏时点击相册按钮，列表覆盖在按钮上方；全屏时正常出现在下方。查实 Flutter `_PopupMenuRouteLayout._fitInsideScreen` 在菜单底边越界时会把整体上移，而非翻转锚点；半屏时按钮全局位置偏低、固定 `maxHeight=376` 超出屏幕底部，因此被上移盖住按钮。
- 修复：为按钮挂 `GlobalKey`，构建菜单约束时按按钮底边的全局坐标计算“屏幕底部－按钮底边－16px”的可用高度，`maxHeight` 在 112–376 之间收缩；半屏自动变矮、全屏保持原最大值，菜单始终从按钮下方展开。首帧无 RenderBox 时用 240 的半屏安全高度，相册数据到达触发重建后按实际位置重算。
- 验证：`dart format` PASS；`flutter analyze --no-pub <identity_avatar_flow.dart>` PASS，0 问题。widget 测试 NOT_RUN（沿用用户此前要求）；合并、发布 NOT_RUN。待设备复验：半屏/全屏分别打开相册列表均位于按钮下方，半屏列表高度收缩且可滚动，不再覆盖按钮和标题区。

## v22 相册下拉列表数量右对齐（2026-11-18）

- 用户反馈：相册列表各标题右侧的图片数量数字未贴最右。查实 `PopupMenuItem` 外层包 `Align(centerStart)`，菜单项内容以 `SizedBox(width:256)` 固定宽度渲染，小于菜单实际宽度，数量停在 256px 处而非菜单右缘。
- 修复：移除 `SizedBox(width:256)`，`Row` 直接占满菜单项可用宽度，标题仍 `Expanded`，数量与选中图标、数量间距保持，从而贴到菜单右缘。
- 验证：`dart format` PASS；`flutter analyze --no-pub <identity_avatar_flow.dart>` PASS，0 问题。widget 测试 NOT_RUN（沿用用户此前要求）；合并、发布 NOT_RUN。待设备复验数量右对齐。

## v23 相册下拉列表宽度对齐按钮（2026-11-18）

- 用户反馈：数量右对齐后，相册标题列表宽度短于触发按钮，两者边缘不对齐。查实菜单约束为固定 `minWidth:288`，面板宽度由菜单项内容决定，未与按钮宽度一致。
- 修复：`_albumMenuConstraints` 读取按钮实际宽度作为 `minWidth`（小于 288 时兜底 288），面板宽度与按钮对齐，高度收缩逻辑保持不变。
- 验证：`dart format` PASS；`flutter analyze --no-pub <identity_avatar_flow.dart>` PASS，0 问题。widget 测试 NOT_RUN（沿用用户此前要求）；合并、发布 NOT_RUN。待设备复验列表宽度与按钮对齐。

## v24 性能优化（动画重绘/重排与资源释放，2026-11-18）

保持视觉与功能不变的前提下，针对本轮涉及的页面与动画做最小性能收口：

1. **头像缩略图请求缓存**：`_AlbumThumb` 由 StatelessWidget 改为 StatefulWidget，缩略图 `Future` 在 `initState` 建立并缓存。此前 `FutureBuilder` 在 build 内新建 `thumbnailDataWithSize` Future，网格滚动/头部重建时重复触发原生解码，造成滚动卡顿与资源浪费。
2. **半屏展开只重建外层 Padding**：`_AlbumSheet` 头部的 `AnimatedBuilder` 将整段头部 Column 固定为 `child`，展开进度只驱动外层 `Padding(top)`，避免拖动逐帧重建相册按钮、箭头与文案，减少无谓重排。
3. **裁切首帧省一次重建**：`_AvatarCropEditor._decode` 顶部移除 `setState(() { _error = null; _busy = true; })`——`initState` 调用时字段初值已表达忙态，省去 build 前一次无谓重建；超 5MB 分支与异常分支的 setState 保留。
4. **页签切换动画加 RepaintBoundary**：`personal_profile.dart` 的 `_SliverTransform` 包一层 `RepaintBoundary`。此前 RenderSliver 逐帧 `markNeedsPaint` 会重绘整棵帖子卡片子树；隔离后动画帧只重绘被平移的合成层，显著降低重绘开销。
- 验证：`dart format` PASS；`flutter analyze --no-pub <identity_avatar_flow.dart> <settings_screen.dart>` PASS，0 问题；`<personal_profile.dart>` 仅剩 1 条存量 `deprecated_member_use`（applyPaintTransform 的 `Matrix4.translate`，v10 已记录，非本次引入）。widget 测试 NOT_RUN（沿用用户此前要求）；合并、发布 NOT_RUN。待设备复验动画观感与滚动流畅度无回归。

## v25 修复相册下拉崩溃（`_elements.contains` 断言，2026-11-18）

- 用户反馈设备红屏：`framework.dart:2170 '_elements.contains(element)' is not true`。根因：v21/v23 用 `GlobalKey.currentContext` + `findRenderObject` 在 build 阶段跨树读取按钮位置，v24 又把头部 Column 固定为 `child`，展开进度变化使按钮 element 被替换，`currentContext` 短暂指向已卸载 element，触发断言。
- 修复（最小改动，仅 `identity_avatar_flow.dart`）：移除 `GlobalKey` 与 `currentContext`/`findRenderObject` 跨树读取。菜单约束改为确定性来源——`minWidth` 取屏幕宽减左右内边距（对齐按钮整宽），`maxHeight` 按 `sheetController.size` 半屏 240 / 全屏 376 自适应，不再依赖 render box。
- 验证：`dart format` PASS；`flutter analyze --no-pub <identity_avatar_flow.dart>` PASS，0 问题；grep 确认无残留 `currentContext`/`findRenderObject`/`_albumButtonKey`。widget 测试 NOT_RUN（沿用用户此前要求）；合并、发布 NOT_RUN。待设备复验相册下拉正常、无红屏。

## v26 回退 sliver 上的 RepaintBoundary（第二次红屏，2026-11-18）

- 用户反馈第二次红屏：`framework.dart:6430 InheritedElement.notifyClients` 断言 dependent 必须是当前 element 的后代。
- 根因：v24 性能优化在 `_SliverTransform` 与 `SliverMainAxisGroup` 之间加了 `RepaintBoundary`。`RepaintBoundary` 是 box 组件，而 `CustomScrollView.slivers` 只接受 sliver；该包裹破坏视口子节点与元素依赖关系，触发断言。
- 修复：仅移除 `personal_profile.dart` 中这一层 `RepaintBoundary`，恢复原来的 `_SliverTransform -> SliverMainAxisGroup` 结构；其它性能优化（缩略图 Future 缓存、动画头部 child 隔离、裁切首帧去重）保留。grep 确认该文件无 `RepaintBoundary` 残留。
- 验证：`dart format` PASS；`flutter analyze --no-pub <personal_profile.dart>` 仅剩 1 条存量 `deprecated_member_use`（v10 已记录）；页面主体代码本轮未改。widget 测试 NOT_RUN（沿用用户此前要求）；合并、发布 NOT_RUN。待设备完整重建后复验无红屏。

## v27 全部回退性能优化并移除控制器读取（第三次红屏，2026-11-18）

- 用户反馈 `DraggableScrollableSheet` 未挂载断言：`DraggableScrollableController is not attached to a sheet`。直接原因：v25 的 `_albumMenuConstraints` 在 build 阶段读取 `widget.sheetController.size`，此时控制器可能尚未 attach。
- 处理：按用户要求回退 v24 全部性能优化——`_AlbumThumb` 恢复 StatelessWidget + build 内 Future；相册头部恢复整段随展开重建；`_AvatarCropEditor._decode` 恢复初始 setState。`personal_profile.dart` 的 `RepaintBoundary` 已于 v26 移除。
- 菜单约束改为不访问控制器：宽度取屏幕宽减左右 16 内边距，最大高度按 `屏幕高 * .62 - 160` 保守计算并限制 112–376，避免半屏上翻且不再触发控制器断言。
- 验证：`dart format` PASS；`flutter analyze --no-pub <identity_avatar_flow.dart> <personal_profile.dart>` 无 error/warning，仅 1 条存量 deprecated info。widget 测试 NOT_RUN（沿用用户此前要求）；合并、发布 NOT_RUN。待完整重建复验。

## v28 PR 提交（2026-11-18）

- 用户回报已完成真机测试（本记录不替代具体设备、步骤与结果证据）。
- 本地提交 `1ccff70`，已变基到当前 `origin/main` `b86b4fb`；`flutter analyze --no-pub apps/mobile` 0 error / 0 warning，4 条存量 info。
- 本次授权范围：推送 `feat/profile-settings-entry` 并创建面向 `main` 的 PR；不包含合并或发布。

## PR #20 CI 修复（2026-11-18）

- 用户续办授权修复并推送 [PR #20](https://github.com/zewbby/SpeechFUL-Develop/pull/20)，由新子代理独立审查后，在 CI 通过且审查无阻塞时合入 `main`。本轮基于 `2449176`，目标基线 `b86b4fb`。
- 对齐新版设置入口、分组、外观记录与已确认的动画行为，保留跨账号隔离、退出确认、身份切换、转帖操作与大字布局验证。恢复转帖更多按钮的可访问名称，并叠加应用字号与系统缩放。
- 原提交的六条 Flutter 工作流 FAIL，见 PR 检查。修复候选的 widget 回归由 GitHub Actions 执行，本机 NOT_RUN。独立审查、合并和发布状态以 PR 和本轮交付记录为准。
- 独立审查 `4294a00` 建议 Request changes。复跑发现窄屏大字卡片溢出、待发送私信的持续动画不能用全局静止判定，以及同账号重登录的控件定位。审查另发现设置资料在会话失效后残留、顶部手势误切页签、关闭相册时过期加载仍回写。上述确定性问题随 CI 收口。
- 用户确认修改密码须保持当前登录，并要求暂缓合并、单独设计改密流程。设置直接调用的校邮重置流程要求退出当前会话。改密入口维持审查阻塞，后续方案须保留既有校邮恢复和验证码约束。本轮合并、发布均未执行。

## 登录态改密与共用相册（2026-11-18）

- 用户确认实施两项改动。设置中先取码并验证当前账号绑定的校邮，再输入和确认新密码。一次确定提交，服务端确认后直接打开登录表单，并提示「密码修改成功，请重新登录」。取消与失败的请求不得误报成功，超时核对原提交。
- 使用 PR #20 的 `19b8d76` 为基线，沿用同一个隔离 Worktree。校邮取码使用现有 V 验证码、限流与发码挑战；C 的登录态改密意图绑定校邮槽位、原会话及操作。在最终 C Safety Gate 事务中写密码、撤销旧会话并记录结果。原找回密码接口继续使用其自身授权用途。
- 头像、发帖和编辑、评论及失败重选、私信、举报举证接入共用相册弹层。相册支持单选和按调用入口限制多选。照片权限采用图片授权，系统选择器用于权限回退。头像独立进入圆形裁切，预览和 PNG 输出使用同一裁切区域。
- 当前实现差异的 Go 测试、vet、API／Verifier 构建和 OpenAPI 全量校验 PASS。WSL2 私有 PostgreSQL `AUTHLAB_EMAIL_TEST_PATTERN='^Test(PasswordChange|EmailReset)' sh authlab/run-email-login-isolated.sh` PASS，覆盖真实校邮证明、改密前会话可用、旧密码和会话失效、新密码登录及旧找回回归。Flutter widget 由 PR 产品 CI 执行，本机 NOT_RUN。原生相册权限、实际媒体读取和裁切手势的真机验收待办。
- Flutter 3.47.5 `flutter analyze --no-pub` PASS。首轮远端运行发现多个旧 Goose 夹具仍断言迁移版本为 52，已更新至改密迁移 53，保留数据回归与拒绝破坏性回滚的断言。
- 合并暂缓，发布未执行。[PR #20 检查](https://github.com/zewbby/SpeechFUL-Develop/pull/20/checks)。
