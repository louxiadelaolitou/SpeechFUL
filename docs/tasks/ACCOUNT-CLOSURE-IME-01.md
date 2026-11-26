# ACCOUNT-CLOSURE-IME-01 — 修复注销账号页面中文输入

- 模式：常规
- 负责人／状态维护人：当前 Session
- 执行 Session（指挥模式）：无
- 分支／Worktree／PR：`codex/fix/account-closure-chinese-ime`；Worktree `E:\code_collection\java_code\SpeechFUL-account-closure-ime`；[PR #36](https://github.com/zewbby/SpeechFUL-Develop/pull/36)
- 本轮规则版本（指挥模式）：不适用
- 授权依据与边界：用户于 2026-11-25 要求新建分支、修复注销账号界面无法输入中文、核验后创建 PR；不包含合并或正式部署授权
- 需求确认：用户报告真实输入故障；代码确认确认短语输入框显式关闭建议，影响依赖组合输入的中文输入法。修复目标唯一，无未决取舍
- 更新日期：2026-11-25
- 状态：实现 已完成；验证 PASS（自动化；真实中文输入法设备 NOT_RUN）；合并 未合并；发布 未发布

## 目标与范围

修复账号注销确认短语输入框的中文输入法组合输入，使用户可通过中文输入法填写并提交「确定注销账号」。保留原有精确短语校验、密码二次确认、注销协议与状态机，不修改后端、认证数据或注销业务规则。

## 接口与依赖

无跨模块接口或数据契约变化。实际开发基线为 `develop/main@a20a45e`；`origin/main` 为精简发布快照，不含本仓库测试与任务记录。

## 验收标准

- 注销确认短语输入框启用输入法建议，中文组合输入的文字与 composing range 在页面重建后不被清除。
- 输入完整「确定注销账号」后可继续原有密码确认与注销申请流程；错误短语仍不能启用按钮。
- 受影响 Flutter 测试、静态分析通过；适用 Android 构建与真实中文输入法设备验证有明确结果。

## 当前进度与证据

| 项目 | 被测版本／环境 | 结果与证据 |
| --- | --- | --- |
| 实现 | `a20a45e`＋本分支差异 | 确认短语输入框由 `enableSuggestions: false` 改为 `true`，并增加中文 composing range、提交文本与按钮启用回归测试。 |
| 修复前复现 | 本分支测试差异；Flutter 3.47.5／Dart 3.13.4 | FAIL：`enableSuggestions` 实际为 `false`，聚焦测试按预期失败，证明测试能拦截该配置回退。 |
| 聚焦回归 | 同一差异；Windows 11，`flutter test --no-pub test/account_closure_screen_test.dart test/authority_renewal_regression_test.dart test/security_identity_localization_test.dart --reporter expanded` | PASS：15 项，覆盖中文组合输入、精确短语、密码确认、未知结果核对、会话变化及注销本地化。 |
| 静态分析与格式 | 同一差异；`flutter analyze --no-pub`、`dart format --output=none --set-exit-if-changed` | PASS：无 analyze 问题，受影响文件格式稳定，差异无空白错误。 |
| Android 构建／真机 | 本机环境；PR Android CI | 本机 BLOCKED：Flutter doctor 未发现 Android SDK，未配置 `ANDROID_HOME`／`adb`，未下载新 SDK；产品 [Android prerelease](https://github.com/zewbby/SpeechFUL-Develop/actions/runs/45640286118) PASS。真实中文输入法设备 NOT_RUN。 |
| PR 与产品 CI | `4a78543`；[PR #36](https://github.com/zewbby/SpeechFUL-Develop/pull/36) | PASS：11 个工作流全部通过，包括 [Feed UI](https://github.com/zewbby/SpeechFUL-Develop/actions/runs/45640287380) 与 [Collaboration](https://github.com/zewbby/SpeechFUL-Develop/actions/runs/45640286845)。[Direct message chat](https://github.com/zewbby/SpeechFUL-Develop/actions/runs/45640286296) 首次因 Gradle 插件下载网络错误失败，测试与 SQL 作业已通过；仅重跑失败作业后 attempt 2 PASS。 |

## 阻碍与下一步

实现与自动化验证已完成，PR #36 等待审查。真实中文输入法设备仍为 NOT_RUN；合并与正式发布未获授权。
