# PUBLISH-UI-01 — 发布任务页重构与身份胶囊对齐

- 模式：常规
- 负责人／状态维护人：当前 Session
- 执行 Session（指挥模式）：无
- 分支／Worktree／PR：`feat/PublishPageAndfix`，基线 `8a34c73`；工作区未提交，无 PR
- 本轮规则版本（指挥模式）：不适用
- 授权依据与边界：用户于 2026-11-24 要求重构发布任务页与「发布中」卡片、把「仅看未读」改为两态切换、统一通知/私信/我的身份胶囊动画并在我的页显示红点；随后确认：可调整按钮分组与文案并改用滑入路由、卡片显示封面＋x/y＋查看进度、接受为通知未读数增加一次请求、切换控件只显示当前状态、动画包含昵称/头像淡入淡出。仅改移动端 UI 与未读数据通路，不改服务端协议。
- 需求确认：已确认，无未决取舍
- 更新日期：2026-11-24
- 状态：实现 已完成；验证 PASS（目标用例）；合并 未合并；发布 未发布

## 目标与范围

1. 发布任务页（`_PostTaskScreen`）改为：状态卡（状态图标/颜色、分类·时间、发布中转圈）、正文卡、提示、图片上传进度块（进度条、x/y、待补传、继续上传）、主操作「核对原操作」（紧跟上传块）、「可选操作」分组（停止未受理、恢复草稿、放弃文字、取消发布、修改文字重试、删除失败入口，条件不变）、错误横幅；入口改用 `HhSlidePageRoute`。
2. 「我的」帖子页的草稿/发布中卡片：首图缩略图（本机媒体，读不到显示占位）、标题两行截断、发布中状态行（转圈＋发布中＋x/y＋查看进度）、失败/草稿状态、右对齐删除按钮。
3. 通知页「仅看未读」改为单个胶囊开关，只显示当前状态：开启时高亮「仅看未读」，关闭时显示「查看全部」；点击切换并沿用原有清缓存重载逻辑。
4. 通知页、私信页、「我的」页共用同一身份胶囊组件：统一几何尺寸、按压反馈、箭头旋转（HhMotion.quick）、菜单动效（HhMotionRoutes.popup）、昵称/头像切换淡入；「我的」胶囊在其他身份有未读私信或未读通知时显示红点。
5. 手动测试模板 `apps/mobile/lib/dev/task_ui_templates.dart`：一个入口页，含通知页、发布中卡片（发布中/草稿/失败/未受理/结果未知）、发布任务页（发布中/失败/未受理）与浅/深主题切换；只替换假数据，复用正式组件，不新增生产路由。`lib/dev` 已在本地 `.git/info/exclude` 中，不进仓库。

不包含：服务端接口/协议变更、通知页整体业务重写、身份管理页交互。

## 接口与依赖

- 通知未读数：复用 `GET /api/v1/notifications/unread-counts` 的 `identityUnread` 字段；`GovernanceController.identityUnreadCounts` 保存，通知页与壳层红点共用。
- 私信未读：壳层从 `DmController.conversations` 的 `identityId`/`unread` 行汇总，过滤 hidden/deleted。
- 本机首图：`PostComposerMedia.personalCoverBytes` 读取草稿媒体（只读，不触发上传）。

## 验收标准

- 发布任务页：各状态只显示对应条件动作，分组后不丢失任何动作或禁用逻辑；发布中/失败/未受理/已发布视觉一致；图片上传能看到进度、待补传与继续上传。
- 发布中卡片：显示首图、发布中、x/y 与「查看进度」，点击进入任务页；草稿/失败卡片仍可长按或点删除。
- 仅看未读：无复选框；单击在「仅看未读 / 查看全部」间切换，标签反映当前状态，列表与未读计数按新筛选重载。
- 身份胶囊：三处尺寸与动画一致；切换身份时昵称与头像淡入；我的页胶囊在其他身份有未读私信或通知时出现红点。
- 手动模板：`flutter run -t lib/dev/task_ui_templates.dart` 可打开入口页并进入通知页、卡片页、三种任务页状态，切换深浅色不报错。
- 320dp、大字、深色/浅色主题不溢出（沿用既有约束）。

## 当前进度与证据

| 项目 | 被测版本／环境 | 结果与证据 |
| --- | --- | --- |
| 实现 | `feat/PublishPageAndfix`，基线 `8a34c73`，工作区改动 | 见上「目标与范围」；main.dart、identity_switcher.dart、inbox_screen.dart、governance_controller.dart、post_screens.dart、personal_profile.dart、post_composer_photos.dart、post_composer_media.dart 及两处本地化词条 |
| 实现 | 手动测试模板（本地，不入库） | `apps/mobile/lib/dev/task_ui_templates.dart`（入口 + 假数据 + 假治理接口）与 `apps/mobile/lib/dev/dev_templates_smoke_test.dart`；生产侧仅新增 `hhDevPersonalPostCard`/`hhDevPostTaskPage` 两个 dev seam 及卡片/任务页的可选 `uploadProgress` 覆盖参数，正式入口默认 null |
| 验证 | `flutter analyze --no-pub`，Flutter 3.47 dev（`D:\000_Programming_software\flutter`），2026-11-24 | 0 error / 0 warning；info 均为该 SDK 既有 `material.dart` 弃用提示 |
| 验证 | `flutter test` 目标用例，同上环境 | PASS：102 项（含新增 `lib/dev/dev_templates_smoke_test.dart` 2 项：卡片页显示「发布中 2/3」「查看进度」，任务页显示「核对原操作」「图片上传」；通知页「查看全部」↔「仅看未读」切换且无 Checkbox） |
| 验证 | `flutter test`（全量） | 13 项 FAIL，均为需 `openssl` 生成本地 TLS 证书的 http/live-probe 用例（`ProcessException: openssl ...`）及并行互扰的 dm 草稿用例（单跑 PASS）；与本轮改动无关，属环境缺 openssl |
| 实现 | `preview_app` 调试入口接入（本地，不入库） | `PreviewLauncher` 六个入口（完整 App / 通知页 / 卡片 / 任务页三态）；`HomeTestFixture.create(devGovernance: true)` 注入假治理接口，完整 App 内通知页有数据源；模板页与假数据构造改为公开（`DevCardsTemplatePage`、`DevTaskTemplatePage`、`DevNotificationTemplatePage`、`hhDev*Item`） |
| 验证 | `flutter analyze --no-pub`，Flutter 3.50.0 stable，2026-11-24 | 0 error / 0 warning |
| 实现 | `preview_app`「我的」页铺满发布状态（本地，不入库） | `HomeTestFixture.create(devPersonalStates: true)`：`_Store` 返回 5 条本地记录（草稿、草稿·限流等待、未受理、结果未知、原结果无法核对），`HomeTestPostsApi.tasks()` 返回「发布中」「发布未成功」两条任务，并补上 `task()` 点查；卡片渲染、排序、点击全部走正式代码 |
| 实现 | 修复 `preview_app` 返回入口页后再进「完整 App」崩溃 | `PreviewLauncher` 改为接收 `buildApp` 工厂，每次进入都重新装配 Controller，规避 `IdentityManagementController was used after being disposed`（`HnuholeApp` 出栈即 dispose 内部控制器） |
| 验证 | `flutter test lib/dev/dev_preview_smoke_test.dart`，Flutter 3.50.0 stable，2026-11-24 | PASS 5/5：入口列表、通知页模板、卡片模板→任务页、「我的」页出现发布中/草稿/发布未成功/未受理/结果未知、点「发布中」进入对应「发布任务」页、卸载后重建不再抛 dispose 异常 |
| 实现 | 交互细节第二轮（2026-11-24） | ①个人帖子卡片统一为同一 Material 容器（raised + 边框 + 16 圆角 + `Clip.antiAlias`），草稿/失败/未受理/未知不再是无底色的裸文本，点击水波纹按圆角裁切；②发布任务页「可选操作」分组改用 Material 承载，水波纹不再越出 16 圆角；③通知卡片时间改为贴右显示，未读点移到时间左侧；④发布任务页把「核对原操作」移到图片上传进度之后，其余动作统一收进「可选操作」（原「其他操作」） |
| 实现 | 交互细节第三轮（2026-11-24） | ①发布中卡片的「查看进度」移到标题行，与标题行箭头同用蓝色，状态行只留转圈＋发布中＋x/y；②「核对原操作」并入「可选操作」第一行，与其余行同一结构（图标圆底＋文字＋箭头），行高与字号加大、主操作强调色；③「我的」页未完成/未发布帖默认合成一叠（同一容器内紧凑行＋N 条未完成＋点击展开），展开后为完整卡片并可收起，卡片间距从 12 收到 6；④文案改半正式：状态词统一为发布中/发布失败/发布未通过/结果待确认，动作词改查看/重试/取消/删除 |
| 实现 | 交互细节第四轮（2026-11-24） | 未完成/未发布帖子收起时只露最近三条紧凑行，底部「共 N 条未完成」＋「展开」按钮；展开后完整列出全部卡片（不再限制三条、不再有「还有 N 条」入口），底部「收起」按钮；两态用 `AnimatedSize`（260ms easeOutCubic）+ `AnimatedSwitcher`（220ms 淡入淡出）过渡，整块放进 `SliverToBoxAdapter`，已发布帖子仍走独立 `SliverList`；紧凑行状态只显示主词（如「草稿」），细节留给展开后的卡片 |
| 验证 | `flutter analyze --no-pub` + `flutter test`（post_screens / personal_profile / dev 冒烟），2026-11-24 | 0 error / 0 warning；PASS 53/53（含「共 3 条未完成→展开→三张卡片」「7 条→收起只出现前三条、第四条不可见→展开出现全部七条＋收起按钮」） |
| 验证 | 真机 iQOO 12 人工走查，2026-11-24 | PASS：卡片标题行蓝色「查看进度」+蓝箭头；任务页「可选操作」首行为「查看发布结果」；「我的」页未完成合并成叠并可展开 |
| 实现 | 交互细节第五轮（2026-11-24） | ①收起态三条摘要每行改为独立可点区域，直达对应帖子/发布任务页，行尾加箭头提示；展开只由底部按钮触发；②高度过渡改为一次性插值：切换时直接换形态，`AnimatedSize`（300ms easeOutCubic、顶部对齐、硬裁剪）把容器从旧高直接过渡到新高，展开只向下推、收起只向上收，不再先收后放；③因系统「移除动画/动画时长倍率为 0」或无障碍服务会令隐式动画降级为瞬切，过渡外显式 `MediaQuery.copyWith(disableAnimations: false)` 覆盖，保证可见 |
| 验证 | 交互细节第五轮，`flutter analyze --no-pub` + `flutter test`（post_screens / personal_profile / dev 模板 / dev 入口），2026-11-24 | 0 error / 0 warning；PASS 53/53；APK 覆盖安装到真机 `10AE7H2YQ1005BH` 供人工调试 |
| 验证 | 真机人工走查（iQOO 12），2026-11-24 | PASS：卡片模板五种状态同一容器样式；发布任务页顺序为状态卡→正文→提示→图片上传→核对原操作→可选操作；「我的」页草稿/未受理/未知均有容器 |
| 验证 | `flutter build apk --debug -t lib/dev/preview_app.dart --android-skip-build-dependency-validation` | PASS，产出 `build/app/outputs/flutter-apk/app-debug.apk` |
| 验证 | 真机 iQOO 12（`10AE7H2YQ1005BH`）安装人工走查，2026-11-24 | PASS（部分）：入口页、发布中卡片模板、完整 App「我的」页与通知红点可见；通知页、发布任务页真机画面未逐一截图，由上述 widget test 覆盖 |

## 阻碍与下一步

- 改动未提交、未推送，等待用户确认。
- 本地不拟入库的改动：`apps/mobile/pubspec.lock`（`flutter pub get` 版本漂移）、`apps/mobile/analysis_options.yaml`（web/windows 排除）、`apps/mobile/android/gradle.properties`（`kotlin.incremental=false` 本机构建规避）；`apps/mobile/lib/dev/` 已被 `.git/info/exclude` 排除。
- 环境说明：flutter 需 `git` 在 PATH；flutter 自身仓库有 dubious ownership，已用进程级 `GIT_CONFIG_COUNT/GIT_CONFIG_KEY_0/GIT_CONFIG_VALUE_0` 规避，未修改全局 git 配置；`flutter pub get` 曾改写 `pubspec.lock`，已还原，仅 `.dart_tool` 变化（不入库）。
- 环境说明：本机 Flutter 为 3.50.0 stable，要求 AGP ≥ 9.1.1，而 `android/settings.gradle.kts` 为 9.1.0（CI 固定 Flutter 3.47.5）。因此本地 `flutter run/build` 会因依赖校验失败，需加 `--android-skip-build-dependency-validation`，或安装 3.47.5 对齐 CI；未改动 AGP 版本。
- 环境说明：IDE 若报大规模 `foundation`/`ValueNotifier` 未定义错误，是 `PUB_CACHE` 指向 C 盘残缺缓存所致；需设 `PUB_CACHE=D:\998_ForEnglishRequirement\002_CachingEverything\pub\cache` 后重新 `pub get`，当前 `.dart_tool/package_config.json` 已指向该缓存。
- 全量 `flutter test` 中依赖 `openssl` 的用例在本机无法通过；如需完整 CI 证据，应在具备 openssl 的环境复跑。
