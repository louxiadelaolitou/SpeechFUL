# LOGOUT-RETURN-WELCOME-01 — 退出确认后停留欢迎页

- 模式：常规；负责人／状态维护人：当前 Session。
- 分支／Worktree／PR：`codex/fix/logout-welcome-rebuilt-20261115`；`D:/SpeechFUL/.worktrees/logout-welcome-rebuilt-20261115`；基线为重建仓库 `origin/main@100b9a15da04270e61037fa3649c05ebe7197cd6`；[新仓库 PR #1](https://github.com/zewbby/SpeechFUL-Develop/pull/1)。
- 授权依据与边界：2026-11-15 用户要求将旧 PR #149 对应代码按规范推送到现有远端，并明确“148不要，148有bug”。本轮只移植 #149 最终的退出、登录流程及必要测试、预发布归档工具修正；不挑入 #148 提交，不改远端主线，不发布或装机。
- 需求确认：退出按钮先弹确认框；取消保留设置页和会话；确定后退出并稳定停在欢迎页，不自动进入登录页，也不显示“暂时无法进入”。用户主动点击“开始”才打开登录；登录成功后直接进入浏览首页，等待目录期间停留登录表单，不闪欢迎页或账号页。以上由本任务先前用户消息确认；无未决取舍。
- 更新日期：2026-11-15。
- 状态：实现已完成；本地验证 PASS；新 PR 已创建、未合并；新仓库 CI 以 [PR #1 的检查](https://github.com/zewbby/SpeechFUL-Develop/pull/1/checks)为准；未发布。

## 目标与范围

将旧 PR #149 的最终行为从本地提交 `53171f2556fc8542de48c41068df8657a7235e66` 移植到重建仓库。设置页退出、根级导航、登录表单及入口故障弹框按已确认流程联动；覆盖取消、确认、嵌套受保护页面、撤销重试、安全存储失败和延迟目录。未引入旧 #147 的字标资源与文档；未挑入有缺陷的 #148 提交。

## 接口与依赖

无新增服务端接口或数据迁移。沿用 `AuthSessionController.logoutVersion`、会话状态、目录状态与根级 Navigator。新远端的主线基于 `f5127dc`，缺少旧 #148/#149；移植采用旧 #149 最终产品文件差异，不合入旧分支历史。`tools/prerelease/environment.py` 仅归档回滚构建所需的 `services/api`，避免无关 LFS 字体在无凭据 CI checkout 中触发下载。

## 验收标准

- 设置页点击退出只打开确认框；取消后设置页与会话保持。
- 点击确定后才清除会话与受保护路由，稳定停在欢迎页；旧页面回调不能再自动打开登录，退出过程不显示目录失败弹框。
- 用户点击欢迎页“开始”才打开登录；登录成功后待首页目录可用，一次性进入浏览首页，不显示欢迎或“账号已登录”中间页。
- 服务端撤销待办重试不改变欢迎页；安全存储故障仍进入原故障处理。
- 新仓库当前提交的适用产品 CI 通过；分支与 PR 链接到本记录，不把旧仓库 CI 当作新仓库验收。

## 当前进度与证据

| 项目 | 被测版本／环境 | 结果与证据 |
| --- | --- | --- |
| 实现 | `3adbbdc`（11 个产品／测试文件）、`4539d8a`（预发布归档工具）；基线 `100b9a1`；来源为本地旧 #149 最终提交 `53171f2` | PASS：最终行为已移植，未挑入 #148 提交；不包含 #147 字标资源。 |
| Flutter 静态与聚焦回归 | Windows Flutter 3.47.5、Dart 3.13.4；`apps/mobile` 内 `flutter analyze --no-pub`；6 个认证、启动、导航、设置、身份测试文件 | PASS：零分析问题；78 项测试通过。 |
| 认证产品回归 | 同环境；`.github/workflows/auth-feedback.yml` 中完整 Flutter 测试清单 | PASS：289 项测试通过。 |
| 预发布工具测试 | Windows Python；`python -m unittest discover -s tools/prerelease/tests -v` | PASS：16 项完成，其中 8 项按 Linux 归属／专用 PostgreSQL 环境条件 SKIP。 |
| 构建／设备 | 当前新分支 | NOT_RUN：旧 #149 的手机试用与旧 CI 属于历史版本，不作为新分支的构建、设备或远端检查证据。 |
| 新仓库 CI | [PR #1 检查页](https://github.com/zewbby/SpeechFUL-Develop/pull/1/checks)；以 PR 当前 head 为准 | 待最终核对适用产品检查及协作检查，结果见实时检查页。 |

## 阻碍与下一步

任务分支已推送并创建新 PR。核对新仓库当前 head 的产品 CI；合并和正式发布另行授权。
