# IOS-V1-01 — 首版业务 iOS 移植

- 模式：常规
- 负责人／状态维护人：当前 Session
- 分支／Worktree／PR：`codex/feat/ios-v1-parity`；`E:/Hnuhole-env/source/SpeechFUL-ios-v1`；[PR #117](https://github.com/zewbby/SpeechFUL/pull/117)。基线 `8493c9c6b020de4fa83fe79be7cc7affe15d6e94`（2026-11-07 fetch 后的 origin/main）。
- 授权依据与边界：用户要求“把功能全部应用 iOS”，随后确认没有 macOS、没有 iPhone，范围为“你只做移植就 OK”。2026-11-08 用户明确授权推送本分支、创建 PR、检查和修复 CI，通过后合并 main。允许对应 GitHub 操作与必要修复；不进行正式部署或真实设备验收。
- 需求确认：对齐当前 Android 首版既有业务；不引入新的产品取舍。原图选择保持 PNG/JPEG/GIF/WebP 原始文件，既有格式与预算约束不变；通知沿用运行期间与重新打开后补同步的本地通知；分享沿用 PR #113 的站内私信卡片，不恢复延期的外部分享。
- 更新日期：2026-11-08
- 状态：实现 已完成；验证 本地 PASS／CI 修复后待复验；合并 待最终 CI 通过后按授权合并（实时状态见 PR）；发布 未发布

## 目标与范围

复用 Flutter 共享业务（认证／身份、帖子与编辑、搜索／标签／热榜、评论、互动与个人页、私信／分享、通知、治理、设置／多语言），补齐 iOS 平台阻断项：原图选择、相册保存、私信原生通知、平台描述与配置，核对 Keychain／Passkey 接线。

## 接口与依赖

iOS 15+ 工程、Flutter 3.47.5／Dart 3.13.4。Passkey 使用既有 AuthenticationServices 适配器（iOS 16+），正式域名关联、Apple Team、签名与 iCloud 条件由部署环境提供。后端业务、安全 Gate 与正式数据不变。

## 验收标准

- 所有使用共享选图器的入口均可调用 iOS 原图选择，不重编码原图。
- 查看器及支持二维码可按用户操作保存原图；权限拒绝、不支持的格式和未知写入结果保留明确反馈。
- iOS 私信通知遵守现有账号作用域、默认隐藏预览、免打扰、清理和点击再授权规则。
- 相关 Dart 检查／回归通过；缺少 Apple 环境的构建和系统行为不标为 PASS。

## 当前进度与证据

| 项目 | 被测版本／环境 | 结果与证据 |
| --- | --- | --- |
| 基线 | `8493c9c`，Windows 11 | 已核对实际仓库、远端、worktree；新建独立干净分支。 |
| 环境 | Flutter 3.47.5；`flutter devices` / `flutter emulators` | 仅 Windows／Edge／Android，无 macOS 或 iOS 模拟器。用户已确认，范围仅移植。 |
| 原生盘点 | 基线源码 | iOS 选图被禁用、默认相册保存只启用 Android、私信通知缺 iOS handler；Keychain／加密业务存储及 Passkey 已有 iOS 实现。 |
| 选图移植 | `8493c9c` + 本任务最终差异（随后同一实现提交） | `hnuhole_native_media` 使用 PHPicker `.current` 文件表示、顺序复制、每次最多 9 项；逐张读取、10 MiB 预算、取消／超时／短期选择回收。帖子／编辑、评论、私信、头像、举报统一通过原有选图入口接入。无 UIImage 重编码；头像继续走既有静态 PNG 处理规则。 |
| 保存移植 | 同上 | iOS 通过 Photos add-only 权限与资源文件导入；查看器默认启用 iOS 保存，二维码沿用同一适配器。无 gal iOS 强制解包；不支持格式、权限、空间及未知结果分类反馈，无自动重试。 |
| 私信通知 | 同上 | iOS MethodChannel、系统授权、四语言通用提醒／用户选择的预览、账号绑定／清理、免打扰／已读 retain、前台展示与冷／热点击接线。异步权限查询与添加回调检查作用域及请求有效性；启动清除崩溃遗留的本通道通知。点击仅为路由提示，不代替业务授权。 |
| iOS 工程与认证 | 同上 | 保留既有 Keychain／业务加密存储；Passkey 改为操作时获取窗口，适配 implicit engine。补多语言相册权限说明、UserDefaults reason manifest、签名／域关联配置入口；移除已延期的外部分享桥与 URL scheme。图标、启动图、主题与四语言共享界面复用基线。 |
| 静态分析 | Windows Flutter 3.47.5；`flutter analyze --no-pub`，分别在 `apps/mobile`、`packages/image_viewer`、`packages/native_media` | PASS，三处均无问题。plist／storyboard XML、workflow YAML 解析与 `git diff --check` PASS；这些不代表 Swift 编译。 |
| 原图桥接回归 | `packages/native_media`，`flutter test --no-pub` | PASS 4 项：选取顺序、原字节、一次读取／释放、取消／数量界限、失败释放和保存错误码。MethodChannel 替身，不代表 Photos 实际行为。 |
| 移动业务聚焦回归 | `apps/mobile`，`flutter test --no-pub test/post_photo_picker_test.dart test/post_composer_photos_test.dart test/identity_avatar_photo_test.dart test/dm_governance_test.dart test/dm_notification_navigation_test.dart test/messaging_localization_test.dart test/settings_screen_test.dart --reporter expanded` | PASS 43 项、SKIP 1 项；跳过项为需 `AVATAR_REPRO_INPUT` 的历史外部头像复现，不是平台能力跳过。[日志](E:/Hnuhole-env/ios-v1-mobile-tests.log)。 |
| 查看器与保存回归 | `packages/image_viewer`，`flutter test --no-pub test/viewer_save_test.dart test/viewer_contract_test.dart --reporter expanded` | PASS 66 项，含 iOS 保存状态映射／原字节／不自动重试和进入查看器不请求权限。[日志](E:/Hnuhole-env/ios-v1-viewer-tests.log)。最后将权限测试 mock 指向 iOS channel 后，39 项 contract 子集复验 PASS，[日志](E:/Hnuhole-env/ios-v1-viewer-contract-final.log)。 |
| Passkey 协议回归 | 从 `apps/mobile` 运行 `flutter test --no-pub ../../packages/auth_passkey/test/native_passkey_client_test.dart --reporter expanded` | PASS 9 项，[日志](E:/Hnuhole-env/ios-v1-passkey-tests.log)。独立包无 dev test 依赖，改用应用测试依赖执行；不代表 AuthenticationServices 真实仪式。 |
| Flutter 资源包 | `apps/mobile`，`flutter build bundle --debug --no-pub` | PASS，[日志](E:/Hnuhole-env/ios-v1-bundle.log)。仅共享 Dart／资源构建，不是 iOS App。 |
| iOS 原生构建／系统交互 | 无 macOS／Xcode／iPhone | BLOCKED：无法编译 Swift、运行 Photos／系统通知／Keychain／Passkey。按用户最终范围仅移植，不安排完整验收。 |
| 产品 CI | [PR #117 checks](https://github.com/zewbby/SpeechFUL/pull/117/checks) | 已推送并创建 PR。macOS Dart 检查与 4+34 项测试 PASS；首轮编译发现 registrar 属性的方法调用，`f6cf587` 修复媒体与 Passkey。第二轮编译发现 auth_vault 声明 iOS 13.0，而文件读取 API 要求 13.4；SPM／podspec 统一对齐应用既有 iOS 15。第二轮 Android 候选包及恢复／回滚演练 PASS。Redis AOF 准备曾单次失败，保留失败证据并重跑失败任务。最终 head 的全部适用 CI 须通过才合并，以 PR checks 与合并记录为准。 |

## 阻碍与下一步

本轮移植源码和本地可执行检查已完成。后续有 macOS 时按 [iOS 工程说明](../../apps/mobile/ios/README.md)执行 Xcode 构建；正式签名需要维护者提供 Apple Team、实际 RP 域关联及对应配置。HEIC 等格式仍不在现有上传契约内，不新增转换规则。iOS 系统交互、Passkey 仪式、备份／卸载行为与正式发行均待后续验收；未发布 IPA、未安装用户设备、未更改后端或正式数据。

原生 API 依据：[PHPicker 表示选择](https://developer.apple.com/documentation/photosui/phpickerconfiguration-c.class/preferredassetrepresentationmode)、[Photos 原文件导入](https://developer.apple.com/documentation/photos/phassetcreationrequest)、[UserDefaults required reason](https://developer.apple.com/documentation/technotes/tn3183-adding-required-reason-api-entries-to-your-privacy-manifest)。依赖沿用已锁版本，仅增加本地原生媒体插件。
