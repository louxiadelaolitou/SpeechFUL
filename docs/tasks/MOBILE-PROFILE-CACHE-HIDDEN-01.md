# MOBILE-PROFILE-CACHE-HIDDEN-01 — 我的页页签缓存与不感兴趣状态边界

- 模式：常规
- 负责人／状态维护人：当前 Session
- 执行 Session（指挥模式）：无
- 分支／Worktree／PR：`feat/home-profile-update`，已 rebase 到 `93deaf1`（`origin/main`；原分支从 `a20a45e` 建立），未创建 PR
- 本轮规则版本（指挥模式）：不适用
- 授权依据与边界：用户本轮指令“修复并优化以下两处问题……我调整的是 mobile 安卓部分，同时做好后要把对应的调试功能做到这里面 lib/dev”。仅改 `apps/mobile`；未改服务端、未合并、未发布；push 与 PR 未授权。
- 需求确认：页签缓存按“首次加载后复用卡片详情、互动、本人能力”实现，保留现有 Threads 式切页签滚动行为；「不感兴趣」按最直接语义限制为“自己的帖子不写入隐藏记录”，其他帖子仍沿用账号级长期隐藏（FEED-VECTOR-CARDS-01）。澄清问题未在窗口内得到回复，此取舍待用户最终确认。
- 更新日期：2026-11-26
- 状态：实现 已完成；验证 PASS；合并 未合并；发布 未发布

## 目标与范围

1. 「我的」页五个页签（帖子／回复／转帖／喜欢／收藏）首次加载后缓存并复用已读取的卡片详情、互动与本人编辑／删除能力；再次切回页签不重复发起同类请求，直接展示已有内容。页签列表、排序与滚动位置行为保持原样。
2. 首页信息流三点菜单的「不感兴趣」不得作用于自己账号发布的帖子：点击时不写入隐藏记录、不改动屏蔽分类或当前身份，并给出提示。其他帖子的账号级长期隐藏与撤销行为不变。
3. 在 `apps/mobile/lib/dev` 增加调试面板：可观察假接口调用计数、对页签切换做基线对比，并可一键自检自己的帖子／他人帖子的隐藏状态边界。

不包含：服务端接口改动、iOS 适配、正式发布或合并。

## 接口与依赖

- 复用 `PostController` 的 `loadDetail`、`loadInteractions`、`postCapabilities`、`changeHiddenPost`；不新增生产 HTTP 接口。
- 「不感兴趣」的自有帖子判断复用帖子能力接口：200 表示当前账号自己的帖子，404 表示他人帖子；无法核对时停止写入。
- 调试面板仅依赖 `lib/dev` 的 `PreviewPostsApi`、`HomeTestPostsApi` 与正式 `PostController`。

## 验收标准

- 首次进入「我的」页并加载帖子／收藏后，切到其他页签再切回，`detail`、单帖互动、能力读取计数不再增加，卡片直接显示已缓存内容。
- 页签本身的列表请求（自己的帖子、回复、转帖、喜欢、收藏）仍只在首次进入或显式下拉刷新时发起。
- 对自己帖子点击「不感兴趣」：隐藏记录、屏蔽分类、当前身份均不变，界面给出“不能对自己发布的帖子设置不感兴趣。”提示。
- 对他人帖子点击「不感兴趣」仍会隐藏并在原位显示撤销；撤销后恢复，屏蔽分类与身份不变。
- 调试面板显示调用计数与最近调用，自带基线与对比；自己的帖子／他人帖子自检分别显示 PASS／FAIL。
- 现有视觉、菜单项与切页签滚动位置不变。

## 当前进度与证据

| 项目 | 被测版本／环境 | 结果与证据 |
| --- | --- | --- |
| 实现 | `2a8687d`（rebase 前的 `c791f94`） | 已完成 `personal_profile.dart` 卡片级缓存、`hidden_post_controller.dart` 归属边界；`lib/dev/dev_diagnostics.dart` 调试面板按用户要求于 2026-11-26 删除，`lib/dev` 预览入口保留 |
| 验证 | Windows，Flutter SDK 3.50.0，命令 `flutter test test/feed_ui_test.dart test/personal_profile_test.dart` | PASS：58 个测试全部通过 |
| 验证 | 同上，`test/post_screens_test.dart test/post_controller_test.dart test/post_edit_screen_test.dart test/community_localization_test.dart` | PASS：92 个测试全部通过 |
| 验证 | 同上，`test/personal_reposts_integration_test.dart` | PASS |
| 验证 | 同上，`flutter analyze --no-pub` | 无 error／warning；仅既有 deprecated info |
| 已知限制 | 同上，`lib/dev/dev_preview_smoke_test.dart` 的通知页模板用例 | 与本次改动无关、依赖当前时间的既有用例失败，未计入本次验收 |

## 阻碍与下一步

- 待用户确认「不感兴趣」边界是否确为“自己的帖子不生效”；若用户实际期望“只隐藏这一条、不改任何账号级偏好”，则需调整 `changeHiddenPost` 的持久化策略。
- 调试面板已删除（用户 2026-11-26 确认“没啥用处”），其边界自检由 `test/feed_ui_test.dart` 的用例覆盖；如需再次可视化观察，需重新实现。
