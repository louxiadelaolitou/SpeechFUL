# AUTHOR-PROFILE-REFRESH-01 — 作者主页视觉与阅读层级优化

- 模式：常规
- 负责人／状态维护人：当前 Session
- 执行 Session：无
- 分支／Worktree／PR：`codex/feat/author-profile-refresh`；`C:/Users/34408/.codex/worktrees/author-profile-refresh/SpeechFUL.git`；[PR #21](https://github.com/zewbby/SpeechFUL-Develop/pull/21)
- 本轮规则版本：`213b8b0` 的 `AGENTS.md`、`CONTRIBUTING.md` 与项目 UI Skill
- 授权依据与边界：2026-11-19 用户要求“建立分支，进行作者主页优化，先生成html模拟，然后再实际改造，最后pr”。授权本任务开发、模拟、验证、提交、推送和创建 PR；不包含合并或发布。
- 需求确认：优化公开作者主页的视觉与阅读层级；保留既有作者读取、分页、私信、账号级拉黑、失效作者、加载、空态、刷新和错误重试语义。不新增粉丝、关注、简介、帖子总数等当前接口不存在的数据，不增加公开账号编号或身份切换。用户于 2026-11-20 查看 HTML 模拟后明确回复“可以，继续”，该模拟方向已确认，无未决取舍。
- 更新日期：2026-11-20
- 状态：实现 已完成；验证 PASS（合入主线后本地检查通过，当前 PR 的产品 CI 待验证）；合并 未合并；发布 未发布

## 目标与范围

从帖子卡片或正文作者入口进入时，先快速确认作者身份，再以更连续的列表阅读其公开帖子。页面保留返回、私信、拉黑、分页、下拉刷新和不可用状态；危险操作从首屏主按钮降级为更多菜单中的明确动作。同步覆盖浅色、深色、窄屏、大字号、空态和长内容。

不包含后端接口、公开资料字段、作者统计、关注关系、身份切换、公开账号编号或新消息能力。

## 接口与依赖

无跨模块接口变化。继续使用 `FeedApi.authorProfile` 返回的 `AuthorProfile`，以及既有 `DmScope`、`AuthorBlockApi`、分页游标和账号授权窗口。

## 验收标准

- 作者主页首屏同时可见作者头像、昵称、私信入口和至少一项帖子内容；长昵称和放大字体不溢出，不遮挡操作。
- 拉黑仍可从更多菜单发起，确认后只在写入成功时退出并刷新可见信息流；失败和拉黑自己被拒绝时保留页面并给出既有原因。
- 帖子按服务端顺序显示，滚动分页只追加且去重；加载更多失败可就地重试，刷新失败不伪装为空态。
- 失效作者保持只读，私信不可发送，空态和状态说明不把未知结果画成正常。
- 浅色与深色主题均使用语义色；所有可点控件至少 44×44 逻辑像素，并保留读屏名称和选中/展开状态。
- 聚焦 `flutter analyze`、作者主页与入口相关 widget 测试、静态 HTML 实际渲染检查；设备与远端 CI 在 PR 后单独记录。

## 当前进度与证据

| 项目 | 被测版本／环境 | 结果与证据 |
| --- | --- | --- |
| 基线 | `develop/main@213b8b0`；Windows | 已核对仓库、远端的完整开发主线与裁剪后的 `origin/main`；实现与测试已无冲突重放到 `213b8b0`。 |
| HTML 模拟 | [`author-profile-refresh-v1.html`](../design/ui-drafts/author-profile-refresh-v1.html)；Edge 157 实际渲染 430×930 | PASS；[完整态浅色](../design/ui-drafts/author-profile-refresh-v1.png)、[空态深色](../design/ui-drafts/author-profile-refresh-empty-dark-v1.png)已检查；中文换行、402×874 手机画布、空态和深色层级正常。 |
| 实现 | 当前工作树相对 `develop/main@213b8b0` 的产品差异 | 已完成公开作者主页的紧凑抬头、单一私信主操作、更多菜单拉黑、连续帖子列表、静态骨架、独立空态和深浅主题适配；读取、分页、刷新、失效作者和账号授权窗口语义未改。 |
| 依赖准备 | Flutter 3.47.5／Dart 3.13.4；Windows | PASS；按最新主线锁文件执行 `flutter pub get --enforce-lockfile`，补齐新增原生相册依赖后完成，未改锁文件。 |
| 静态分析 | 当前工作树；`flutter analyze --no-pub` | PASS；`No issues found!` |
| 联合 widget 回归 | `213b8b0` 上的单提交；`flutter test --no-pub --reporter compact test/feed_ui_test.dart test/post_screens_test.dart test/author_profile_render_test.dart` | PASS；73 项，覆盖作者主页入口、分页、错误重试、失效作者、长昵称／大字号、深色主题、更多菜单拉黑、成功退出与拒绝自拉黑，以及详情／本人页相邻回归。 |
| Flutter 真实渲染 | 同一工作树；`flutter test --no-pub --reporter expanded test/author_profile_render_test.dart` | PASS；402×874、Roboto／Noto Sans SC 真实字体输出 `apps/mobile/build/author-profile-render/light.png`、`dark.png`，已逐张检查；构建产物不入库。 |
| 设计确认 | 2026-11-20 用户消息 | PASS；用户查看 [HTML 模拟](../design/ui-drafts/author-profile-refresh-v1.html)、完整态浅色与空态深色图后回复“可以，继续”，确认当前结构、色彩和阅读层级方向。 |
| PR | [PR #21](https://github.com/zewbby/SpeechFUL-Develop/pull/21)，head `96eaa2f` | 已转为 Ready for review；`mergeable=true`、`mergeable_state=clean`。 |
| 产品 CI | PR #21，head `96eaa2f` | PASS；23 项检查中 22 项 `success`、`actual-process-acceptance` 按条件 `skipped`，0 失败；包含 `compile`、`flutter`、`flutter-and-real-feed`、`flutter-and-android`、`candidate`、`rehearsal` 与 PostgreSQL 检查。 |
| Android 目标构建 | 同一工作树；`flutter build apk --debug --no-pub --target lib/main.dart --target-platform android-arm64` | BLOCKED；本机 Flutter doctor 明确 `Unable to locate Android SDK`，未发现 `ANDROID_HOME`／`adb`／`sdkmanager`。以 PR 产品 CI 的 Android 构建补证，不将本机环境失败记作代码失败。 |
| 主线同步 | PR head `2d8a3bd` + `main@e7e8b01` | PASS；合并提交保留近期工作索引中的两条任务链接；两侧移动端补丁的 `git patch-id --stable` 均一致。 |
| 合并后本地回归 | Windows，Flutter 3.47.5；被测合并树 `8a9e27b16092e2e07e655ff3079a91bbb9337c34`（其后只更新此任务记录） | PASS；`flutter pub get --offline --enforce-lockfile`、`flutter analyze --no-pub`、`flutter test --no-pub --reporter expanded`（`feed_ui`、`author_profile_render`、`post_screens`、`post_composer_media`、`post_controller`、`post_edit_screen`）共 147 项通过；复用了由锁定版本校验的 SQLite DLL。协作检查与 19 项脚本回归通过。 |

## 阻碍与下一步

合入最新主线后，本地静态分析和作者主页、发帖编辑等联合回归通过。以 PR 当前 head 的产品 CI 确认 Android 构建及集成检查。PR 已 Ready，合并和发布待后续明确授权。
