# AUTH-EMAIL-RESET-FEEDBACK-01 — 校邮验证后无账号提示

- 模式：常规；负责人／状态维护人：当前 Session；无委派。
- 分支／Worktree／PR：`codex/fix/email-reset-unregistered`；`E:/Hnuhole-env/source/SpeechFUL-email-reset-unregistered`；基线 `89b592e2761a75f053cf6ec87444c71a52afb070`（已 fetch 核对 origin/main）；未创建 PR。
- 授权依据与边界：2026-11-11 用户连接手机授权排查 v1.1.0 找回 bug，明确验证码正确、点击验证仍提示通用失败；随后要求先修改无账号提示。允许本地修复、验证；不改真实账号／密码／资格记录，不沿用其他任务的发布授权。
- 需求确认：用户最终指定原文“该邮箱未找到账户信息，请先注册”。校邮验证方无账号、校邮已验证但社区侧返回 EMAIL_UNREGISTERED，均使用该文案；其他错误不得冒充无账号。无未决取舍。
- 后续授权：用户明确要求“推送合并，合并成功后删除本次产生的所有空间”，覆盖本任务推送、PR、适用 CI 通过后合并及本任务分支／Worktree／诊断构建文件清理；不覆盖正式部署或手机安装包升级。清理后本机日志／截图链接不再可用，以本记录摘要及 PR CI 保留证据。
- 更新日期：2026-11-11。
- 状态：实现 已完成；验证 PASS（本地认证回归／静态分析／Android 构建）；合并 未合并；发布 未发布。

## 目标与范围

修正找回账号中“输入正确验证码 → 验证校邮 → 社区侧未找到账号”的错误反馈，替换笼统的“操作未完成，请检查输入或核对原操作”。同步校邮验证方无账号时的已有页面提示和繁中／英文／日文翻译。保持现有校邮验证、账号查询及最终安全检查。

## 接口与依赖

复用现有 EMAIL_UNREGISTERED，无服务端或 API 契约改动。注册未知结果闭环在另一工作区的 AUTH-REGISTRATION-RECOVERY-01 中进行，本分支不改写该任务或其未提交实现。本修复只改善无账号结果的提示，不改变已有资格或替用户重新注册。

## 验收标准

- 正确验证码已通过，社区侧返回 EMAIL_UNREGISTERED 时，弹窗显示用户指定原文；关闭后可更正邮箱继续操作，无密码提交或自动注册。
- 校邮验证方返回 UNREGISTERED 时，同样显示指定文案，不请求社区重置许可。
- 服务不可用等其他错误不显示无账号提示，不误清仍需核对的重置证明。
- 既有正常重置完成回登录页、未知结果与会话代次回归保持通过。

## 当前进度与证据

| 项目 | 被测版本／环境 | 结果与证据 |
| --- | --- | --- |
| 手机版本 | vivo V2323A；ADB 只读核对 | PASS：安装版本 1.1.0，versionCode 2026111102；未清数据、卸载或改密码 |
| 故障事实 | 现有试用环境，只读事务；仅输出状态，不导出邮箱／槽位／密码／OTP／证明 | PASS：校邮验证结果为 PROOF_AVAILABLE，对应社区账号查询无匹配，未创建重置许可；校邮控制权验证与社区账号存在是两个阶段 |
| 代码调查 | 基线及 v1.1.0 标签源码 | FAIL（旧实现）：EMAIL_UNREGISTERED 清理本次重置状态后落入通用失败文案 |
| 本地修复 | 基线 SHA 加本任务 auth_flows、auth_screen、翻译及 email_reset_test 差异 | 已完成；改动由本任务分支后续本地提交承载 |
| 静态分析 | Windows Flutter 3.47.5；`flutter analyze --no-pub` | PASS：无问题，见本机证据目录的 flutter-analyze.log |
| 认证／交互回归 | 同一基线＋上述差异；`flutter test --no-pub test/email_reset_test.dart test/email_login_test.dart test/auth_flows_test.dart test/auth_screen_test.dart test/auth_api_test.dart test/app_localizations_test.dart --reporter expanded` | PASS：136 项；新增 V 无账号、C 无账号、服务异常三种实际按钮交互，含关闭错误弹窗后可更正邮箱；正常重置、未知结果及会话代次回归通过。见 flutter-tests.log |
| 依赖与格式 | `flutter pub get --offline --enforce-lockfile`（pub.dev）；显式 Dart 文件格式化；`git diff --check` | PASS：锁文件无差异。首次默认镜像离线解析产生的版本漂移已撤销后按锁文件重新解析；首次沙箱测试在 SQLite 官方资产下载阶段受阻，正常工具环境下载校验后上述测试通过 |
| Android 构建 | 同一基线＋上述差异；`flutter build apk --debug --no-pub --target-platform android-arm64` | PASS：assembleDebug 112.2 秒，见 android-build.log；仅本地验证包，未覆盖用户手机或发布 |
| 差异及协作检查 | `git diff --cached --check`；`python tools/check-collaboration.py --staged` | PASS；协作检查仅证明协作规则，不代替产品验收 |
| 产品 CI／手机新包验收 | 未推送／未创建 PR；用户手机仍为原 v1.1.0 | NOT_RUN；不将本地界面替身测试或构建冒充线上／真机修复验收 |

本机证据目录：`C:/Users/24237/Documents/ChatGPT/Hnuhole/recovery-prompt-review-20261111`。日志、截图及只读诊断脚本不提交。

## 阻碍与下一步

本地修复与聚焦验证已完成。按后续明确授权推送、创建 PR、核对适用 CI 后合并并清理本任务空间；PR 的实时 CI 与合并记录作为后续交付状态。正式发布、手机升级及线上复验未执行，手机数据与原安装包保留。
