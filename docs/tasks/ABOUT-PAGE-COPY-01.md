# ABOUT-PAGE-COPY-01 — 更新关于 SpeechFUL 页面文案

- 模式：常规
- 负责人／状态维护人：当前 Session
- 执行 Session（指挥模式）：无
- 分支／Worktree／PR：`codex/feat/about-page-copy`；`E:/code_collection/java_code/SpeechFUL.git`；[PR #37](https://github.com/zewbby/SpeechFUL-Develop/pull/37)
- 本轮规则版本（指挥模式）：不适用
- 授权依据与边界：用户 2026-11-25 提供最终文案，要求建立分支、完成修改、验证并创建 PR；不改应用包版本、发布元数据或合并。
- 需求确认：关于页按用户提供文本展示，入口版本与页内版本同步为 `v2.0.0`；补齐繁体中文、英文和日文翻译，并保证长文案可滚动。无未决取舍。
- 更新日期：2026-11-25
- 状态：实现 已完成；验证 PASS；合并 未合并；发布 未发布

## 目标与范围

将“关于 SpeechFUL”页面替换为已确认的项目介绍、交流功能、身份隐私说明和独立项目声明；更新设置入口版本值及四语翻译。页面在窄屏和大字号下可滚动且无布局溢出。

## 接口与依赖

无接口、数据或服务依赖。

## 验收标准

- 设置入口显示 `v2.0.0`，关于页显示同一版本及用户提供的完整中文文案。
- 关于页内容只在选择对应语言时展示该语言的完整翻译。
- 窄屏、横屏或大字号下内容完整可达，不出现溢出。

## 当前进度与证据

| 项目 | 被测版本／环境 | 结果与证据 |
| --- | --- | --- |
| 实现 | `a7418bc`（基于 `develop/main@a20a45e`） | 已完成关于页文案、版本值、四语翻译及可滚动布局。 |
| 静态分析 | Windows 11 25H2 x64；Flutter 3.47.5／Dart 3.13.4；`flutter analyze --no-pub` | PASS：No issues found。 |
| 聚焦测试 | 同一环境；`flutter test --no-pub test/settings_screen_test.dart test/app_localizations_test.dart --reporter expanded` | PASS：18 项，包含四语完整文案及 320×360、1.8 倍字号滚动无溢出。 |
| 全量移动端测试 | 同一环境；`flutter test --no-pub --concurrency=2` | FAIL（环境）：1285 项通过、4 项跳过、11 项失败；失败均为 TLS 夹具调用不存在的 `openssl`，与关于页差异无关。 |
| 本地 Android 构建 | 同一环境；`flutter build apk --debug --no-pub`、`flutter build bundle --debug --no-pub` | BLOCKED：本机未安装或未配置 Android SDK。 |
| 产品 CI | [PR #37](https://github.com/zewbby/SpeechFUL-Develop/pull/37)，head `a7418bc` | PASS：11 个工作流成功；22 个检查中 21 个成功、1 个条件跳过、0 失败，包含 Android 预发布、iOS 编译、Flutter 与集成回归。 |

## 阻碍与下一步

本机缺少 Android SDK 与 `openssl`，因此本地目标包与全量 TLS 测试不可复现；对应产品链路已由 PR 产品 CI 覆盖。合并与发布仍待用户明确授权。
