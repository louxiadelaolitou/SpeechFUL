# iOS 工程

首版功能共用 `../lib`。原图选择／保存由 `packages/native_media` 的 PhotosUI／Photos 插件提供；私信本地通知由 `Runner/DmNotifications.swift` 提供。任务范围与实际验证状态见 [IOS-V1-01](../../../docs/tasks/IOS-V1-01.md)。

- 最低 iOS 15；原生 Passkey 要求 iOS 16+。不支持对应原生仪式时仍使用现有密码／恢复码流程。
- 选择 PNG、JPEG、GIF、WebP 时不进行客户端格式转换。HEIC 等不在现有服务端契约内，由已有预检提示格式不支持；不为 iOS 改变上传规则。
- 相册写入仅在用户点保存后申请 add-only 权限，Photos 不支持原格式时返回失败，不转换或自动重复保存。
- 私信通知只覆盖 App 已接收并落盘的消息及重新打开后的同步；没有 APNs、后台常驻或被杀进程推送承诺。通知点击仍需 Flutter 账号／身份／会话校验。
- 对齐当前站内分享，不注册延期的外部帖子链接或系统分享入口。

## 有 Mac 后构建

使用 Flutter 3.47.5 与配套 Xcode。当前项目使用 Swift Package Manager；原生自有插件也保留 CocoaPods podspec。先在仓库根目录执行 `git lfs pull`，再运行：

```sh
cd apps/mobile
flutter pub get --enforce-lockfile
flutter build ios --simulator --debug --no-codesign
```

后端地址沿用 Android 的可信 HTTPS 构建配置：`--dart-define=AUTH_COMMUNITY_BASE_URL=...` 与 `--dart-define=AUTH_VERIFIER_BASE_URL=...`。不使用 ATS 全局放宽或信任任意证书。

## 签名与 Passkey 域名关联

未填写 Apple Team 或虚构凭据。实际部署时用现有 `tools/configure-passkey-associations.py`，提供真实 `--rp-id`、`--ios-team-id`、`--ios-bundle-id` 和新的 `--out` 目录；Bundle ID 默认 `org.hnuhole.hnuholeMobile`，RP ID 必须与服务端一致。

将生成的 `Runner.Passkey.entitlements` 放入 `Runner/`，在本地忽略的 `Flutter/Local.xcconfig` 配置：

```xcconfig
DEVELOPMENT_TEAM = <实际 Apple Team ID>
SPEECHFUL_ENTITLEMENTS = Runner/Runner.Passkey.entitlements
```

由域名维护者在 RP 的 `/.well-known/apple-app-site-association` 发布生成的 webcredentials 关联，并启用匹配的签名 capability。配置模板及编译均不证明 Passkey 仪式、备份／卸载行为或真实相册交互已经通过。
