# AVATAR-FEED-PROJECTION-01 — 更换头像后首页帖子头像及时更新

- 模式：常规
- 负责人／状态维护人：当前 Session
- 执行 Session（指挥模式）：无
- 分支／Worktree／PR：`fix/avatar-feed-projection`；目标 `develop/main@c6d98df`，分支起点 `dbbc249`（merge-base 同）；实现提交 `c16b8e6`（rebase 前 `ed90104`），审查修复 `01adb68`，远端 PR head `fab02f7`，PR 合并树 `1450b8f`（与 `refs/pull/25/merge` `82c487f` 的树一致）；PR [#25](https://github.com/zewbby/SpeechFUL-Develop/pull/25)
- 本轮规则版本（指挥模式）：不适用
- 授权依据与边界：用户报告“修改头像后主页我发过帖子的头像不会改变，点进帖子页又更新，别人改头像我能看到更新”。本次只修复该展示同步问题；不改服务端、OpenAPI、迁移、发布流程。用户 2026-11-20 指令“提交pr”，授权推送分支并创建 PR；2026-11-20 指令“修复这鞋问题”，授权修复 PR #24/#25 的审查问题；2026-11-21 指令“推送最新pr”“修复ci”，授权推送最新提交并重跑失败的 CI；合并、发布、部署仍未授权。
- 需求确认：用户期望换头像后自己已发帖子的头像无需进入详情即可更新；同时保留详情页读取最新投影的现状。未涉及产品取舍；无未决取舍。
- 更新日期：2026-11-21
- 状态：实现 已完成（含审查修复 `01adb68`）；验证 PASS（自动化 + 真机手工；审查修复后的真机复测 NOT_RUN，强制读取失败注入 NOT_RUN）；新 head `fab02f7` 的 CI PASS（24 success、1 skip）；PR [#25](https://github.com/zewbby/SpeechFUL-Develop/pull/25) 已创建；合并 未合并；发布 未发布

## 目标与范围

修复更换头像后已显示帖子仍使用旧 `author.avatar` 的展示问题。覆盖首页时间线（`FeedController.timelineItems`）以及“我的”页本人帖子投影；帖子详情继续以服务端最新读取结果为准。不包含服务端投影改动、历史帖子数据迁移、头像字节缓存策略调整或发布操作。

根因：身份变更返回后，`PostController.refreshKnownPosts()` 只刷新旧 `feedItems`，而首页 `FeedScreen` 实际展示 `PostController.homeFeed.timelineItems`，后者没有刷新，因此保留旧头像；进入详情时重新读取服务端最新作者投影，所以显示新头像。他人修改头像可见，是本机列表重新加载服务端最新投影的结果。

## 接口与依赖

无新增接口。沿用现有帖子列表、详情与身份目录读取；只调整客户端在身份资料变化后的已显示投影刷新路径。

## 验收标准

- 更换当前身份头像并返回首页后，首页已显示的本账号帖子头像更新为新头像，无需打开帖子详情或重启应用。
- 换个身份或重新进入首页后，列表顺序、分页游标与滚动数据不被重置。
- “我的”页已加载的本人帖子头像与身份目录中的当前头像一致。
- 帖子详情仍读取服务端最新作者投影；身份目录读取失败时保持现有失败提示与重试语义。
- 现有首页增量刷新、删除/隐藏过滤和互动状态不受影响。

## 当前进度与证据

| 项目 | 被测版本／环境 | 结果与证据 |
| --- | --- | --- |
| 实现 | 基线 `develop/main@dbbc249`；分支 `fix/avatar-feed-projection`，实现提交 `c16b8e6`（rebase 前 `ed90104`），审查修复 `01adb68` | `FeedController.refreshDisplayedProjections()` 按当前已显示帖子 ID 重新读取公共时间线页，原位替换标题、正文、作者、标签与图片投影，保留列表顺序、分页游标、频道条件、删除过滤和本地较大的浏览量；`PostController.refreshKnownPosts()` 在身份变更后先调用该刷新，再保留原有旧列表刷新。新增首页与个人页头像刷新回归测试，并补充跨页、默认不插入新帖和浏览量保留断言。审查修复：翻页终止不再由已显示帖子数量推导固定页数，改为公共时间线按发布时间倒序翻到严格早于最早已显示帖子即停止，头部新增大量帖子导致页面位移时不再漏刷；`_applyProjection` 合并服务端、列表旧值与本地 `_postViews` 三者的最大浏览量，不用较小投影覆盖本地已观测值。 |
| 审查修复验证 | `01adb68`；Windows 11 25H2 x64 / Flutter 3.47.5 / Dart 3.13.4 | `flutter test --no-pub test/avatar_feed_projection_test.dart test/feed_incremental_test.dart test/feed_ui_test.dart -r compact` → 52 项 PASS；`flutter test --no-pub test/post_controller_test.dart test/personal_profile_test.dart test/login_home_navigation_test.dart test/active_identity_test.dart -r compact` → 77 项 PASS；完整 `flutter test --no-pub --concurrency=2 -r compact` → 1372 项通过、4 项跳过 PASS。新增“本地更大浏览量不被覆盖”与“41 条新帖位移后仍刷新到最早已显示帖子且不请求第 5 页”两项用例，临时回退 `feed_controller.dart` 时均按预期失败（浏览量 5000→1260、最旧帖子头像未更新）；`flutter analyze --no-pub` → `No issues found!`；`dart format --output=none --set-exit-if-changed` → 0 项变化；`git diff --check` → PASS。真机复测 `NOT_RUN`；新 head `fab02f7` 已推送，CI `PASS`（24 success、1 skip）。 |
| 验证 | `c16b8e6`（rebase 后，rebase 前 `ed90104`）；Windows 11 25H2 x64 / Flutter 3.47.5 / Dart 3.13.4 | 自动化：`flutter test test/feed_incremental_test.dart test/avatar_feed_projection_test.dart --no-pub -r expanded` → 7 项 PASS（新增首页用例修复前失败、修复后通过；个人页用例修复前后均通过）；`flutter test test/post_controller_test.dart test/feed_ui_test.dart test/personal_profile_test.dart test/login_home_navigation_test.dart test/active_identity_test.dart --no-pub -r compact` → 118 项 PASS（rebase 后复跑 120 项 PASS）；`flutter analyze --no-pub` → PASS（No issues found，rebase 后复跑 PASS）；完整 `flutter test --no-pub --concurrency=2 -r compact` → 1362 项通过、4 项跳过 PASS（rebase 前）；受影响 4 个 Dart 文件 `dart format --output=none --set-exit-if-changed` → 0 项变化；`git diff --check` → PASS。rebase 后定向与受影响套件已复跑；完整套件由 PR CI 覆盖。 |
| 真机验证 | 设备 APK 构建于 rebase 前 HEAD `ea62171`（实现提交 `ed90104`，之后仅文档差异）；rebase 到 `dbbc249` 后修复提交为 `c16b8e6`，修复 diff 相同，设备 APK 不含 #21 的无关改动；debug APK 构建于 2026-11-20 13:17；Xiaomi `24129PN74C` / Android 17 / API 37 / arm64-v8a；`org.hnuhole.hnuhole_mobile` 1.2.0（`2026111701`）；一次性本地开发 C/V 服务（`127.0.0.1:8443/8444`，经 `adb reverse`），一次性合成账号 | 场景 1 PASS：在“校园吐槽”发布合成帖“Avatar check”后，首页已显示该帖默认字母头像；在“我的”页更换头像为橙蓝测试图，返回首页未打开详情、未重启，首条本人帖子头像原位更新为橙蓝图；帖子详情显示同一新头像；“我的”页头部与本人帖子卡片同步为新头像。场景 2 PASS（滚动保持）：将首页滚动到“作者主页验收帖 6/5/4/3”后，在“我的”页更换头像为绿星图并返回首页，四张卡片的屏幕位置与更换前逐项一致（`[39,533][1161,680]`、`[39,719][1161,1278]`、`[39,1317][1161,1876]`、`[39,1915][1161,2474]`），列表顺序未变；滚回顶部后本人帖子与页头头像均为绿星图，“我的”页头部与本人帖子卡片亦为绿星图。服务端记录头像由 `default-v1` 更新为 `avatar-v1:…`（开发库查询）。截图留存于本地未提交的 `.zcode/device-avatar-17-before-change.png`、`device-avatar-24-home-after.png`、`device-avatar-25-detail.png`、`device-avatar-26-feed-scrolled.png`、`device-avatar-27-home-green.png`、`device-avatar-28-profile-green.png`。强制身份目录读取失败/重试注入 `NOT_RUN`（未断开 C）；分页游标直接观测 `NOT_RUN`（顺序与滚动未重置，未出现列表重建）。产品 CI：PR [#25](https://github.com/zewbby/SpeechFUL-Develop/pull/25) head `fab02f7` 的 24 项检查 success、1 项 skip（`actual-process-acceptance` 按设计跳过），无 failure。 |

## 阻碍与下一步

自动化与真机主链路验证已完成：首页已显示本人帖子在换头像后原位更新，详情与“我的”页一致，列表顺序和滚动位置保持不变。审查修复后的自动化已通过，但真机复测、强制身份目录读取失败/重试注入、分页游标直接观测仍为 `NOT_RUN`；现有自动化测试覆盖失败分支、游标保持和页面位移。已推送的 PR [#25](https://github.com/zewbby/SpeechFUL-Develop/pull/25) 最新 head `fab02f7` CI 全部通过（24 success、1 skip）；审查修复 `01adb68` 及其后的文档提交已推送；合并、发布与部署仍未授权。2026-11-21 复查确认首页不渲染置顶帖（置顶仅出现在“我的”页 UI），按发布时间倒序的翻页终止条件成立；未发现新的正确性问题。
