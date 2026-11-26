# MOBILE-FEEDBACK-PENDING-TOGGLE-01 — 统一轻量反馈与未完成集合按钮

- 模式：常规
- 负责人／状态维护人：当前 Session
- 分支／Worktree／PR：`feat/home-profile-update`，已 rebase 到 `93deaf1`（`origin/main`；原分支从 `a20a45e` 建立），未创建 PR
- 授权依据与边界：用户要求统一应用内提示交互并统一「我的」页未完成发布集合的展开/收起按钮；仅修改 `apps/mobile` 展示与交互，不改业务数据、权限或服务端；未 push、未合并、未部署。
- 需求确认：轻量反馈显示后 2 秒自动关闭，提供明显叉号，支持左右滑动移除；其他同类 SnackBar 统一接入；展开/收起按钮同一样式并有按压动画。无未决取舍。
- 更新日期：2026-11-26
- 状态：实现 已完成；验证 PASS（Widget／静态检查），真机手势 NOT_RUN；合并 未合并；发布 未发布

## 目标与范围

新增 `showHhFeedback` 作为应用内 SnackBar 的唯一展示入口，将首页「不感兴趣」失败提示及帖子、评论、私信、举报、通知、设置、分享、头像流程中的同类提示全部接入。SnackBar 仅作计时宿主（透明底、零内边距），可见胶囊自绘：配色取 `HhTelegramPalette`（surface/line/ink/secondary），高度紧凑（单行约 38px）；新提示替换当前提示，不排队叠加；2 秒自动关闭；叉号关闭；整颗胶囊（容器、图标、文字）随左右滑动一起位移并可移除。

「我的」页未完成发布集合的展开和收起按钮复用 `_PendingToggleButton`，统一填充配色、尺寸、圆角和图标，并通过局部 `ScaleTransition` 与既有 `HhMotion` 时长提供按压缩放；保留原展开/收起状态及内容高度动画。

不包含「不感兴趣」成功后的原位隐藏/撤销占位，也不改变各提示的既有业务文案与触发条件。

## 接口与依赖

- `lib/src/design_system/feedback.dart`：`showHhFeedback(BuildContext, String, {bool replaceCurrent = true})`。
- 复用根 `ScaffoldMessenger`、`hhText`、`HhTelegramPalette` 和 `HhMotion`。
- 显式禁用 SDK 默认单向 SnackBar 滑动手势，由内容区统一处理双向手势，避免手势竞争。

## 验收标准

- 「不感兴趣」失败及其他原 SnackBar 均呈现统一轻量反馈；业务源码不再直接调用 `showSnackBar`。
- 2 秒自动关闭、叉号立即关闭、左滑和右滑均可关闭。
- 胶囊为紧凑单行高度，配色与当前主题（浅色／深色 palette）一致；滑动时容器与文字同步位移。
- 连续触发不排队显示多条；快速关闭及提示期间页面切换均可正常关闭。
- 展开和收起按钮样式一致，使用共享按压动画；点击后集合状态与原有一致。

## 当前进度与证据

| 项目 | 被测版本／环境 | 结果与证据 |
| --- | --- | --- |
| 实现 | `ab47547`（统一入口与全量接入）＋ `8e57ad7`（紧凑胶囊、整颗滑动、主题配色） | 统一反馈入口及全部 25 个调用点收敛；`_PendingToggleButton` 统一按钮外观与按压动画。 |
| Widget 验证 | Windows 本机 Flutter 3.50.0；被测内容 = `8e57ad7` | PASS：`flutter test --no-pub test/feedback_test.dart test/post_screens_test.dart test/feed_ui_test.dart test/identity_avatar_photo_test.dart test/motion_test.dart test/personal_profile_test.dart`，114 项通过、1 项原配置跳过。覆盖紧凑高度（<44）、浅／深色主题配色、胶囊与文字同步位移、自动关闭、叉号、连续触发、左右滑动、切页关闭、展开/收起及既有主页／头像／动效回归。 |
| 全量回归 | 同上，`flutter test --no-pub` | 1291 通过／4 跳过／12 失败；11 项为 TLS fixture 用例（本机缺 `openssl`，无法生成自签证书），1 项 `dm_chat_draft_ui_test` 仅在全量并行时失败、单跑通过。均与本轮差异无关，不计入本任务结论。 |
| 静态检查 | 同上 | PASS（无 error／warning）：`flutter analyze --no-pub` 全量 202 条 info 均为既有 Material 弃用等提示；`git diff --check` 通过；协作规范检查（合规分支名与 PR 标题）通过。 |
| 真机／模拟器 | 无 | NOT_RUN：实际触屏手感仍需设备验收。 |

## 阻碍与下一步

无代码阻碍。未执行 push、PR、合并或部署；如进入发布验收，在目标设备复验滑动手感与页面切换时的提示表现。
