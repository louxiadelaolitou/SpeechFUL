# WELCOME-PLAIN-AT-01 — 恢复欢迎页粗斜体 @

- 模式：常规。
- 分支／Worktree：`codex/fix/welcome-plain-at-main`，`D:/SpeechFUL/.worktrees/welcome-plain-at-main`，基线 `SpeechFUL-Develop/main@e7e8b017a3db915c15cd06168365fa1206a8a27a`。
- 授权依据：2026-11-20 用户要求将普通粗斜体 @ 合入主线。本次范围为开发主线修复，未包含 APK 重新发布。
- 需求确认：沿用 [旧 PR #147](https://github.com/zewbby/SpeechFUL/pull/147) 已确认的字标。`@SpeechFUL` 使用同一 Roboto Black Italic 字体、字号、基线和字距。无未决取舍。
- 更新日期：2026-11-20。

## 回归原因与修复

旧仓库 `SpeechFUL` 在 11 月 14 日通过 `3da3152` 合入修复，重建的 `SpeechFUL-Develop/main` 从共同基线 `f5127dc` 接续。迁移退出流程的 PR 明确保留独立范围，未移植字标。v1.3.0 的实际构建提交 `1a06547` 因此仍用含 Threads 轮廓的 Flutter 字标，并由 [v1.3.0 Release](https://github.com/zewbby/SpeechFUL-Develop/releases/tag/v1.3.0) 发布。迁移范围见 [退出流程任务](LOGOUT-RETURN-WELCOME-01.md)。

将旧修复的 Flutter 矢量路径、SVG、生成器及来源说明恢复到开发主线。欢迎页继续通过同一绘制组件展示固定矢量字标，沿用原有启动及认证流程。

## 验证与交付

本地验证基于 `e7e8b01` 加上述四个产品文件的差异。环境为 Windows、Flutter 3.47.5，日志位于 `E:/Hnuhole-env/welcome-plain-at-main-20261120/`。

| 项目 | 结果与证据 |
| --- | --- |
| 字形一致性 | PASS。复用 `3da3152` 的四个产品文件，固定 SHA-256 的 Roboto Black Italic 重新生成 SVG 和 Dart，产物与工作区一致。 |
| Flutter 依赖和静态检查 | PASS。`flutter pub get --offline --enforce-lockfile` 和 `flutter analyze --no-pub`。 |
| Flutter 界面回归 | PASS。`flutter test --no-pub --reporter expanded test/startup_entry_test.dart test/auth_screen_test.dart test/login_home_navigation_test.dart test/registration_recovery_test.dart`，72 项通过。 |
| 产品 CI 与主线合并 | 以本任务对应的 `SpeechFUL-Develop` PR 检查及合并回执为准。 |
| 真机与发布 | NOT_RUN。现有 v1.3.0 Release 仍对应 `1a06547`。 |
