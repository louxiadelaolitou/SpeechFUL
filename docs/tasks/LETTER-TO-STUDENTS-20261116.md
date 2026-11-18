# LETTER-TO-STUDENTS-20261116 - 致同学的一封信

- 模式：常规；负责人／状态维护人：当前 Session。
- 分支／Worktree／PR：`codex/fix/guest-letter-entry`；`E:/code_collection/java_code/SpeechFUL.git`；基于 `develop/main@315f1513f9362013956676234cd6ef40c1d234b5`；本轮 PR [#19](https://github.com/zewbby/SpeechFUL-Develop/pull/19)。此前统一 PR [#8](https://github.com/zewbby/SpeechFUL-Develop/pull/8) 已合并，其登录首页按账号展示方案由本轮取代。
- 授权依据与边界：用户要求调整弹窗主题、仅未登录展示、移除设置入口，并重新创建 PR。覆盖本任务开发、验证、提交、推送和 PR；合并、发布未授权。
- 需求确认：2026-11-17 用户明确要求弹窗使用 App 现有蓝色主题，按钮、组件和字体与 App 一致；仅在未登录状态弹出；移除侧滑设置中的入口。此前确认的文案、“我已阅读”按钮及正文滚动到底后才能确认保持不变。
- 更新日期：2026-11-17。
- 状态：实现与本机聚焦验证完成；PR #19 已创建，产品 CI 以 PR 当前 head 为准；原生设备及生产验收未运行。

## 目标与范围

在未登录欢迎页首次展示《致同学的一封信》。已登录首页不展示；同一安装阅读一次后不再自动展示，新安装重新展示。设置中不提供重读入口。

## 文案

### 致同学的一封信

同学，你好：

欢迎来到 SpeechFUL，一个自由说话的地方。

一门课程的体验，一家值得推荐的饭店，一次寻找搭子的尝试，或是一件困扰自己的事情，都可以在这里聊聊。校园生活里有许多的事，我们想为这些声音留出位置。

我们希望，匿名能让表达轻松一些。你可以用自己选择的名字，分享经历、提出疑问，倾诉一些自己独特的感受。

让世界多一些爱与真诚，这是我们写在 SpeechFUL 里的愿望。

谢谢你来到这里。

SpeechFUL项目组

## 接口与依赖

- 根组件只在 `AuthStatus.signedOut` 时把 `EntryScreen` 包进 `StudentLetterGate`；`starting`、`restoring`、`authenticated`、`unavailable` 等状态不挂载信件。
- 阅读回执使用 `FlutterAuthVault` 的独立本机命名空间，只由环境派生，不再按账号派生。确认后写入并读回核对；同一安装退出、登录或切换账号都不会改变该回执。
- 弹窗正文与标题使用 App 现有 `TextTheme` 和字体回退；主色、阅读进度和按钮使用 `HhTelegramPalette.action`（`#229AF0`）及现有 `FilledButtonTheme`。旧宋体子集、许可证、来源文档和生成工具已移除。
- 设置页不再引用或展示 `settings-student-letter`。分享链接与私信通知不再等待信件阅读完成。

## 验收标准

- 未登录首次进入欢迎页时自动展示信件；已登录任何状态均不展示。
- 会话恢复期间不展示，确认最终为未登录后才展示。
- “我已阅读”在正文第一屏不可见、不可触达；正文滚动到底后出现并可点击。
- 点击后保存安装级回执并关闭弹窗，欢迎页保留；保存失败时弹窗保持打开并允许重试。
- 同一安装不再重复展示，新安装重新展示；设置中找不到信件入口。
- 明暗主题、蓝色按钮、App 字体、小屏大字号和横屏无溢出或遮挡。

## 验证证据

| 项目 | 被测版本／环境 | 结果与证据 |
| --- | --- | --- |
| 分支基线 | `develop/main@315f1513f9362013956676234cd6ef40c1d234b5`；Windows | PASS：`codex/fix/guest-letter-entry` 工作区基于该提交，未改动旧 PR。 |
| PR 创建 | 实现提交 `b97f9bd9fe7c956777e37954ce53f4fba9dd04be` | PASS：开发仓库 PR #19 已创建，open、mergeable；最终 head 与 CI 以 PR 实时状态为准。 |
| 静态检查 | 本轮差异；Windows／Flutter 3.47.5／Dart 3.13.4 | PASS：`flutter analyze --no-pub` 输出 `No issues found!`。 |
| 信件聚焦测试 | 同上；`test/student_letter_test.dart` | PASS：10 项，覆盖未登录首次展示、登录不展示、恢复期不展示、滚动门槛、安装级回执、新安装重展示、保存失败重试及 5 组明暗／语言／尺寸。 |
| 关联回归 | 同上；登录、身份、导航、私信运行时及主题集成 | PASS：42 项通过。 |
| 渲染检查 | 本轮差异；`apps/mobile/build/student-letter-render/` | PASS：5 组场景共 10 张首末截图；实际查看中文明暗主题及 2 倍字暗色末尾，蓝色强调、App 字体、按钮与滚动闸门正常。 |
| 协作检查 | 本轮暂存差异 | PASS：`git diff --cached --check` 无问题，`python tools/check-collaboration.py --staged` 通过。 |
| PR 产品 CI | PR #19；GitHub Actions | RUNNING：创建后首次汇总中多数检查 queued，`actual-process-acceptance` skipped；最终结论以 PR 当前 head 为准。 |
| 原生设备／生产 | 本轮宿主验证 | NOT_RUN：宿主存储替身与组件截图不证明原生存储、真机或生产验收。 |

## 历史说明

旧方案曾在登录首页按账号＋安装实例展示，并允许从设置重读；对应 PR #8 已合并。本轮将其改为未登录安装级展示，旧行为不再作为当前验收依据。

## 下一步

等待 PR #19 的当前 head CI 完成并接受审查。合并与发布等待明确授权。
